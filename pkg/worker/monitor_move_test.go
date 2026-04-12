package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/shishobooks/shisho/internal/testgen"
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
