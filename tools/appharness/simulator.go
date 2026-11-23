package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// baseSimulatorName is the simulator every worktree's simulators, and every
// disposable one, are cloned from. The harness makes it once and boots it
// once, so iOS's first-boot setup is done in the base rather than in every
// clone. Runs never boot it again, so a clone always starts from the same
// device and runs in other worktrees never share one.
//
// The number changes when how the harness makes a base does; it deletes a
// base of an older number and makes a new one. CI caches the base by this
// name (.github/actions/setup-ios), so keep it on one line as it is.
const baseSimulatorName = "Shisho Harness Base 4"

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
	var base string
	err := withBase(ctx, func(udid string) error {
		base = udid
		return nil
	})
	return base, err
}

// cloneBase clones the base simulator as name, returning the base's UDID and
// the clone's.
func cloneBase(ctx context.Context, name string) (base, clone string, err error) {
	err = withBase(ctx, func(udid string) error {
		base = udid
		clone, err = simctl(ctx, "clone", udid, name)
		return err
	})
	return base, clone, err
}

// withBase calls use with the base simulator's UDID, under a lock that runs
// in other worktrees take too, so none makes a second base or deletes an old
// one while another is cloning it.
func withBase(ctx context.Context, use func(udid string) error) error {
	if named := os.Getenv("SHISHO_SIMULATOR_BASE"); named != "" {
		return use(named)
	}
	dir, err := simulatorStateDir()
	if err != nil {
		return err
	}
	unlock, err := waitForLock(ctx, filepath.Join(dir, "base.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	base, err := findOrMakeBase(ctx)
	if err != nil {
		return err
	}
	return use(base)
}

// findOrMakeBase deletes bases of older numbers, and returns the current
// base's UDID, making it when there's none. Callers hold the base lock.
func findOrMakeBase(ctx context.Context) (string, error) {
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

// setUp boots a new simulator until iOS finishes its first-boot setup, shuts
// it down, and disallows password AutoFill.
func setUp(ctx context.Context, udid string) error {
	if _, err := simctl(ctx, "boot", udid); err != nil {
		return err
	}
	if _, err := simctl(ctx, "bootstatus", udid, "-b"); err != nil {
		return err
	}
	home, err := simctl(ctx, "getenv", udid, "HOME")
	if err != nil {
		return err
	}
	if _, err := simctl(ctx, "shutdown", udid); err != nil {
		return err
	}
	return disallowPasswordAutoFill(ctx, home)
}

// disallowPasswordAutoFill sets the restriction a device management profile
// would, in the settings the first boot wrote, so that iOS never offers to
// save the password after a flow signs in. Turning password AutoFill off in
// Settings only hides the offer: iOS still starts SafariViewService for it,
// and once the app lets that go, iOS throttles it in the background while
// XCTest still reports it in front, so Maestro's driver waits on it each time
// it reads the screen, for up to half a minute on a busy CI runner.
func disallowPasswordAutoFill(ctx context.Context, home string) error {
	for _, settings := range []string{
		"Containers/Shared/SystemGroup/systemgroup.com.apple.configurationprofiles/Library/ConfigurationProfiles/UserSettings.plist",
		"Library/UserConfigurationProfiles/EffectiveUserSettings.plist",
		"Library/UserConfigurationProfiles/PublicInfo/PublicEffectiveUserSettings.plist",
	} {
		path := filepath.Join(home, settings)
		//nolint:gosec // The harness's own plutil arguments.
		if out, err := exec.CommandContext(ctx, "plutil", "-replace", "restrictedBool.allowPasswordAutoFill.value", "-bool", "NO", path).CombinedOutput(); err != nil {
			return errors.Wrapf(err, "disallow password AutoFill in %s: %s", path, out)
		}
	}
	return nil
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
	if clone, ok := takePreparedClone(ctx); ok {
		return clone, nil
	}
	_, udid, err := cloneBase(ctx, fmt.Sprintf("Shisho Harness %d", time.Now().UnixNano()))
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

// preparedCloneName names the clone `base` boots on one of GitHub's runners
// while the app builds, which the job's run then takes instead of cloning
// and booting one after the build: a boot there takes a minute or two.
const preparedCloneName = "Shisho Harness Prepared"

// prepareClone clones the base and boots it, for the job's run to take. It's
// only a head start, so a failure is reported and the run clones its own.
func prepareClone(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if err := bootPreparedClone(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "appharness: couldn't boot the run's simulator ahead of it:", err)
	}
}

// bootPreparedClone boots a clone under a name of its own, and gives it the
// prepared name only once it's ready, so the run never waits on one that
// failed; a failed one is deleted.
func bootPreparedClone(ctx context.Context) error {
	_, udid, err := cloneBase(ctx, fmt.Sprintf("Shisho Harness Preparing %d", time.Now().UnixNano()))
	if err != nil {
		return err
	}
	if _, err := simctl(ctx, "boot", udid); err != nil {
		removeSimulator(udid)
		return err
	}
	if _, err := simctl(ctx, "bootstatus", udid, "-b"); err != nil {
		removeSimulator(udid)
		return err
	}
	if _, err := simctl(ctx, "rename", udid, preparedCloneName); err != nil {
		removeSimulator(udid)
		return err
	}
	return nil
}

// takePreparedClone returns the clone prepareClone booted, if there's one,
// renamed so no other run takes it too.
func takePreparedClone(ctx context.Context) (*simulatorClone, bool) {
	if !disposableRunner() {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	devices, err := listDevices(ctx)
	if err != nil {
		return nil, false
	}
	for udid, device := range devices {
		if device.Name != preparedCloneName || !device.IsAvailable {
			continue
		}
		if _, err := simctl(ctx, "rename", udid, fmt.Sprintf("Shisho Harness %d", time.Now().UnixNano())); err != nil {
			return nil, false
		}
		if _, err := simctl(ctx, "bootstatus", udid, "-b"); err != nil {
			removeSimulator(udid)
			return nil, false
		}
		fmt.Fprintln(os.Stderr, "appharness: using the simulator booted while the app built")
		return &simulatorClone{UDID: udid}, true
	}
	return nil, false
}

// Remove shuts the clone down and deletes it, even after the run was
// interrupted. On one of GitHub's runners, which is thrown away after the
// job, it leaves the clone, saving the best part of a minute.
func (c *simulatorClone) Remove() {
	if disposableRunner() {
		return
	}
	removeSimulator(c.UDID)
}

// runOnSimulator runs the command after "--" with a simulator's UDID in
// SIMULATOR_UDID: the worktree's own for tests (worktreesim.go), with the app
// at -app removed first, or a disposable clone (simulatorForRun).
func runOnSimulator(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("simulator", flag.ExitOnError)
	app := flags.String("app", defaultAppPath, "the app whose data to remove before the command runs")
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		return errors.New("simulator: name a command after --")
	}
	root, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	bundleID, err := appBundleID(ctx, *app)
	if err != nil {
		return err
	}
	sim, err := simulatorForRun(ctx, root, "tests", bundleID)
	if err != nil {
		return err
	}
	defer sim.Release()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // Runs the command it was given, like env or xargs.
	cmd.Env = append(os.Environ(), "SIMULATOR_UDID="+sim.UDID)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	stopWithProcessGroup(cmd)
	err = cmd.Run()
	killProcessGroup(ctx, cmd)
	return errors.WithStack(err)
}

// stopWithProcessGroup runs cmd in its own process group and, when the run
// is interrupted, stops the whole group: a test runner under `sh -c` or
// Maestro's driver would otherwise outlive the run, and the lock it held on
// the simulator, and the next run would reset the simulator under it.
func stopWithProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 30 * time.Second
}

// killProcessGroup kills what's left of cmd's process group after an
// interrupted run, before the run lets go of its simulator: WaitDelay kills
// only cmd itself, and a process under it may have ignored SIGTERM.
func killProcessGroup(ctx context.Context, cmd *exec.Cmd) {
	if ctx.Err() != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// defaultAppPath is where `mise ios:build` puts the app for the simulator.
const defaultAppPath = "mobile/ios/build/DerivedData/Build/Products/Debug-iphonesimulator/Shisho.app"

// appBundleID reads the app's bundle id, which Signing.local.xcconfig may
// have changed.
func appBundleID(ctx context.Context, appPath string) (string, error) {
	if _, err := os.Stat(appPath); err != nil {
		return "", errors.Errorf("no app at %s; build it first with `mise ios:build`", appPath)
	}
	//nolint:gosec // The app this run built.
	out, err := exec.CommandContext(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(appPath, "Info.plist")).Output()
	if err != nil {
		return "", errors.Wrap(err, "read the app's bundle id")
	}
	return strings.TrimSpace(string(out)), nil
}
