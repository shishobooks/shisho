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
	"slices"
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
// The number changes when how the harness makes a base does, and the harness
// makes a new one. A base of another number stays as long as a worktree's
// simulator was cloned from it, since a worktree on another commit still
// uses it (unusedBases). CI caches the base by this name
// (.github/actions/setup-ios), so keep it on one line as it is.
const baseSimulatorName = "Shisho Base 5"

// baseSimulatorPrefix names every base, of any number, since Base 5.
const baseSimulatorPrefix = "Shisho Base "

// oldBaseSimulatorPrefix names the bases harnesses made before Base 5. Each
// of those deletes every base under it but its own at every run, so newer
// bases are named outside it.
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

// quietServices are the system services waitForQuietBoot stops, which
// nothing the app or its tests do needs:
//
//   - apsd, Apple's push notification service. On CI runners it reconnects
//     to Apple's servers several times a second, about 60% of the simulator's
//     log and, through the logging daemons that carry that log to Maestro,
//     about 14% of the runner's CPU.
//   - mobileassetd, which downloads iOS's on-demand assets in the background
//     after each boot: Siri, speech, and language models, over a gigabyte
//     of them. A clone throws away what its base had half downloaded and
//     fetches it again as its own copy, costing disk, CPU, and network while
//     the flows run. The app bundles its fonts and needs none of the assets.
var quietServices = []string{"com.apple.apsd", "com.apple.mobileassetd"}

// quietTimeout is how long waitForQuietBoot keeps quieting once the
// simulator has booted, when launchd should take the commands at once.
const quietTimeout = 30 * time.Second

// quietRetry is how long quietSimulator waits between tries, going easy on a
// CI runner busy booting.
const quietRetry = time.Second

// waitForQuietBoot waits for a simulator that `simctl boot` started to
// finish booting, stopping quietServices meanwhile, as soon as launchd takes
// the commands, so mobileassetd has little time to start. launchd sends
// mobileassetd's commands to the user's domain, which a booting simulator
// doesn't have yet: on a CI runner's first boot, not for minutes. It quiets
// once more after the boot, when the commands always take. The services stay
// disabled through the simulator's later boots but not into its clones.
func waitForQuietBoot(ctx context.Context, udid string) error {
	return bootQuietly(ctx, simctl, udid, quietTimeout)
}

// simctlFunc runs a simctl command, as simctl does.
type simctlFunc func(ctx context.Context, args ...string) (string, error)

// bootQuietly is waitForQuietBoot with its simctl, and how long it keeps
// quieting after the boot.
func bootQuietly(ctx context.Context, run simctlFunc, udid string, afterBoot time.Duration) error {
	quietCtx, stopQuieting := context.WithCancel(ctx)
	quieted := make(chan bool, 1)
	go func() { quieted <- quietSimulator(quietCtx, run, udid) }()
	_, err := run(ctx, "bootstatus", udid, "-b")
	stopQuieting()
	if <-quieted || err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, afterBoot)
	defer cancel()
	if !quietSimulator(ctx, run, udid) {
		fmt.Fprintln(logOut(ctx), "appharness: couldn't stop the simulator's background services; it may download iOS assets")
	}
	return nil
}

// quietSimulator disables quietServices and stops them, trying again until
// launchd lists every one as disabled and none as running, or ctx ends, and
// says whether it did. Checking both matters: a try cut off between the
// disable and the bootout leaves a service disabled but running.
func quietSimulator(ctx context.Context, run simctlFunc, udid string) bool {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(quietRetry):
			}
		}
		disabled, err := run(ctx, "spawn", udid, "launchctl", "print-disabled", "system")
		if err == nil && allDisabled(disabled, quietServices) {
			listed, err := run(ctx, "spawn", udid, "launchctl", "list")
			if err == nil && !anyRunning(listed, quietServices) {
				return true
			}
		}
		// Disabling keeps a service from starting; booting it out stops it
		// if it already has.
		for _, service := range quietServices {
			_, _ = run(ctx, "spawn", udid, "launchctl", "disable", "system/"+service)
			_, _ = run(ctx, "spawn", udid, "launchctl", "bootout", "system/"+service)
		}
	}
}

// allDisabled says whether `launchctl print-disabled` printed every one of
// services as disabled.
func allDisabled(printed string, services []string) bool {
	disabled := map[string]bool{}
	for line := range strings.SplitSeq(printed, "\n") {
		label, state, ok := strings.Cut(strings.TrimSpace(line), " => ")
		if ok && state == "disabled" {
			disabled[strings.Trim(label, `"`)] = true
		}
	}
	for _, service := range services {
		if !disabled[service] {
			return false
		}
	}
	return true
}

// anyRunning says whether `launchctl list`, which prints a PID, a status,
// and a label on each line, lists one of services with a PID.
func anyRunning(listed string, services []string) bool {
	for line := range strings.SplitSeq(listed, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] != "-" && slices.Contains(services, fields[2]) {
			return true
		}
	}
	return false
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
	var all []simctlDevice
	for _, devices := range listing.Devices {
		all = append(all, devices...)
	}
	// Without every worktree's state, no base is known to be unused.
	if dir, err := simulatorStateDir(); err == nil {
		if used, err := worktreeSimulatorBases(dir); err == nil {
			for _, udid := range unusedBases(all, used) {
				_, _ = simctl(ctx, "delete", udid)
			}
		}
	}
	for _, d := range all {
		if d.Name == baseSimulatorName {
			return d.UDID, nil
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

// unusedBases returns the UDIDs of the bases among devices, other than
// baseSimulatorName, that no worktree's simulator was cloned from: used
// holds the bases worktrees' simulators were. A base is named by one of the
// base prefixes and a number, so a simulator someone named "Shisho Base QA"
// isn't one.
func unusedBases(devices []simctlDevice, used map[string]bool) []string {
	var unused []string
	for _, d := range devices {
		if isBaseName(d.Name) && d.Name != baseSimulatorName && !used[d.UDID] {
			unused = append(unused, d.UDID)
		}
	}
	return unused
}

func isBaseName(name string) bool {
	for _, prefix := range []string{baseSimulatorPrefix, oldBaseSimulatorPrefix + " "} {
		if number, ok := strings.CutPrefix(name, prefix); ok && number != "" && strings.Trim(number, "0123456789") == "" {
			return true
		}
	}
	return false
}

// setUp boots a new simulator until iOS finishes its first-boot setup, shuts
// it down, and disallows password AutoFill. It's quieted first, so the base
// carries no half-downloaded assets for its clones to throw away and fetch
// again.
func setUp(ctx context.Context, udid string) error {
	if _, err := simctl(ctx, "boot", udid); err != nil {
		return err
	}
	if err := waitForQuietBoot(ctx, udid); err != nil {
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
	if err := waitForQuietBoot(ctx, udid); err != nil {
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
	if err := waitForQuietBoot(ctx, udid); err != nil {
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
		fmt.Fprintln(logOut(ctx), "appharness: using the simulator booted while the app built")
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
