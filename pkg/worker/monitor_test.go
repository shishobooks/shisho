package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMonitor(t *testing.T) *Monitor {
	t.Helper()
	w := &Worker{
		config: &config.Config{
			LibraryMonitorDelaySeconds: minMonitorDelaySeconds,
		},
		log: logger.New(),
	}
	m := newMonitor(w)
	m.pathToLibrary["/library/books"] = 1
	m.pathToLibrary["/library/audiobooks"] = 2
	return m
}

func TestMonitor_DelayClampsToMinimum(t *testing.T) {
	t.Parallel()

	w := &Worker{
		config: &config.Config{LibraryMonitorDelaySeconds: 0},
		log:    logger.New(),
	}
	m := newMonitor(w)
	assert.Equal(t, time.Duration(minMonitorDelaySeconds)*time.Second, m.delay)

	w2 := &Worker{
		config: &config.Config{LibraryMonitorDelaySeconds: 120},
		log:    logger.New(),
	}
	m2 := newMonitor(w2)
	assert.Equal(t, 120*time.Second, m2.delay)
}

func TestMonitor_FindLibraryID(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"file in books library", "/library/books/author/book.epub", 1},
		{"file in audiobooks library", "/library/audiobooks/author/book.m4b", 2},
		{"file not in any library", "/other/path/book.epub", 0},
		{"exact library path", "/library/books", 1},
		{"partial prefix that doesn't match", "/library/bookstore/item.epub", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, m.findLibraryID(tt.path))
		})
	}
}

func TestMonitor_IsScannable(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"epub file", "/lib/book.epub", true},
		{"m4b file", "/lib/book.m4b", true},
		{"cbz file", "/lib/comic.cbz", true},
		{"jpg file", "/lib/cover.jpg", false},
		{"json file", "/lib/meta.json", false},
		{"no extension", "/lib/README", false},
		{"uppercase epub", "/lib/book.EPUB", true},
		{"txt file", "/lib/notes.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, m.isScannable(tt.path))
		})
	}
}

func TestMonitor_IgnorePath(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	path := "/library/books/cover.jpg"

	// Initially not ignored.
	assert.False(t, m.isIgnored(path))

	// After adding, should be ignored.
	m.IgnorePath(path)
	assert.True(t, m.isIgnored(path))

	// Different path should not be ignored.
	assert.False(t, m.isIgnored("/library/books/other.jpg"))
}

func TestMonitor_IgnorePath_Expiry(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	path := "/library/books/cover.jpg"

	// Manually insert an already-expired entry.
	m.ignoreMu.Lock()
	m.ignored[path] = time.Now().Add(-time.Second)
	m.ignoreMu.Unlock()

	// Expired entries should not be considered ignored.
	assert.False(t, m.isIgnored(path))
}

func TestMonitor_CleanupIgnored(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	// Add some entries.
	m.ignoreMu.Lock()
	m.ignored["/a"] = time.Now().Add(-time.Hour) // already expired
	m.ignored["/b"] = time.Now().Add(time.Hour)  // still valid
	m.ignoreMu.Unlock()

	m.cleanupIgnored()

	m.ignoreMu.RLock()
	defer m.ignoreMu.RUnlock()
	assert.NotContains(t, m.ignored, "/a")
	assert.Contains(t, m.ignored, "/b")
}

func TestMonitor_HandleEvent_SkipsShishoSpecialFiles(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	// Cover files should be skipped.
	m.handleEvent(nil, fsnotify.Event{
		Name: "/library/books/book.epub.cover.jpg",
		Op:   fsnotify.Create,
	})
	assert.Empty(t, m.pending)

	// Metadata files should be skipped.
	m.handleEvent(nil, fsnotify.Event{
		Name: "/library/books/book.epub.metadata.json",
		Op:   fsnotify.Create,
	})
	assert.Empty(t, m.pending)
}

func TestMonitor_HandleEvent_SkipsNonScannableFiles(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	m.handleEvent(nil, fsnotify.Event{
		Name: "/library/books/readme.txt",
		Op:   fsnotify.Create,
	})
	assert.Empty(t, m.pending)
}

func TestMonitor_HandleEvent_SkipsIgnoredPaths(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	path := "/library/books/book.epub"
	m.IgnorePath(path)

	m.handleEvent(nil, fsnotify.Event{
		Name: path,
		Op:   fsnotify.Create,
	})
	assert.Empty(t, m.pending)
}

func TestMonitor_HandleEvent_AccumulatesOps(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)
	// Stop the debounce timer so it doesn't fire during the test.
	defer func() {
		m.mu.Lock()
		if m.timer != nil {
			m.timer.Stop()
		}
		m.mu.Unlock()
	}()

	path := "/library/books/book.epub"

	// Simulate Create event.
	m.handleEvent(nil, fsnotify.Event{Name: path, Op: fsnotify.Create})
	m.mu.Lock()
	assert.Equal(t, fsnotify.Create, m.pending[path].Op)
	m.mu.Unlock()

	// Simulate Write event for same path — ops should be merged.
	m.handleEvent(nil, fsnotify.Event{Name: path, Op: fsnotify.Write})
	m.mu.Lock()
	assert.Equal(t, fsnotify.Create|fsnotify.Write, m.pending[path].Op)
	assert.Equal(t, 1, m.pending[path].LibraryID)
	m.mu.Unlock()
}

func TestMonitor_HandleEvent_SkipsPathsOutsideLibrary(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	m.handleEvent(nil, fsnotify.Event{
		Name: "/other/path/book.epub",
		Op:   fsnotify.Create,
	})
	assert.Empty(t, m.pending)
}

func TestMonitor_WatchRecursive(t *testing.T) {
	t.Parallel()

	// Create a temp directory structure.
	root := t.TempDir()
	sub1 := filepath.Join(root, "sub1")
	sub2 := filepath.Join(root, "sub1", "sub2")
	require.NoError(t, os.MkdirAll(sub2, 0755))

	m := newTestMonitor(t)
	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()

	count, err := m.watchRecursive(watcher, root)
	require.NoError(t, err)
	assert.Equal(t, 3, count) // root, sub1, sub2

	// Verify the watcher list includes all three directories.
	watchList := watcher.WatchList()
	assert.Contains(t, watchList, root)
	assert.Contains(t, watchList, sub1)
	assert.Contains(t, watchList, sub2)
}

func TestMonitor_EnqueueExistingFiles(t *testing.T) {
	t.Parallel()

	// Create a directory with files already in it (simulating the race condition
	// where files exist before the watch is added).
	root := t.TempDir()
	sub := filepath.Join(root, "author")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "book.epub"), []byte("test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "cover.jpg"), []byte("test"), 0644))        // not scannable
	require.NoError(t, os.WriteFile(filepath.Join(sub, "book.epub.cover.jpg"), []byte("t"), 0644)) // shisho special

	m := newTestMonitor(t)
	m.pathToLibrary[root] = 1

	m.enqueueExistingFiles(root)

	// Stop the debounce timer so it doesn't fire after the test.
	m.mu.Lock()
	if m.timer != nil {
		m.timer.Stop()
	}

	// Only the epub should be enqueued.
	assert.Len(t, m.pending, 1)
	ep, ok := m.pending[filepath.Join(sub, "book.epub")]
	assert.True(t, ok)
	assert.Equal(t, fsnotify.Create, ep.Op)
	assert.Equal(t, 1, ep.LibraryID)
	m.mu.Unlock()
}

func TestMonitor_HandleEvent_NewDirEnqueuesExistingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "new-author")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "book.epub"), []byte("test"), 0644))

	m := newTestMonitor(t)
	m.pathToLibrary[root] = 1

	watcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer watcher.Close()
	_, err = m.watchRecursive(watcher, root)
	require.NoError(t, err)

	// Stop the debounce timer so it doesn't fire during the test.
	defer func() {
		m.mu.Lock()
		if m.timer != nil {
			m.timer.Stop()
		}
		m.mu.Unlock()
	}()

	// Simulate a Create event for the new directory.
	m.handleEvent(watcher, fsnotify.Event{Name: sub, Op: fsnotify.Create})

	m.mu.Lock()
	defer m.mu.Unlock()

	// The epub inside the new directory should have been enqueued.
	assert.Len(t, m.pending, 1)
	_, ok := m.pending[filepath.Join(sub, "book.epub")]
	assert.True(t, ok)
}

func TestMonitor_ProcessEvent_CreateSkipsNonexistentFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Try to process a create event for a file that doesn't exist on disk.
	// processEvent should return nil (skip) since the file isn't there.
	result := m.processEvent(tc.ctx, filepath.Join(libDir, "nonexistent.epub"), pendingEvent{
		Op:        fsnotify.Create,
		LibraryID: 1,
	})
	assert.Nil(t, result)
}

func TestMonitor_ProcessEvent_DeleteSkipsUnknownFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Try to process a delete event for a file that was never in the DB.
	result := m.processEvent(tc.ctx, filepath.Join(libDir, "never-existed.epub"), pendingEvent{
		Op:        fsnotify.Remove,
		LibraryID: 1,
	})
	assert.Nil(t, result)
}

func TestMonitor_ProcessEvent_CreateScansNewFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create a real EPUB file.
	bookDir := testgen.CreateSubDir(t, libDir, "Test Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Monitor Test Book",
		Authors: []string{"Test Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Process a Create event — should scan the file and create a book.
	result := m.processEvent(tc.ctx, epubPath, pendingEvent{
		Op:        fsnotify.Create,
		LibraryID: 1,
	})
	require.NotNil(t, result)
	assert.True(t, result.FileCreated)
	assert.NotNil(t, result.Book)
	assert.Equal(t, "Monitor Test Book", result.Book.Title)

	// Verify the file exists in the database.
	files := tc.listFiles()
	assert.Len(t, files, 1)
	assert.Equal(t, epubPath, files[0].Filepath)
}

func TestMonitor_ProcessEvent_WriteRescansExistingFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create and scan a file first.
	bookDir := testgen.CreateSubDir(t, libDir, "Test Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Original Title",
		Authors: []string{"Test Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Initial scan via Create.
	result := m.processEvent(tc.ctx, epubPath, pendingEvent{
		Op:        fsnotify.Create,
		LibraryID: 1,
	})
	require.NotNil(t, result)

	// Verify the file is in DB.
	file, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{
		Filepath: &epubPath,
	})
	require.NoError(t, err)
	require.NotNil(t, file)

	// Now process a Write event — should rescan with ForceRefresh.
	result = m.processEvent(tc.ctx, epubPath, pendingEvent{
		Op:        fsnotify.Write,
		LibraryID: 1,
	})
	require.NotNil(t, result)
	assert.NotNil(t, result.Book)
}

func TestMonitor_ProcessEvent_DeleteRemovesFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create and scan a file.
	bookDir := testgen.CreateSubDir(t, libDir, "Test Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Delete Me",
		Authors: []string{"Test Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Initial scan.
	result := m.processEvent(tc.ctx, epubPath, pendingEvent{
		Op:        fsnotify.Create,
		LibraryID: 1,
	})
	require.NotNil(t, result)
	assert.Len(t, tc.listFiles(), 1)

	// Remove the file from disk, then process a Remove event.
	require.NoError(t, os.Remove(epubPath))
	result = m.processEvent(tc.ctx, epubPath, pendingEvent{
		Op:        fsnotify.Remove,
		LibraryID: 1,
	})
	require.NotNil(t, result)
	assert.True(t, result.FileDeleted)

	// File and book should be cleaned up.
	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

func TestMonitor_SkipsWhenScanJobActive(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create an active scan job.
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusInProgress,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Add a pending event.
	m.pending[filepath.Join(libDir, "book.epub")] = pendingEvent{
		Op:        fsnotify.Create,
		LibraryID: 1,
	}

	// processPendingEvents should re-queue because a scan job is active.
	m.processPendingEvents()

	// Events should be re-queued (not dropped) and a timer should be set.
	m.mu.Lock()
	assert.Len(t, m.pending, 1)
	_, ok := m.pending[filepath.Join(libDir, "book.epub")]
	assert.True(t, ok, "event should be re-queued")
	assert.NotNil(t, m.timer, "timer should be restarted for re-queued events")
	m.timer.Stop()
	m.mu.Unlock()
}

func TestMonitor_HandleEvent_QueuesDirectoryRemoveAsDirectoryEvent(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)
	defer func() {
		m.mu.Lock()
		if m.timer != nil {
			m.timer.Stop()
		}
		m.mu.Unlock()
	}()

	dirPath := "/library/books/author-title"
	m.handleEvent(nil, fsnotify.Event{Name: dirPath, Op: fsnotify.Remove})

	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.pending[dirPath]
	require.True(t, ok, "directory remove event should be queued")
	assert.True(t, ev.IsDirectory)
	assert.Equal(t, fsnotify.Remove, ev.Op)
	assert.Equal(t, 1, ev.LibraryID)
}

func TestMonitor_HandleEvent_QueuesDirectoryRenameAsDirectoryEvent(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)
	defer func() {
		m.mu.Lock()
		if m.timer != nil {
			m.timer.Stop()
		}
		m.mu.Unlock()
	}()

	dirPath := "/library/books/old-name"
	m.handleEvent(nil, fsnotify.Event{Name: dirPath, Op: fsnotify.Rename})

	m.mu.Lock()
	defer m.mu.Unlock()
	ev, ok := m.pending[dirPath]
	require.True(t, ok, "directory rename event should be queued")
	assert.True(t, ev.IsDirectory)
	assert.Equal(t, fsnotify.Rename, ev.Op)
}

func TestMonitor_HandleEvent_IgnoresDirectoryRemoveOutsideLibrary(t *testing.T) {
	t.Parallel()
	m := newTestMonitor(t)

	m.handleEvent(nil, fsnotify.Event{Name: "/other/path/folder", Op: fsnotify.Remove})
	assert.Empty(t, m.pending)
}

func TestMonitor_ProcessEvent_DirectoryRemoveDeletesBookAndFile(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create and scan a book in its own directory.
	bookDir := testgen.CreateSubDir(t, libDir, "Author - Title")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Dir Delete Me",
		Authors: []string{"Test Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	// Seed the DB.
	result := m.processEvent(tc.ctx, epubPath, pendingEvent{Op: fsnotify.Create, LibraryID: 1})
	require.NotNil(t, result)
	require.Len(t, tc.listFiles(), 1)
	require.Len(t, tc.listBooks(), 1)

	// Remove the whole directory from disk, then fire a directory remove event.
	require.NoError(t, os.RemoveAll(bookDir))

	results := m.processDirectoryEvent(tc.ctx, bookDir, pendingEvent{
		Op:          fsnotify.Remove,
		LibraryID:   1,
		IsDirectory: true,
	})
	require.Len(t, results, 1)
	assert.True(t, results[0].FileDeleted)

	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

func TestMonitor_ProcessEvent_DirectoryRemoveCascadesToNestedBooks(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Two books in sibling subdirs under a common parent.
	parentDir := testgen.CreateSubDir(t, libDir, "Author")
	bookDirA := testgen.CreateSubDir(t, parentDir, "Title A")
	bookDirB := testgen.CreateSubDir(t, parentDir, "Title B")
	epubA := testgen.GenerateEPUB(t, bookDirA, "a.epub", testgen.EPUBOptions{
		Title: "A", Authors: []string{"Author"},
	})
	epubB := testgen.GenerateEPUB(t, bookDirB, "b.epub", testgen.EPUBOptions{
		Title: "B", Authors: []string{"Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	require.NotNil(t, m.processEvent(tc.ctx, epubA, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.NotNil(t, m.processEvent(tc.ctx, epubB, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.Len(t, tc.listFiles(), 2)
	require.Len(t, tc.listBooks(), 2)

	// Remove the entire parent subtree from disk.
	require.NoError(t, os.RemoveAll(parentDir))

	results := m.processDirectoryEvent(tc.ctx, parentDir, pendingEvent{
		Op:          fsnotify.Remove,
		LibraryID:   1,
		IsDirectory: true,
	})
	assert.Len(t, results, 2)

	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

func TestMonitor_ProcessEvent_DirectoryRemoveDoesNotMatchSiblingPrefix(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Two sibling directories whose names share a prefix: "Author" and "Author Two".
	// Removing "Author" must not cascade into "Author Two".
	keepDir := testgen.CreateSubDir(t, libDir, "Author Two")
	goDir := testgen.CreateSubDir(t, libDir, "Author")
	keepEpub := testgen.GenerateEPUB(t, keepDir, "keep.epub", testgen.EPUBOptions{
		Title: "Keep", Authors: []string{"Keep"},
	})
	goEpub := testgen.GenerateEPUB(t, goDir, "go.epub", testgen.EPUBOptions{
		Title: "Go", Authors: []string{"Go"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	require.NotNil(t, m.processEvent(tc.ctx, keepEpub, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.NotNil(t, m.processEvent(tc.ctx, goEpub, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.Len(t, tc.listBooks(), 2)

	// Only remove the "Author" subtree from disk; "Author Two" stays intact.
	require.NoError(t, os.RemoveAll(goDir))

	results := m.processDirectoryEvent(tc.ctx, goDir, pendingEvent{
		Op:          fsnotify.Remove,
		LibraryID:   1,
		IsDirectory: true,
	})
	assert.Len(t, results, 1, "only files under the removed directory should be touched")

	remaining := tc.listFiles()
	require.Len(t, remaining, 1)
	assert.Equal(t, keepEpub, remaining[0].Filepath)
	assert.Len(t, tc.listBooks(), 1)
}

func TestMonitor_ProcessEvent_DirectoryRenameCleansOldRows(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Seed a book in its folder.
	oldDir := testgen.CreateSubDir(t, libDir, "Old Name")
	epubPath := testgen.GenerateEPUB(t, oldDir, "book.epub", testgen.EPUBOptions{
		Title: "Renamed", Authors: []string{"Author"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	require.NotNil(t, m.processEvent(tc.ctx, epubPath, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.Len(t, tc.listBooks(), 1)

	// Simulate the directory being renamed away on disk (move it to a new name).
	newDir := filepath.Join(libDir, "New Name")
	require.NoError(t, os.Rename(oldDir, newDir))

	// Fire the Rename event for the old directory path. The file no longer
	// exists at the old path, so scanInternal should delete the old rows.
	results := m.processDirectoryEvent(tc.ctx, oldDir, pendingEvent{
		Op:          fsnotify.Rename,
		LibraryID:   1,
		IsDirectory: true,
	})
	require.Len(t, results, 1)
	assert.True(t, results[0].FileDeleted)

	// The stale book row is gone. (The new book row is created by the
	// separate Create event on the new path, which is tested elsewhere.)
	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
}

func TestMonitor_ProcessEvent_DirectoryRemoveWithMultipleFilesKeepsBookUntilLast(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Two files inside the same book folder.
	bookDir := testgen.CreateSubDir(t, libDir, "Multi")
	epub1 := testgen.GenerateEPUB(t, bookDir, "one.epub", testgen.EPUBOptions{
		Title: "Multi", Authors: []string{"Multi"},
	})
	epub2 := testgen.GenerateEPUB(t, bookDir, "two.epub", testgen.EPUBOptions{
		Title: "Multi", Authors: []string{"Multi"},
	})

	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds
	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = 1

	require.NotNil(t, m.processEvent(tc.ctx, epub1, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.NotNil(t, m.processEvent(tc.ctx, epub2, pendingEvent{Op: fsnotify.Create, LibraryID: 1}))
	require.Len(t, tc.listFiles(), 2)

	require.NoError(t, os.RemoveAll(bookDir))

	results := m.processDirectoryEvent(tc.ctx, bookDir, pendingEvent{
		Op:          fsnotify.Remove,
		LibraryID:   1,
		IsDirectory: true,
	})
	assert.Len(t, results, 2)

	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}
