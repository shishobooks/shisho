package main

import (
	"context"
	"fmt"
	"io/fs"
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

// legacyRunDirIdle is how long a run folder without runDirLock, made by a
// harness from before the lock, must go unchanged before sweepRunDirs
// removes it. Worktrees on an older commit still make those.
const legacyRunDirIdle = time.Hour

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
		return &runDir{Path: dir, keep: true}, errors.WithStack(err)
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

// sweepRunDirs removes the folders in tmp of runs that have ended without
// removing them, and returns how many it removed. A run's server is in a
// process group of its own, so it outlives a run killed outright, holding
// the folder's files on disk after they're removed; any process running from
// a removed folder is killed with kill.
//
// A folder whose runDirLock no one holds is left over. One without the lock
// file, made by an older harness, is left over once nothing in it has
// changed for legacyRunDirIdle and no process runs from it but orphans,
// whose parent run is gone. An idle QA server changes nothing for hours, so
// the age alone isn't enough; without the list of processes, none of those
// is removed.
func sweepRunDirs(tmp string, processes func() ([]process, error), kill func(pid int)) int {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return 0
	}
	var listed []process
	var listErr error
	listOnce := false
	list := func() ([]process, error) {
		if !listOnce {
			listed, listErr = processes()
			listOnce = true
		}
		return listed, listErr
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || !isRunDirName(entry.Name()) {
			continue
		}
		dir := filepath.Join(tmp, entry.Name())
		lock, err := os.OpenFile(filepath.Join(dir, runDirLock), os.O_RDWR, 0) //nolint:gosec // A run folder of the harness's own.
		switch {
		case err == nil:
			if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
				_ = lock.Close()
				continue
			}
			running, _ := list()
			removeRunDir(dir, running, kill)
			unlockFile(lock)
			removed++
		case os.IsNotExist(err):
			if changedSince(dir, time.Now().Add(-legacyRunDirIdle)) {
				continue
			}
			running, err := list()
			if err != nil || runDirHasLiveRun(dir, running) {
				continue
			}
			removeRunDir(dir, running, kill)
			removed++
		}
	}
	return removed
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

// runDirHasLiveRun says whether a process runs from dir whose parent, the
// run, is still alive. An orphan's parent is launchd, PID 1.
func runDirHasLiveRun(dir string, running []process) bool {
	for _, p := range running {
		if runsFrom(p, dir) && p.PPID != 1 {
			return true
		}
	}
	return false
}

func removeRunDir(dir string, running []process, kill func(pid int)) {
	for _, p := range running {
		if runsFrom(p, dir) {
			kill(p.PID)
		}
	}
	_ = os.RemoveAll(dir)
}

// changedSince says whether anything in dir changed after since.
func changedSince(dir string, since time.Time) bool {
	changed := false
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // A file gone mid-walk changed nothing.
		}
		if info, err := entry.Info(); err == nil && info.ModTime().After(since) {
			changed = true
			return filepath.SkipAll
		}
		return nil
	})
	return changed
}
