package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// worktreeSimulatorPrefix names every worktree's simulator. It mustn't start
// with baseSimulatorPrefix or oldBaseSimulatorPrefix, whose simulators
// baseSimulator deletes.
const worktreeSimulatorPrefix = "Shisho Worktree"

// worktreePurposes are the simulators each worktree has: one for the unit
// tests and one for the flows, so the two can run at once, and one more for
// each other shard of `ios -parallel` (flowsPurpose).
var worktreePurposes = []string{"tests", "flows", "flows-2"}

// idleSimulatorShutdown is how long another worktree's simulator stays
// booted unused before a run shuts it down.
const idleSimulatorShutdown = time.Hour

// readyTimeout is how long cloning, booting, and resetting a worktree's
// simulator may take before it's replaced. A cold boot takes about half a
// minute.
const readyTimeout = 5 * time.Minute

// errSimulatorBusy says a run holds the simulator's lock.
var errSimulatorBusy = errors.New("the simulator is in use")

// runSimulator is the booted simulator a run uses, its UDID in UDID. Release
// it when the run ends.
type runSimulator struct {
	UDID    string
	release func()
}

func (s *runSimulator) Release() { s.release() }

// disposableRunner says whether this runs on one of GitHub's own runners,
// which is thrown away after the job: there's nothing to keep for later runs,
// and nothing to clean up. A self-hosted runner, like a worktree, keeps its
// checkout's simulators warm between runs.
func disposableRunner() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true" && os.Getenv("RUNNER_ENVIRONMENT") != "self-hosted"
}

// freshRun says whether SHISHO_HARNESS_FRESH asks for a run that reuses
// nothing: a disposable simulator, and fixtures made for the run.
func freshRun() bool {
	return os.Getenv("SHISHO_HARNESS_FRESH") != ""
}

// simulatorForRun returns a booted simulator for purpose (one of
// worktreePurposes) in the worktree at root, without the app bundleID or
// anything it kept, so the run starts from a known state.
//
// It's the worktree's own, kept booted between runs, since a clone of the
// base spends half a minute booting, and runs in other worktrees never share
// it. A run holds its simulator's lock for as long as it uses it; a second
// run in the same worktree meanwhile gets a disposable clone. GitHub's own
// runners (disposableRunner) get a disposable clone too, and so does
// SHISHO_HARNESS_FRESH, which also discards the worktree's own.
func simulatorForRun(ctx context.Context, root, purpose, bundleID string) (*runSimulator, error) {
	switch {
	case disposableRunner():
	case freshRun():
		if err := discardWorktreeSimulator(root, purpose); err != nil && !errors.Is(err, errSimulatorBusy) {
			return nil, err
		}
	default:
		sim, err := worktreeSimulator(ctx, root, purpose, bundleID)
		if !errors.Is(err, errSimulatorBusy) {
			return sim, err
		}
		fmt.Fprintln(os.Stderr, "appharness: another run in this worktree is using its simulator; using a disposable one")
	}
	clone, err := cloneSimulator(ctx)
	if err != nil {
		return nil, err
	}
	return &runSimulator{UDID: clone.UDID, release: clone.Remove}, nil
}

// worktreeSimulatorState is what the harness keeps of a worktree's
// simulator, beside its lock, under the user's cache folder.
type worktreeSimulatorState struct {
	UDID string `json:"udid"`
	// Base is the base simulator it was cloned from; a new base replaces it.
	Base string `json:"base"`
	// Root is the worktree's folder; once it's gone, so is the simulator.
	Root string `json:"root"`
}

// worktreeSimulatorFiles are the state and lock files of a worktree's
// simulator, and the simulator's name.
type worktreeSimulatorFiles struct {
	state, lock, name string
}

func simulatorStateDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", errors.WithStack(err)
	}
	dir := filepath.Join(cache, "shisho-appharness", "simulators")
	return dir, errors.WithStack(os.MkdirAll(dir, 0o755))
}

func worktreeSimulatorPaths(dir, root, purpose string) worktreeSimulatorFiles {
	sum := sha256.Sum256([]byte(root))
	hash := hex.EncodeToString(sum[:])[:12]
	id := purpose + "-" + hash
	return worktreeSimulatorFiles{
		state: filepath.Join(dir, id+".json"),
		lock:  filepath.Join(dir, id+".lock"),
		name:  fmt.Sprintf("%s %s %s", worktreeSimulatorPrefix, purpose, hash),
	}
}

func readSimulatorState(path string) worktreeSimulatorState {
	var state worktreeSimulatorState
	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // The harness's own state folder.
		_ = json.Unmarshal(data, &state)
	}
	return state
}

// worktreeSimulatorBases returns the bases the worktrees' simulators in dir
// were cloned from, as their state files say. A state file it can't read
// might name any base, so it fails rather than leave one out.
func worktreeSimulatorBases(dir string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	bases := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path) //nolint:gosec // The harness's own state folder.
		if os.IsNotExist(err) {
			continue // Its worktree was cleaned up meanwhile.
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		var state worktreeSimulatorState
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, errors.Wrapf(err, "read %s", path)
		}
		if state.Base != "" {
			bases[state.Base] = true
		}
	}
	return bases, nil
}

// writeSimulatorState writes a simulator's state file whole, so a reader
// never finds it half written.
func writeSimulatorState(path string, state worktreeSimulatorState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return errors.WithStack(err)
	}
	pending := path + ".new"
	if err := os.WriteFile(pending, data, 0o600); err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.Rename(pending, path))
}

// lockFile takes path's exclusive lock without waiting. The lock ends with
// the process if unlockFile never runs. The file itself stays, so every run
// locks the same one.
func lockFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // The harness's own state folder.
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errSimulatorBusy
		}
		return nil, errors.Wrapf(err, "lock %s", path)
	}
	return f, nil
}

// waitForLock takes path's exclusive lock, waiting for whoever holds it, and
// returns how to let it go.
func waitForLock(ctx context.Context, path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // The harness's own state folder.
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { unlockFile(f) }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, errors.Wrapf(err, "lock %s", path)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, errors.WithStack(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

// worktreeSimulator returns the worktree's simulator for purpose, booted and
// reset, cloning the base for it the first time and after the base changes.
// One that fails to boot or reset is replaced once, so a wedged simulator
// doesn't fail every later run.
func worktreeSimulator(ctx context.Context, root, purpose, bundleID string) (*runSimulator, error) {
	dir, err := simulatorStateDir()
	if err != nil {
		return nil, err
	}
	paths := worktreeSimulatorPaths(dir, root, purpose)
	lock, err := lockFile(paths.lock)
	if err != nil {
		return nil, err
	}
	held := false
	defer func() {
		if !held {
			unlockFile(lock)
		}
	}()
	pruneWorktreeSimulators(ctx, dir, paths.state)

	base, err := baseSimulator(ctx)
	if err != nil {
		return nil, err
	}
	// Each attempt has its own deadline, so a simctl command that hangs
	// leaves time for the replacement.
	attempt := func(replace bool) (string, error) {
		attemptCtx, cancel := context.WithTimeout(ctx, readyTimeout)
		defer cancel()
		return readyWorktreeSimulator(attemptCtx, paths, root, base, bundleID, replace)
	}
	udid, err := attempt(false)
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "appharness: this worktree's simulator failed (%v); replacing it\n", err)
		udid, err = attempt(true)
	}
	if err != nil {
		return nil, err
	}
	held = true
	return &runSimulator{UDID: udid, release: func() { unlockFile(lock) }}, nil
}

// readyWorktreeSimulator boots and resets the simulator paths names, first
// cloning base for it when there's none yet, it's from another base, or
// replace says to.
func readyWorktreeSimulator(ctx context.Context, paths worktreeSimulatorFiles, root, base, bundleID string, replace bool) (string, error) {
	state := readSimulatorState(paths.state)
	devices, err := listDevices(ctx)
	if err != nil {
		return "", err
	}
	device, exists := devices[state.UDID]
	if replace || !exists || !device.IsAvailable || state.Base != base {
		if exists {
			removeSimulator(state.UDID)
		}
		cloned, udid, err := cloneBase(ctx, paths.name)
		if err != nil {
			return "", err
		}
		state = worktreeSimulatorState{UDID: udid, Base: cloned, Root: root}
		if err := writeSimulatorState(paths.state, state); err != nil {
			removeSimulator(udid)
			return "", err
		}
		device = simctlDevice{UDID: udid, State: "Shutdown"}
	}
	// Its last use, for pruneWorktreeSimulators.
	now := time.Now()
	_ = os.Chtimes(paths.state, now, now)
	if device.State != "Booted" {
		fmt.Fprintln(os.Stderr, "appharness: booting this worktree's simulator, which stays booted for later runs")
		if _, err := simctl(ctx, "boot", state.UDID); err != nil {
			return "", err
		}
	}
	if err := waitForQuietBoot(ctx, state.UDID); err != nil {
		return "", err
	}
	return state.UDID, resetSimulator(ctx, state.UDID, bundleID)
}

// resetSimulator removes the app and its data, and empties the Keychain,
// which keeps the app's sign-ins after it's removed.
func resetSimulator(ctx context.Context, udid, bundleID string) error {
	if _, err := simctl(ctx, "uninstall", udid, bundleID); err != nil {
		return err
	}
	_, err := simctl(ctx, "keychain", udid, "reset")
	return err
}

// discardWorktreeSimulator deletes the worktree's simulator for purpose,
// unless a run holds it (errSimulatorBusy).
func discardWorktreeSimulator(root, purpose string) error {
	dir, err := simulatorStateDir()
	if err != nil {
		return err
	}
	paths := worktreeSimulatorPaths(dir, root, purpose)
	// A worktree that never ran the checks has nothing to discard.
	if _, err := os.Stat(paths.state); os.IsNotExist(err) {
		return nil
	}
	lock, err := lockFile(paths.lock)
	if err != nil {
		return err
	}
	defer unlockFile(lock)
	if state := readSimulatorState(paths.state); state.UDID != "" {
		removeSimulator(state.UDID)
	}
	if err := os.Remove(paths.state); err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	return nil
}

// runCleanup deletes the simulators of the worktree at root, and of
// worktrees that are gone, and the temp folders of runs that were killed
// (rundir.go), for when a worktree is put away (`mise cleanup`). It leaves
// the fixture cache, which worktrees share.
func runCleanup(ctx context.Context, root string) error {
	for _, purpose := range worktreePurposes {
		err := discardWorktreeSimulator(root, purpose)
		if errors.Is(err, errSimulatorBusy) {
			fmt.Fprintf(os.Stderr, "appharness: a run is using this worktree's %s simulator; left it\n", purpose)
			continue
		}
		if err != nil {
			return err
		}
	}
	dir, err := simulatorStateDir()
	if err != nil {
		return err
	}
	pruneWorktreeSimulators(ctx, dir, "")
	sweepLeftoverRunDirs()
	return nil
}

// pruneWorktreeSimulators deletes the simulators of worktrees that are gone,
// and shuts down those unused for idleSimulatorShutdown, skipping any a run
// holds. except is the state file of the caller's own.
func pruneWorktreeSimulators(ctx context.Context, dir, except string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		statePath := filepath.Join(dir, entry.Name())
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || statePath == except || !prunable(statePath) {
			continue
		}
		lock, err := lockFile(filepath.Join(dir, id+".lock"))
		if err != nil {
			continue
		}
		// Checked again under the lock: a run may have used it meanwhile.
		if prunable(statePath) {
			state := readSimulatorState(statePath)
			if worktreeGone(state.Root) {
				removeSimulator(state.UDID)
				_ = os.Remove(statePath)
			} else {
				_, _ = simctl(ctx, "shutdown", state.UDID)
			}
		}
		unlockFile(lock)
	}
	removeOrphanSimulators(ctx, dir)
}

// removeOrphanSimulators deletes worktree simulators no state file names,
// such as one whose deletion failed after its state file went. Each is
// checked under its worktree's lock, which a run holds from cloning a
// simulator until its state file names it.
func removeOrphanSimulators(ctx context.Context, dir string) {
	devices, err := listDevices(ctx)
	if err != nil {
		return
	}
	for udid, device := range devices {
		purposeAndHash, ok := strings.CutPrefix(device.Name, worktreeSimulatorPrefix+" ")
		if !ok {
			continue
		}
		id := strings.ReplaceAll(purposeAndHash, " ", "-")
		lock, err := lockFile(filepath.Join(dir, id+".lock"))
		if err != nil {
			continue
		}
		if readSimulatorState(filepath.Join(dir, id+".json")).UDID != udid {
			removeSimulator(udid)
		}
		unlockFile(lock)
	}
}

// prunable says whether the simulator statePath describes belongs to a
// worktree that's gone, or has gone unused for idleSimulatorShutdown.
func prunable(statePath string) bool {
	info, err := os.Stat(statePath)
	if err != nil {
		return false
	}
	state := readSimulatorState(statePath)
	return state.UDID != "" && (worktreeGone(state.Root) || time.Since(info.ModTime()) >= idleSimulatorShutdown)
}

func worktreeGone(root string) bool {
	_, err := os.Stat(root)
	return os.IsNotExist(err)
}

// listDevices returns every simulator by UDID.
func listDevices(ctx context.Context) (map[string]simctlDevice, error) {
	out, err := simctl(ctx, "list", "devices", "-j")
	if err != nil {
		return nil, err
	}
	var listing struct {
		Devices map[string][]simctlDevice `json:"devices"`
	}
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		return nil, errors.WithStack(err)
	}
	byUDID := map[string]simctlDevice{}
	for _, devices := range listing.Devices {
		for _, d := range devices {
			byUDID[d.UDID] = d
		}
	}
	return byUDID, nil
}

// removeSimulator shuts a simulator down and deletes it, even after the run
// was interrupted.
func removeSimulator(udid string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, _ = simctl(ctx, "shutdown", udid)
	_, _ = simctl(ctx, "delete", udid)
}
