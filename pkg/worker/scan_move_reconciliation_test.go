package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanReconciliation_DetectsMove verifies that when a file has been moved
// while the server was offline (old path gone, new path present), the
// reconciliation phase detects the move via size+sha256 matching, updates the
// DB row's filepath in-place, and does NOT delete the row or create a duplicate.
func TestScanReconciliation_DetectsMove(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Arrange: create library with a file at "old/path"
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	oldDir := testgen.CreateSubDir(t, libraryPath, "[Author] Moved Book")
	epubPath := testgen.GenerateEPUB(t, oldDir, "book.epub", testgen.EPUBOptions{
		Title:   "Moved Book",
		Authors: []string{"Author"},
	})

	// Initial scan: registers file at old path
	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID
	oldPath := files[0].Filepath

	// Insert a sha256 fingerprint so the reconciliation phase can match
	hash, err := computeSHA256ForTest(epubPath)
	require.NoError(t, err)
	err = tc.fingerprintService.Insert(tc.ctx, fileID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)

	// Simulate move: rename the file on disk to a new path inside the library.
	// The new directory represents the file at a different location.
	newDir := testgen.CreateSubDir(t, libraryPath, "[Author] Moved Book New Location")
	newPath := filepath.Join(newDir, "book.epub")
	err = os.Rename(epubPath, newPath)
	require.NoError(t, err)
	// Also remove the old directory so it looks like a true move
	os.Remove(oldDir) // ignore error – dir may not be empty if subfiles exist

	// Act: run scan again (reconciliation should detect the move)
	err = tc.runScan()
	require.NoError(t, err)

	// Assert: exactly one file row, filepath updated to new path
	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 1, "expected no duplicate file rows after move")
	assert.Equal(t, fileID, filesAfter[0].ID, "same DB row should be reused (not deleted and recreated)")
	assert.NotEqual(t, oldPath, filesAfter[0].Filepath, "filepath should have been updated away from old path")
	assert.Equal(t, newPath, filesAfter[0].Filepath, "filepath should point to new location")

	// Assert: exactly one book (no ghost books left over), and its filepath
	// was updated to the new directory so cover serving / organize continue
	// to resolve correctly.
	booksAfter := tc.listBooks()
	require.Len(t, booksAfter, 1, "expected exactly one book after move")
	assert.Equal(t, newDir, booksAfter[0].Filepath, "book filepath should follow the file to the new directory")
}

// TestScanReconciliation_SizeMismatch_Deletes verifies that when the old DB
// file has a different size from the new file on disk (so it cannot be the
// same content), the old row is treated as a normal orphan (deleted) and the
// new file at the new path creates a fresh row.
func TestScanReconciliation_SizeMismatch_Deletes(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// File in DB at "old path" - with one set of content
	oldDir := testgen.CreateSubDir(t, libraryPath, "[Author] Old Book")
	epubOldPath := testgen.GenerateEPUB(t, oldDir, "old.epub", testgen.EPUBOptions{
		Title:   "Old Book",
		Authors: []string{"Author"},
	})

	// Initial scan
	err := tc.runScan()
	require.NoError(t, err)
	files := tc.listFiles()
	require.Len(t, files, 1)
	oldFileID := files[0].ID

	// Insert a sha256 fingerprint for the old file
	oldHash, err := computeSHA256ForTest(epubOldPath)
	require.NoError(t, err)
	err = tc.fingerprintService.Insert(tc.ctx, oldFileID, models.FingerprintAlgorithmSHA256, oldHash)
	require.NoError(t, err)

	// Remove old file from disk (it's "gone")
	require.NoError(t, os.Remove(epubOldPath))

	// Create a different file at new path (different content = different size)
	newDir := testgen.CreateSubDir(t, libraryPath, "[Author] New Book")
	testgen.GenerateEPUB(t, newDir, "new.epub", testgen.EPUBOptions{
		Title:    "New Book With Different Content",
		Authors:  []string{"Author"},
		HasCover: true, // larger file due to cover
	})

	// Act: scan — size mismatch means no reconciliation match
	err = tc.runScan()
	require.NoError(t, err)

	// Assert: old row was deleted (orphan cleanup), new row was created
	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 1, "expected exactly one file row: the new file")
	assert.NotEqual(t, oldFileID, filesAfter[0].ID, "old row should have been deleted, not reused")
}

// TestScanReconciliation_NoFingerprint_Deletes verifies that when the old DB
// file has no fingerprint stored, it cannot be matched via reconciliation and
// falls through to normal orphan deletion. The new file at the new path gets
// a fresh row.
func TestScanReconciliation_NoFingerprint_Deletes(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// File in DB at old path
	oldDir := testgen.CreateSubDir(t, libraryPath, "[Author] No Fingerprint Book")
	epubOldPath := testgen.GenerateEPUB(t, oldDir, "nofp.epub", testgen.EPUBOptions{
		Title:   "No Fingerprint Book",
		Authors: []string{"Author"},
	})

	// Initial scan - do NOT insert any fingerprint
	err := tc.runScan()
	require.NoError(t, err)
	files := tc.listFiles()
	require.Len(t, files, 1)
	oldFileID := files[0].ID

	// Verify there is genuinely no fingerprint
	count, err := tc.fingerprintService.CountForFile(tc.ctx, oldFileID)
	require.NoError(t, err)
	require.Equal(t, 0, count, "test precondition: old file must have no fingerprint")

	// Move file on disk to new path
	newDir := testgen.CreateSubDir(t, libraryPath, "[Author] No Fingerprint Book New")
	newPath := filepath.Join(newDir, "nofp.epub")
	err = os.Rename(epubOldPath, newPath)
	require.NoError(t, err)

	// Act: scan — no fingerprint means reconciliation cannot match → orphan deletion
	err = tc.runScan()
	require.NoError(t, err)

	// Assert: old row was deleted, new row was created at new path
	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 1, "expected exactly one file row after scan")
	assert.NotEqual(t, oldFileID, filesAfter[0].ID, "old row should have been deleted, not reused")
	assert.Equal(t, newPath, filesAfter[0].Filepath, "new file row should be at new path")
}

// computeSHA256ForTest is a test helper that computes the sha256 of a file.
// It uses the same fingerprint package that production code uses to ensure
// test hashes match production hashes.
func computeSHA256ForTest(path string) (string, error) {
	return computeFileSHA256(path)
}
