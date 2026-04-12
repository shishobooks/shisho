package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// injectMonitorEvent places a synthetic pending event into the monitor's
// pending map. This lets tests simulate filesystem events without needing a
// real fsnotify watcher.
//
//nolint:unparam // isDir is false in current tests but the parameter is needed for completeness
func injectMonitorEvent(m *Monitor, path string, op fsnotify.Op, libraryID int, isDir bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending[path] = pendingEvent{
		Op:          op,
		LibraryID:   libraryID,
		IsDirectory: isDir,
	}
}

// newTestMonitorWithWorker creates a Monitor wired up to a real worker and
// test context. Returns the monitor and the library ID of the first library
// so callers can inject events with the correct library ID.
func newTestMonitorWithWorker(tc *testContext, libDir string) (*Monitor, int) {
	tc.t.Helper()
	tc.worker.config.LibraryMonitorDelaySeconds = minMonitorDelaySeconds

	libs, err := tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{})
	if err != nil || len(libs) == 0 {
		tc.t.Fatalf("newTestMonitorWithWorker: could not retrieve library: %v", err)
	}
	libID := libs[0].ID

	m := newMonitor(tc.worker)
	m.pathToLibrary[libDir] = libID
	return m, libID
}

// TestMonitor_DetectsFileMove verifies that when a batch contains a REMOVE for
// an old path and a CREATE for a new path with the same content, the monitor
// detects the move via sha256 matching and updates the file row's filepath in
// place rather than creating a duplicate book.
func TestMonitor_DetectsFileMove(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create and scan the file at its original location.
	oldDir := testgen.CreateSubDir(t, libDir, "Author - Old Location")
	epubPath := testgen.GenerateEPUB(t, oldDir, "book.epub", testgen.EPUBOptions{
		Title:   "Moveable Feast",
		Authors: []string{"Author"},
	})

	m, libID := newTestMonitorWithWorker(tc, libDir)

	// Seed the DB via the monitor's processEvent.
	result := m.processEvent(tc.ctx, epubPath, pendingEvent{Op: fsnotify.Create, LibraryID: libID})
	require.NotNil(t, result)
	require.True(t, result.FileCreated)
	require.Len(t, tc.listFiles(), 1)

	originalFileID := tc.listFiles()[0].ID

	// Insert a sha256 fingerprint so tryDetectMove can match it.
	hash, err := computeFileSHA256(epubPath)
	require.NoError(t, err)
	err = tc.fingerprintService.Insert(tc.ctx, originalFileID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)

	// Simulate a move: rename the file on disk to a new location.
	newDir := testgen.CreateSubDir(t, libDir, "Author - New Location")
	newPath := filepath.Join(newDir, "book.epub")
	require.NoError(t, os.Rename(epubPath, newPath))

	// Inject a mixed batch: REMOVE for old path, CREATE for new path.
	injectMonitorEvent(m, epubPath, fsnotify.Remove, libID, false)
	injectMonitorEvent(m, newPath, fsnotify.Create, libID, false)

	// Process the batch.
	m.processPendingEvents()

	// Assert: still exactly one file row (no duplicate), filepath updated.
	files := tc.listFiles()
	require.Len(t, files, 1, "expected no duplicate file rows after move")
	assert.Equal(t, originalFileID, files[0].ID, "same DB row should be reused (not deleted and recreated)")
	assert.Equal(t, newPath, files[0].Filepath, "filepath should point to new location")

	// Assert: exactly one book (no ghost books left over), AND the book's
	// filepath was updated to the new directory so cover serving, supplement
	// detection, and organize all continue to resolve correctly.
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "expected exactly one book after move")
	assert.Equal(t, newDir, allBooks[0].Filepath, "book filepath should follow the file to the new directory")
}

// TestMonitor_CreateOnlyBatch_NoSyncHashing verifies that when a batch contains
// only CREATE events (no REMOVEs), the monitor does NOT compute sha256 inline
// and instead queues a hash generation job after processing.
func TestMonitor_CreateOnlyBatch_NoSyncHashing(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create a file on disk but do NOT insert a fingerprint.
	bookDir := testgen.CreateSubDir(t, libDir, "New Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "Create Only",
		Authors: []string{"Author"},
	})

	m, libID := newTestMonitorWithWorker(tc, libDir)

	// Inject only a CREATE event — no REMOVE.
	injectMonitorEvent(m, epubPath, fsnotify.Create, libID, false)
	m.processPendingEvents()

	// File should be in the DB.
	files := tc.listFiles()
	require.Len(t, files, 1, "file should have been scanned and created")

	// A hash generation job should have been enqueued (not inline hashing).
	// The file should NOT have a fingerprint stored immediately.
	count, err := tc.fingerprintService.CountForFile(tc.ctx, files[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no fingerprint should be computed inline for a create-only batch")

	// Verify a hash generation job was enqueued.
	jobType := models.JobTypeHashGeneration
	pending, err := tc.jobService.ListJobs(tc.ctx, jobs.ListJobsOptions{
		Type:     &jobType,
		Statuses: []string{models.JobStatusPending, models.JobStatusInProgress},
	})
	require.NoError(t, err)
	assert.Len(t, pending, 1, "expected one pending hash generation job after create-only batch")
}

// TestMonitor_PathStillExists_TreatAsCopy verifies that when a batch contains
// a REMOVE and CREATE for different paths but both paths still exist on disk
// (i.e. the "old" path was not actually removed — it's a copy), the monitor
// treats it as a new file rather than a move, preserving the original row.
func TestMonitor_PathStillExists_TreatAsCopy(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create and scan the original file.
	origDir := testgen.CreateSubDir(t, libDir, "Original")
	origPath := testgen.GenerateEPUB(t, origDir, "book.epub", testgen.EPUBOptions{
		Title:   "Copy Test",
		Authors: []string{"Author"},
	})

	m, libID := newTestMonitorWithWorker(tc, libDir)

	result := m.processEvent(tc.ctx, origPath, pendingEvent{Op: fsnotify.Create, LibraryID: libID})
	require.NotNil(t, result)
	require.Len(t, tc.listFiles(), 1)

	originalFileID := tc.listFiles()[0].ID

	// Insert fingerprint for the original file.
	hash, err := computeFileSHA256(origPath)
	require.NoError(t, err)
	err = tc.fingerprintService.Insert(tc.ctx, originalFileID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)

	// Copy the file (both old and new paths exist on disk).
	copyDir := testgen.CreateSubDir(t, libDir, "Copy")
	copyPath := filepath.Join(copyDir, "book.epub")
	data, err := os.ReadFile(origPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(copyPath, data, 0644))

	// Inject a mixed batch: a REMOVE event for some unrelated old path (so
	// needsSyncHash is true) and a CREATE for the copy path.
	// The original path still exists on disk — so tryDetectMove should NOT
	// repurpose the original row (it's a copy, not a move).
	unrelatedOldPath := filepath.Join(libDir, "unrelated.epub")
	injectMonitorEvent(m, unrelatedOldPath, fsnotify.Remove, libID, false)
	injectMonitorEvent(m, copyPath, fsnotify.Create, libID, false)

	m.processPendingEvents()

	// Both the original file and the copy should be in the DB.
	files := tc.listFiles()
	require.Len(t, files, 2, "original and copy should both be in DB")

	// Original row must be unchanged.
	var origFound bool
	var copyFound bool
	for _, f := range files {
		if f.ID == originalFileID {
			origFound = true
			assert.Equal(t, origPath, f.Filepath, "original row filepath should be unchanged")
		}
		if f.Filepath == copyPath {
			copyFound = true
		}
	}
	assert.True(t, origFound, "original file row should still exist unchanged")
	assert.True(t, copyFound, "copy file should have a new row")
}

// TestMonitor_DetectsFileMove_MultipleDisplacedCandidates verifies the
// tiebreak behavior of tryDetectMove when multiple file rows share the same
// sha256 AND all of their stored paths are missing on disk (i.e. the library
// had byte-identical books at several locations and they all got moved or
// deleted before the monitor reconciled them).
//
// Expected behavior: the candidate with the most recent FileModifiedAt wins
// — its filepath is updated to the new path — and any other displaced
// candidates are deleted so they don't linger as ghost rows with dead paths.
func TestMonitor_DetectsFileMove_MultipleDisplacedCandidates(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create the real file at the NEW location — this is the file the monitor
	// will scan. Both DB rows will point at old paths that never existed on
	// disk (we create them purely in the DB) so both pass the "path is gone"
	// check in tryDetectMove.
	newDir := testgen.CreateSubDir(t, libDir, "Author - Destination")
	newPath := testgen.GenerateEPUB(t, newDir, "book.epub", testgen.EPUBOptions{
		Title:   "Shared Content",
		Authors: []string{"Author"},
	})

	hash, err := computeFileSHA256(newPath)
	require.NoError(t, err)

	// Resolve the library ID.
	libs, err := tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{})
	require.NoError(t, err)
	require.Len(t, libs, 1)
	libID := libs[0].ID

	// Create two books with phantom files that share the same content hash.
	// Each row's Filepath points at a location that doesn't exist on disk.
	olderTime := time.Now().Add(-2 * time.Hour)
	newerTime := time.Now().Add(-1 * time.Hour)

	stat, err := os.Stat(newPath)
	require.NoError(t, err)
	size := stat.Size()

	olderBook := &models.Book{
		LibraryID:    libID,
		Filepath:     filepath.Join(libDir, "older-ghost-dir"),
		Title:        "Older Ghost",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Older Ghost",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, olderBook))

	olderFile := &models.File{
		LibraryID:      libID,
		BookID:         olderBook.ID,
		Filepath:       filepath.Join(libDir, "older-ghost-dir", "book.epub"),
		FileType:       models.FileTypeEPUB,
		FileRole:       models.FileRoleMain,
		FilesizeBytes:  size,
		FileModifiedAt: &olderTime,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, olderFile))
	require.NoError(t,
		tc.fingerprintService.Insert(tc.ctx, olderFile.ID, models.FingerprintAlgorithmSHA256, hash),
	)

	newerBook := &models.Book{
		LibraryID:    libID,
		Filepath:     filepath.Join(libDir, "newer-ghost-dir"),
		Title:        "Newer Ghost",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Newer Ghost",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, newerBook))

	newerFile := &models.File{
		LibraryID:      libID,
		BookID:         newerBook.ID,
		Filepath:       filepath.Join(libDir, "newer-ghost-dir", "book.epub"),
		FileType:       models.FileTypeEPUB,
		FileRole:       models.FileRoleMain,
		FilesizeBytes:  size,
		FileModifiedAt: &newerTime,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, newerFile))
	require.NoError(t,
		tc.fingerprintService.Insert(tc.ctx, newerFile.ID, models.FingerprintAlgorithmSHA256, hash),
	)

	m, _ := newTestMonitorWithWorker(tc, libDir)

	// Trigger move detection directly — we're testing tryDetectMove's
	// tiebreak logic in isolation.
	moved, err := m.tryDetectMove(tc.ctx, newPath, libID)
	require.NoError(t, err)
	require.NotNil(t, moved, "move should be detected")

	// The newer candidate should have won the tiebreak.
	assert.Equal(t, newerFile.ID, moved.ID, "tiebreak should pick the most recently modified candidate")
	assert.Equal(t, newPath, moved.Filepath, "winner's filepath should point to the new location")

	// The older candidate should have been deleted as a ghost.
	_, err = tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &olderFile.ID})
	require.Error(t, err, "older ghost file row should have been deleted")

	// Verify the winner's row is still present at the new path.
	winnerAfter, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &newerFile.ID})
	require.NoError(t, err)
	assert.Equal(t, newPath, winnerAfter.Filepath)
}

// TestMonitor_DetectsFileMove_CrossTypeCollisionIgnored verifies that
// tryDetectMove does not repurpose a file row whose FileType differs from
// the new file's extension, even if the sha256 happens to match. This
// guards against the (unlikely) case where a supplement's content collides
// with a main file — allowing it would corrupt book state and confuse
// syncBookFilepathAfterMove.
func TestMonitor_DetectsFileMove_CrossTypeCollisionIgnored(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})

	// Create a real .epub at a new location — this is what the monitor will
	// scan and attempt to match against the fingerprint index.
	newDir := testgen.CreateSubDir(t, libDir, "Author - Destination")
	newPath := testgen.GenerateEPUB(t, newDir, "book.epub", testgen.EPUBOptions{
		Title:   "Cross Type",
		Authors: []string{"Author"},
	})

	hash, err := computeFileSHA256(newPath)
	require.NoError(t, err)

	libs, err := tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{})
	require.NoError(t, err)
	libID := libs[0].ID

	// Seed a phantom .pdf file row whose content hash matches the .epub
	// above (simulating an unlikely cross-type collision). Its path does
	// not exist on disk.
	ghostBook := &models.Book{
		LibraryID:    libID,
		Filepath:     filepath.Join(libDir, "phantom-pdf-dir"),
		Title:        "Phantom PDF",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Phantom PDF",
		AuthorSource: models.DataSourceFilepath,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, ghostBook))

	stat, err := os.Stat(newPath)
	require.NoError(t, err)
	ghostFile := &models.File{
		LibraryID:     libID,
		BookID:        ghostBook.ID,
		Filepath:      filepath.Join(libDir, "phantom-pdf-dir", "doc.pdf"),
		FileType:      models.FileTypePDF, // different type from the incoming .epub
		FileRole:      models.FileRoleMain,
		FilesizeBytes: stat.Size(),
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, ghostFile))
	require.NoError(t,
		tc.fingerprintService.Insert(tc.ctx, ghostFile.ID, models.FingerprintAlgorithmSHA256, hash),
	)

	m, _ := newTestMonitorWithWorker(tc, libDir)

	// tryDetectMove should refuse to match the .pdf ghost against the .epub
	// newcomer, even though their sha256 matches.
	moved, err := m.tryDetectMove(tc.ctx, newPath, libID)
	require.NoError(t, err)
	assert.Nil(t, moved, "cross-type collision must not be treated as a move")

	// The ghost file row should be untouched.
	ghostAfter, err := tc.bookService.RetrieveFile(tc.ctx, books.RetrieveFileOptions{ID: &ghostFile.ID})
	require.NoError(t, err)
	assert.Equal(t, ghostFile.Filepath, ghostAfter.Filepath, "ghost PDF row must not be repurposed")
}
