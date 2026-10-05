package worker

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/robinjoseph08/golib/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file swap the global logger output to assert on warnings,
// so none of them may run in parallel.

// newLogCapturingMonitor returns a monitor whose logger writes to the returned
// buffer.
func newLogCapturingMonitor(t *testing.T) (*Monitor, *bytes.Buffer) {
	t.Helper()
	t.Setenv("LOG_LEVEL", "")
	original := logger.Output()
	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	t.Cleanup(func() { logger.SetOutput(original) })
	return newTestMonitor(t), buf
}

// fakeDirWatcher models fsnotify's kqueue backend. A path's first Add
// registers the directory and then "lists" it, returning the next queued error
// for that path if there is one. The directory stays registered even when the
// listing fails, and an Add on a registered path succeeds without listing
// again, so only Remove followed by Add produces a fresh listing. A path in
// removeOnAdd is deleted from disk before its error is returned, simulating a
// directory removed mid-walk.
type fakeDirWatcher struct {
	errs        map[string][]error
	removeOnAdd map[string]bool
	calls       map[string]int
	registered  map[string]bool
	listed      []string
}

func newFakeDirWatcher() *fakeDirWatcher {
	return &fakeDirWatcher{
		errs:        map[string][]error{},
		removeOnAdd: map[string]bool{},
		calls:       map[string]int{},
		registered:  map[string]bool{},
	}
}

func (f *fakeDirWatcher) Add(path string) error {
	f.calls[path]++
	if f.registered[path] {
		return nil
	}
	f.registered[path] = true
	if f.removeOnAdd[path] {
		_ = os.RemoveAll(path)
	}
	if queued := f.errs[path]; len(queued) > 0 {
		f.errs[path] = queued[1:]
		return queued[0]
	}
	f.listed = append(f.listed, path)
	return nil
}

func (f *fakeDirWatcher) Remove(path string) error {
	if !f.registered[path] {
		return fsnotify.ErrNonExistentWatch
	}
	delete(f.registered, path)
	return nil
}

// vanishedFileErr mirrors the error kqueue's Add returns when a file in the
// directory disappears between listing it and reading its info.
func vanishedFileErr(dir string) error {
	return &fs.PathError{
		Op:   "lstat",
		Path: filepath.Join(dir, "Old Name.metadata.json"),
		Err:  fs.ErrNotExist,
	}
}

func makeWatchTree(t *testing.T) (root, sub1, sub2 string) {
	t.Helper()
	root = t.TempDir()
	sub1 = filepath.Join(root, "sub1")
	sub2 = filepath.Join(root, "sub2")
	require.NoError(t, os.MkdirAll(sub1, 0755))
	require.NoError(t, os.MkdirAll(sub2, 0755))
	return root, sub1, sub2
}

func TestMonitor_WatchRecursive_RetriesAfterVanishedFile(t *testing.T) {
	root, sub1, sub2 := makeWatchTree(t)
	m, logs := newLogCapturingMonitor(t)

	watcher := newFakeDirWatcher()
	watcher.errs[sub1] = []error{vanishedFileErr(sub1)}

	count, err := m.watchRecursive(watcher, root)
	require.NoError(t, err)

	assert.Equal(t, 3, count)
	assert.Equal(t, 2, watcher.calls[sub1])
	assert.ElementsMatch(t, []string{root, sub1, sub2}, watcher.listed)
	assert.NotContains(t, logs.String(), "failed to watch directory")
}

func TestMonitor_WatchRecursive_WarnsOnceWhenRetryFails(t *testing.T) {
	root, sub1, sub2 := makeWatchTree(t)
	m, logs := newLogCapturingMonitor(t)

	watcher := newFakeDirWatcher()
	watcher.errs[sub1] = []error{vanishedFileErr(sub1), vanishedFileErr(sub1), vanishedFileErr(sub1)}

	count, err := m.watchRecursive(watcher, root)
	require.NoError(t, err)

	assert.Equal(t, 2, count)
	assert.Equal(t, 2, watcher.calls[sub1])
	assert.ElementsMatch(t, []string{root, sub2}, watcher.listed)
	assert.Equal(t, 1, strings.Count(logs.String(), "failed to watch directory"))
}

func TestMonitor_WatchRecursive_DoesNotRetryOtherErrors(t *testing.T) {
	root, sub1, sub2 := makeWatchTree(t)
	m, logs := newLogCapturingMonitor(t)

	watcher := newFakeDirWatcher()
	watcher.errs[sub1] = []error{&fs.PathError{Op: "open", Path: sub1, Err: fs.ErrPermission}}

	count, err := m.watchRecursive(watcher, root)
	require.NoError(t, err)

	assert.Equal(t, 2, count)
	assert.Equal(t, 1, watcher.calls[sub1])
	assert.ElementsMatch(t, []string{root, sub2}, watcher.listed)
	assert.Equal(t, 1, strings.Count(logs.String(), "failed to watch directory"))
}

func TestMonitor_WatchRecursive_SkipsRemovedDirectory(t *testing.T) {
	root, sub1, sub2 := makeWatchTree(t)
	m, logs := newLogCapturingMonitor(t)

	watcher := newFakeDirWatcher()
	watcher.removeOnAdd[sub1] = true
	watcher.errs[sub1] = []error{&fs.PathError{Op: "open", Path: sub1, Err: fs.ErrNotExist}}

	count, err := m.watchRecursive(watcher, root)
	require.NoError(t, err)

	assert.Equal(t, 2, count)
	assert.Equal(t, 1, watcher.calls[sub1])
	assert.ElementsMatch(t, []string{root, sub2}, watcher.listed)
	assert.NotContains(t, logs.String(), "failed to watch directory")
}
