package worker

import (
	"os"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanFileByPath_InvalidatesFingerprint verifies that when a file's size or
// mtime changes since the last scan (indicating out-of-band content
// modification), any stored fingerprints are deleted so the next hash
// generation job recomputes them.
func TestScanFileByPath_InvalidatesFingerprint(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Set up library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Write a real EPUB so the initial scan can parse metadata
	epubPath := testgen.GenerateEPUB(t, libraryPath, "my-book.epub", testgen.EPUBOptions{
		Title:   "My Book",
		Authors: []string{"Some Author"},
	})

	// Run initial scan to create the file row with correct FilesizeBytes and FileModifiedAt
	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	// Insert a stale fingerprint
	err = tc.fingerprintService.Insert(tc.ctx, fileID, models.FingerprintAlgorithmSHA256, "stale-hash")
	require.NoError(t, err)

	// Verify the fingerprint exists before we trigger a change
	count, err := tc.fingerprintService.CountForFile(tc.ctx, fileID)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected stale fingerprint to exist before rescan")

	// Overwrite the file on disk with different content (different size → size
	// check fails → scan detects change). The new file must be a valid EPUB so
	// the rescan's metadata parsing doesn't abort before the invalidation logic
	// runs (invalidation happens before delegation to scanFileByID which does
	// the actual parsing, but we want a clean, deterministic test).
	testgen.GenerateEPUB(t, libraryPath, "my-book.epub", testgen.EPUBOptions{
		Title:    "My Book — Updated Edition",
		Authors:  []string{"Some Author"},
		HasCover: true,
	})

	// Verify the on-disk file actually differs from what the DB recorded
	stat, err := os.Stat(epubPath)
	require.NoError(t, err)
	require.NotEqual(t, files[0].FilesizeBytes, stat.Size(),
		"test requires different file size to trigger change detection")

	// Rescan by path — should detect the size change and invalidate fingerprints
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)

	// Stale fingerprint must have been deleted
	count, err = tc.fingerprintService.CountForFile(tc.ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "stale fingerprint should have been deleted when file content changed")
}

// TestScanFileByPath_UnchangedFile_PreservesFingerprint verifies that when
// scanFileByPath is called for an unchanged file (same size and mtime),
// its fingerprint is NOT deleted. This protects against spurious monitor
// events (e.g. organize-generated CREATE events that slip past IgnorePath)
// from wiping fingerprints for files whose content hasn't actually changed.
func TestScanFileByPath_UnchangedFile_PreservesFingerprint(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	epubPath := testgen.GenerateEPUB(t, libraryPath, "unchanged.epub", testgen.EPUBOptions{
		Title:   "Unchanged Book",
		Authors: []string{"Some Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	// Store a fingerprint as if the hash gen job had already run.
	err = tc.fingerprintService.Insert(tc.ctx, fileID, models.FingerprintAlgorithmSHA256, "existing-hash-value")
	require.NoError(t, err)

	// Rescan by path WITHOUT touching the file on disk — size and mtime
	// match what's in the DB, so the rescan should short-circuit without
	// invalidating the fingerprint.
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	}, nil)
	require.NoError(t, err)

	count, err := tc.fingerprintService.CountForFile(tc.ctx, fileID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "fingerprint must survive a rescan of an unchanged file")
}
