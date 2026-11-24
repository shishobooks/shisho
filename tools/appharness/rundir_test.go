package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// killedRunDir makes a run's folder as a run killed outright leaves it: its
// lock file there, held by no one.
func killedRunDir(t *testing.T, tmp, prefix string) string {
	t.Helper()
	run, err := makeRunDir(tmp, prefix, false)
	require.NoError(t, err)
	unlockFile(run.lock)
	return run.Path
}

func noProcesses() ([]process, error) { return nil, nil }

func noKills(t *testing.T) func(int) {
	return func(pid int) { t.Fatalf("killed process %d", pid) }
}

func TestSweepRunDirsKeepsARunThatsStillGoing(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	run, err := makeRunDir(tmp, flowsRunPrefix, false)
	require.NoError(t, err)
	defer run.Close()

	removed := sweepRunDirs(tmp, noProcesses, noKills(t))

	assert.Zero(t, removed)
	assert.DirExists(t, run.Path)
}

func TestSweepRunDirsRemovesAKilledRunAndItsOrphanedServer(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := killedRunDir(t, tmp, qaServerRunPrefix)

	var killed []int
	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return []process{
			{PID: 41, PPID: 1, Command: filepath.Join(dir, "shisho")},
			{PID: 42, PPID: 1, Command: filepath.Join(tmp, "elsewhere", "shisho")},
			{PID: 43, PPID: 1, Command: dir + "0/shisho"},
		}, nil
	}, func(pid int) { killed = append(killed, pid) })

	assert.Equal(t, 1, removed)
	assert.NoDirExists(t, dir)
	assert.Equal(t, []int{41}, killed)
}

func TestSweepRunDirsListsProcessesAgainForEachFolder(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	killedRunDir(t, tmp, flowsRunPrefix)
	killedRunDir(t, tmp, qaServerRunPrefix)

	// Not one list for the whole sweep: removing a folder can take seconds,
	// time enough for a listed PID to go to another process.
	lists := 0
	removed := sweepRunDirs(tmp, func() ([]process, error) {
		lists++
		return nil, nil
	}, noKills(t))

	assert.Equal(t, 2, removed)
	assert.Equal(t, 2, lists)
}

func TestSweepRunDirsKeepsAKilledRunsFolderWhileALiveProcessRunsFromIt(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := killedRunDir(t, tmp, flowsRunPrefix)

	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return []process{
			{PID: 50, PPID: 1, Command: filepath.Join(dir, "shisho")},
			{PID: 51, PPID: 49, Command: filepath.Join(dir, "tool")},
		}, nil
	}, noKills(t))

	assert.Zero(t, removed)
	assert.DirExists(t, dir)
}

func TestSweepRunDirsKeepsAKilledRunsFolderWhenProcessesCantBeListed(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := killedRunDir(t, tmp, flowsRunPrefix)

	removed := sweepRunDirs(tmp, func() ([]process, error) {
		return nil, os.ErrPermission
	}, noKills(t))

	assert.Zero(t, removed)
	assert.DirExists(t, dir)
}

func TestSweepRunDirsKeepsTheLockUntilTheFolderIsGone(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	dir := killedRunDir(t, tmp, flowsRunPrefix)
	stuck := filepath.Join(dir, "maestro")
	require.NoError(t, os.MkdirAll(stuck, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stuck, "screenshot.png"), []byte("x"), 0o600))
	require.NoError(t, os.Chmod(stuck, 0o500))
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o755) })

	assert.Zero(t, sweepRunDirs(tmp, noProcesses, noKills(t)))
	assert.FileExists(t, filepath.Join(dir, runDirLock), "a later sweep finds the folder by its lock")

	require.NoError(t, os.Chmod(stuck, 0o755))
	assert.Equal(t, 1, sweepRunDirs(tmp, noProcesses, noKills(t)))
	assert.NoDirExists(t, dir)
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
	assert.Zero(t, sweepRunDirs(tmp, noProcesses, noKills(t)))
	assert.DirExists(t, kept.Path)
}

func TestSweepRunDirsLeavesFoldersWithoutALock(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Folders a harness from before the lock made, which may be a live run,
	// one kept with -keep, or one named with -out; and folders that aren't
	// runs at all.
	var dirs []string
	for _, name := range []string{"shisho-harness-123", "shisho-qa-server-456", "shisho-harness-notes", "shisho-harness-", "something-123"} {
		dir := filepath.Join(tmp, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(dir, old, old))
		dirs = append(dirs, dir)
	}

	assert.Zero(t, sweepRunDirs(tmp, noProcesses, noKills(t)))
	for _, dir := range dirs {
		assert.DirExists(t, dir)
	}
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
