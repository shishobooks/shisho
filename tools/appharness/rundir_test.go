package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staleRunDir makes a run folder like one an older harness, which didn't
// lock its folders, left behind, last changed age ago.
func staleRunDir(t *testing.T, tmp, name string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(tmp, name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "library"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "library", "book.epub"), []byte("x"), 0o600))
	then := time.Now().Add(-age)
	require.NoError(t, filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(path, then, then)
	}))
	return dir
}

func noProcesses() ([]process, error) { return nil, nil }

func TestSweepRunDirsKeepsARunThatsStillGoing(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	run, err := makeRunDir(tmp, flowsRunPrefix, false)
	require.NoError(t, err)
	defer run.Close()

	removed := sweepRunDirs(tmp, noProcesses, func(int) { t.Fatal("killed a process") })

	assert.Zero(t, removed)
	assert.DirExists(t, run.Path)
}

func TestSweepRunDirsRemovesAKilledRunAndItsServer(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	run, err := makeRunDir(tmp, qaServerRunPrefix, false)
	require.NoError(t, err)
	// A hard kill lets go of the lock without removing anything.
	unlockFile(run.lock)
	other := filepath.Join(tmp, "elsewhere", "shisho")

	var killed []int
	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return []process{
			{PID: 41, PPID: 1, Command: filepath.Join(run.Path, "shisho")},
			{PID: 42, PPID: 1, Command: other},
		}, nil
	}, func(pid int) { killed = append(killed, pid) })

	assert.Equal(t, 1, removed)
	assert.NoDirExists(t, run.Path)
	assert.Equal(t, []int{41}, killed)
}

func TestRunDirCloseRemovesItUnlessKept(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	run, err := makeRunDir(tmp, flowsRunPrefix, false)
	require.NoError(t, err)
	kept, err := makeRunDir(tmp, flowsRunPrefix, true)
	require.NoError(t, err)

	run.Close()
	kept.Close()

	assert.NoDirExists(t, run.Path)
	assert.DirExists(t, kept.Path)
	// Nor does a sweep remove a kept one, which the developer asked for.
	assert.Zero(t, sweepRunDirs(tmp, noProcesses, func(int) {}))
	assert.DirExists(t, kept.Path)
}

func TestSweepRunDirsLeavesFoldersThatArentRuns(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	var dirs []string
	for _, name := range []string{"shisho-harness-notes", "shisho-harness-", "something-123", "shisho-qa-server-12a"} {
		dirs = append(dirs, staleRunDir(t, tmp, name, 48*time.Hour))
	}

	assert.Zero(t, sweepRunDirs(tmp, noProcesses, func(int) {}))
	for _, dir := range dirs {
		assert.DirExists(t, dir)
	}
}

func TestSweepRunDirsRemovesAnOlderHarnessesFolderOnceIdle(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	idle := staleRunDir(t, tmp, "shisho-harness-123", 2*legacyRunDirIdle)
	recent := staleRunDir(t, tmp, "shisho-qa-server-456", legacyRunDirIdle/2)

	removed := sweepRunDirs(tmp, noProcesses, func(int) { t.Fatal("killed a process") })

	assert.Equal(t, 1, removed)
	assert.NoDirExists(t, idle)
	assert.DirExists(t, recent)
}

func TestSweepRunDirsKeepsAnOlderHarnessesFolderWhileItsServerHasItsRun(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// An idle QA server writes nothing for hours, but its harness is alive.
	live := staleRunDir(t, tmp, "shisho-qa-server-123", 2*legacyRunDirIdle)
	orphaned := staleRunDir(t, tmp, "shisho-harness-456", 2*legacyRunDirIdle)

	var killed []int
	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return []process{
			{PID: 50, PPID: 49, Command: filepath.Join(live, "shisho")},
			{PID: 60, PPID: 1, Command: filepath.Join(orphaned, "shisho")},
		}, nil
	}, func(pid int) { killed = append(killed, pid) })

	assert.Equal(t, 1, removed)
	assert.DirExists(t, live)
	assert.NoDirExists(t, orphaned)
	assert.Equal(t, []int{60}, killed)
}

func TestSweepRunDirsRemovesNothingOfAnOlderHarnessWhenProcessesCantBeListed(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	idle := staleRunDir(t, tmp, "shisho-harness-123", 2*legacyRunDirIdle)

	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return nil, os.ErrPermission
	}, func(int) { t.Fatal("killed a process") })

	assert.Zero(t, removed)
	assert.DirExists(t, idle)
}

func TestParseProcesses(t *testing.T) {
	t.Parallel()
	out := "    1     0 /sbin/launchd\n" +
		"27955  27471 /var/folders/9q/T/shisho-harness-572043948/shisho\n" +
		"  300     1 /Volumes/U34 Bolt/bin/tool -flag\n" +
		"garbage\n"

	assert.Equal(t, []process{
		{PID: 1, PPID: 0, Command: "/sbin/launchd"},
		{PID: 27955, PPID: 27471, Command: "/var/folders/9q/T/shisho-harness-572043948/shisho"},
		{PID: 300, PPID: 1, Command: "/Volumes/U34 Bolt/bin/tool -flag"},
	}, parseProcesses(out))
}
