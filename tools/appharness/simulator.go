package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// baseSimulatorName is the simulator every run clones. The harness makes it
// once and boots it once, so iOS's first-boot setup is done in the base
// rather than in every clone: a clone then boots in seconds instead of half a
// minute here, and minutes on CI. Runs never boot it again, so a clone always
// starts from the same device and runs in other worktrees never share one.
//
// The number changes when how the harness makes a base does; it deletes a
// base of an older number and makes a new one. CI caches the base.
const baseSimulatorName = "Shisho Harness Base 2"

// oldBaseSimulatorPrefix names every base the harness has made.
const oldBaseSimulatorPrefix = "Shisho Harness Base"

// iPhoneType matches a plain current iPhone, such as "iPhone 17".
var iPhoneType = regexp.MustCompile(`^iPhone \d+$`)

type simctlRuntime struct {
	Identifier           string `json:"identifier"`
	Platform             string `json:"platform"`
	Version              string `json:"version"`
	IsAvailable          bool   `json:"isAvailable"`
	SupportedDeviceTypes []struct {
		Name       string `json:"name"`
		Identifier string `json:"identifier"`
	} `json:"supportedDeviceTypes"`
}

type simctlDevice struct {
	UDID        string `json:"udid"`
	Name        string `json:"name"`
	State       string `json:"state"`
	IsAvailable bool   `json:"isAvailable"`
}

func simctl(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "xcrun", append([]string{"simctl"}, args...)...).CombinedOutput() //nolint:gosec // The harness's own simctl arguments.
	if err != nil {
		return "", errors.Wrapf(err, "simctl %s: %s", strings.Join(args, " "), out)
	}
	return strings.TrimSpace(string(out)), nil
}

// baseSimulator returns the base simulator's UDID, making it on the newest
// iOS runtime when it doesn't exist yet. SHISHO_SIMULATOR_BASE names
// another to clone instead.
func baseSimulator(ctx context.Context) (string, error) {
	if named := os.Getenv("SHISHO_SIMULATOR_BASE"); named != "" {
		return named, nil
	}
	out, err := simctl(ctx, "list", "devices", "available", "-j")
	if err != nil {
		return "", err
	}
	var listing struct {
		Devices map[string][]simctlDevice `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		return "", errors.WithStack(err)
	}
	for _, devices := range listing.Devices {
		for _, d := range devices {
			if strings.HasPrefix(d.Name, oldBaseSimulatorPrefix) && d.Name != baseSimulatorName {
				_, _ = simctl(ctx, "delete", d.UDID)
			}
		}
	}
	for _, devices := range listing.Devices {
		for _, d := range devices {
			if d.Name == baseSimulatorName {
				return d.UDID, nil
			}
		}
	}

	out, err = simctl(ctx, "list", "runtimes", "-j")
	if err != nil {
		return "", err
	}
	var runtimes struct {
		Runtimes []simctlRuntime `json:"runtimes"`
	}
	if err := json.Unmarshal([]byte(out), &runtimes); err != nil {
		return "", errors.WithStack(err)
	}
	sort.Slice(runtimes.Runtimes, func(i, j int) bool {
		return versionLess(runtimes.Runtimes[j].Version, runtimes.Runtimes[i].Version)
	})
	for _, rt := range runtimes.Runtimes {
		if rt.Platform != "iOS" || !rt.IsAvailable {
			continue
		}
		for _, dt := range rt.SupportedDeviceTypes {
			if iPhoneType.MatchString(dt.Name) {
				fmt.Fprintf(os.Stderr, "appharness: making the base simulator, an %s on iOS %s, and booting it once\n", dt.Name, rt.Version)
				udid, err := simctl(ctx, "create", baseSimulatorName, dt.Identifier, rt.Identifier)
				if err != nil {
					return "", err
				}
				if err := setUp(ctx, udid); err != nil {
					_, _ = simctl(ctx, "delete", udid)
					return "", err
				}
				return udid, nil
			}
		}
	}
	return "", errors.New("no iOS simulator runtime with an iPhone is installed; install one in Xcode's Components settings")
}

// setUp boots a new simulator until iOS finishes its first-boot setup, then
// shuts it down.
func setUp(ctx context.Context, udid string) error {
	if _, err := simctl(ctx, "boot", udid); err != nil {
		return err
	}
	if _, err := simctl(ctx, "bootstatus", udid, "-b"); err != nil {
		return err
	}
	_, err := simctl(ctx, "shutdown", udid)
	return err
}

// versionLess compares dotted versions numerically.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

// simulatorClone is a disposable copy of the base simulator, booted.
type simulatorClone struct {
	UDID string
}

func cloneSimulator(ctx context.Context) (*simulatorClone, error) {
	base, err := baseSimulator(ctx)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("Shisho Harness %d", time.Now().UnixNano())
	udid, err := simctl(ctx, "clone", base, name)
	if err != nil {
		return nil, err
	}
	clone := &simulatorClone{UDID: udid}
	if _, err := simctl(ctx, "boot", udid); err != nil {
		clone.Remove()
		return nil, err
	}
	if _, err := simctl(ctx, "bootstatus", udid, "-b"); err != nil {
		clone.Remove()
		return nil, err
	}
	return clone, nil
}

// Remove shuts the clone down and deletes it, even after the run was
// interrupted. On a GitHub Actions runner, which is thrown away after the
// job, it leaves the clone, saving the best part of a minute.
func (c *simulatorClone) Remove() {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, _ = simctl(ctx, "shutdown", c.UDID)
	_, _ = simctl(ctx, "delete", c.UDID)
}

// runOnSimulator runs the command after "--" with a disposable simulator
// clone's UDID in SIMULATOR_UDID.
func runOnSimulator(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return errors.New("simulator: name a command after --")
	}
	clone, err := cloneSimulator(ctx)
	if err != nil {
		return err
	}
	defer clone.Remove()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // Runs the command it was given, like env or xargs.
	cmd.Env = append(os.Environ(), "SIMULATOR_UDID="+clone.UDID)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return errors.WithStack(cmd.Run())
}
