package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

// The temp folders a run keeps its server, fixtures, and Maestro output in.
// os.MkdirTemp follows each prefix with digits only.
const (
	flowsRunPrefix    = "shisho-harness-"
	qaServerRunPrefix = "shisho-qa-server-"
	// keptRunPrefix names the folder of a run that keeps it (-keep), which
	// sweepRunDirs never removes: its name isn't a prefix and digits.
	keptRunPrefix = "shisho-harness-kept-"
)

// runDirLock is the file in a run's folder whose lock the run holds until it
// ends. A run killed outright (SIGKILL, such as an agent's tool timeout)
// skips its deferred cleanup, but the lock ends with the process, which is
// how sweepRunDirs knows the folder is left over.
const runDirLock = ".appharness.lock"

// runDir is a run's temp folder. Close it when the run ends.
type runDir struct {
	Path string
	keep bool
	lock *os.File
}

// makeRunDir makes a run's folder in tmp, locked until Close, which removes
// it unless keep says to leave it for debugging.
func makeRunDir(tmp, prefix string, keep bool) (*runDir, error) {
	if keep {
		dir, err := os.MkdirTemp(tmp, keptRunPrefix)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return &runDir{Path: dir, keep: true}, nil
	}
	dir, err := os.MkdirTemp(tmp, prefix)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Locked under another name first, so a sweep never finds runDirLock
	// unlocked while the run lives. The lock follows the file's rename.
	pending := filepath.Join(dir, runDirLock+".new")
	lock, err := lockFile(pending)
	if err == nil {
		err = errors.WithStack(os.Rename(pending, filepath.Join(dir, runDirLock)))
	}
	if err != nil {
		if lock != nil {
			unlockFile(lock)
		}
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &runDir{Path: dir, lock: lock}, nil
}

// Close removes the folder, unless it's kept, and lets go of its lock.
func (r *runDir) Close() {
	if !r.keep {
		_ = os.RemoveAll(r.Path)
	}
	if r.lock != nil {
		unlockFile(r.lock)
	}
}

// process is a running process, from ps.
type process struct {
	PID, PPID int
	Command   string
}

// listProcesses lists every process with its command line.
func listProcesses() ([]process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, errors.Wrap(err, "list processes")
	}
	return parseProcesses(string(out)), nil
}

func parseProcesses(out string) []process {
	var processes []process
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		// The command keeps its own spacing, as a path with a space in it
		// needs.
		command := strings.TrimSpace(line)
		for range 2 {
			command = strings.TrimSpace(strings.TrimLeft(command, "0123456789"))
		}
		processes = append(processes, process{PID: pid, PPID: ppid, Command: command})
	}
	return processes
}

func killProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// sweepLeftoverRunDirs removes the run folders in the temp folder that runs
// killed outright left behind, saying how many it removed.
func sweepLeftoverRunDirs() {
	if removed := sweepRunDirs(os.TempDir(), listProcesses, killProcess); removed > 0 {
		fmt.Fprintf(os.Stderr, "appharness: removed %d folders left in %s by runs that were killed\n", removed, os.TempDir())
	}
}

// sweepRunDirs removes the folders in tmp of runs that ended without
// removing them, and returns how many it removed. A folder is left over when
// it has a runDirLock no one holds. One without the lock file is never
// removed: a harness from before the lock made it, and nothing shows that
// its run is over or that it wasn't kept with -keep.
//
// A run's server is in a process group of its own, so it outlives a run
// killed outright, holding the folder's files on disk after they're removed.
// An orphan running from the folder, whose parent is launchd (PID 1), is
// killed with kill. A folder another process still runs from is left, and so
// is one whose processes can't be listed, so a later sweep can try again.
func sweepRunDirs(tmp string, processes func() ([]process, error), kill func(pid int)) int {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() && isRunDirName(entry.Name()) && sweepRunDir(filepath.Join(tmp, entry.Name()), processes, kill) {
			removed++
		}
	}
	return removed
}

// sweepRunDir removes dir if its run is over, and says whether it did.
func sweepRunDir(dir string, processes func() ([]process, error), kill func(pid int)) bool {
	lock, err := os.OpenFile(filepath.Join(dir, runDirLock), os.O_RDWR, 0) //nolint:gosec // A run folder of the harness's own.
	if err != nil {
		return false
	}
	defer unlockFile(lock)
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	// Listed for each folder, just before the kill: removing a folder can
	// take seconds, time enough for a PID to go to another process.
	running, err := processes()
	if err != nil {
		return false
	}
	var orphans []int
	for _, p := range running {
		if !runsFrom(p, dir) {
			continue
		}
		if p.PPID != 1 {
			return false
		}
		orphans = append(orphans, p.PID)
	}
	for _, pid := range orphans {
		kill(pid)
	}
	_ = os.RemoveAll(dir)
	return true
}

// isRunDirName says whether name is a run folder's: a run prefix followed by
// os.MkdirTemp's digits.
func isRunDirName(name string) bool {
	for _, prefix := range []string{flowsRunPrefix, qaServerRunPrefix} {
		if digits, ok := strings.CutPrefix(name, prefix); ok && digits != "" && strings.Trim(digits, "0123456789") == "" {
			return true
		}
	}
	return false
}

// runsFrom says whether p's executable is in dir, as a run's server is.
func runsFrom(p process, dir string) bool {
	return strings.HasPrefix(p.Command, dir+string(filepath.Separator))
}
