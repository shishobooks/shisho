package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/pkg/errors"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// truncateFile chops the tail off a file so its zip central directory is
// incomplete. This mirrors real-world EPUBs damaged by interrupted writes:
// every content entry is intact but the archive cannot be opened.
func truncateFile(t *testing.T, path string, bytesToDrop int64) {
	t.Helper()
	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.Greater(t, stat.Size(), bytesToDrop)
	require.NoError(t, os.Truncate(path, stat.Size()-bytesToDrop))
}

// TestScan_UnreadableFile_RecordsScanErrorAndKeepsSidecar covers a file that
// was imported successfully and later replaced on disk by a damaged copy.
// The scanner must keep the previous sidecar (it is the only on-disk record
// of the file's metadata now), persist the parse failure on the file row so
// the UI can surface it, and clear that state once the file is readable again.
func TestScan_UnreadableFile_RecordsScanErrorAndKeepsSidecar(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	epubPath := testgen.GenerateEPUB(t, libraryPath, "damaged-later.epub", testgen.EPUBOptions{
		Title:   "Damaged Later",
		Authors: []string{"Some Author"},
	})

	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)
	require.Nil(t, files[0].ScanError)

	sidecarPath := sidecar.FileSidecarPath(files[0].Filepath)
	require.FileExists(t, sidecarPath, "initial scan should write a file sidecar")

	// Replace the file with a truncated copy. The size change makes the
	// scanner treat it as swapped, which is the path that used to delete the
	// sidecar before attempting to parse.
	truncateFile(t, epubPath, 64)

	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: files[0].ID}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid zip file")

	assert.FileExists(t, sidecarPath, "sidecar must survive a failed parse")

	files = tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].ScanError, "parse failure should be persisted on the file")
	assert.Contains(t, *files[0].ScanError, "not a valid zip file")

	// A full library scan hits the same path but only warns; the state must
	// still be recorded.
	files[0].ScanError = nil
	require.NoError(t, tc.bookService.UpdateFile(tc.ctx, files[0], books.UpdateFileOptions{Columns: []string{"scan_error"}}))
	require.NoError(t, tc.runScan())
	files = tc.listFiles()
	require.Len(t, files, 1)
	require.NotNil(t, files[0].ScanError)

	// Repair the file (regenerate a valid EPUB with different content so the
	// size changes again) and rescan: the error must clear.
	testgen.GenerateEPUB(t, libraryPath, "damaged-later.epub", testgen.EPUBOptions{
		Title:    "Damaged Later",
		Authors:  []string{"Some Author"},
		HasCover: true,
	})
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: files[0].ID}, nil)
	require.NoError(t, err)

	files = tc.listFiles()
	require.Len(t, files, 1)
	assert.Nil(t, files[0].ScanError, "successful rescan should clear the scan error")
	assert.FileExists(t, sidecarPath)
}

// TestScanFileByPath_ScanErrorForcesReparse covers a repair that restores the
// file with the same size and mtime the database already has (for example
// cp -p from a backup). The size/mtime shortcut must not skip a file that is
// currently flagged unreadable, otherwise the flag would never clear.
func TestScanFileByPath_ScanErrorForcesReparse(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	epubPath := testgen.GenerateEPUB(t, libraryPath, "restored.epub", testgen.EPUBOptions{
		Title:   "Restored",
		Authors: []string{"Some Author"},
	})
	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)

	// Flag the file without touching it on disk, so size and mtime still
	// match the stored values.
	stale := "zip: not a valid zip file"
	files[0].ScanError = &stale
	require.NoError(t, tc.bookService.UpdateFile(tc.ctx, files[0], books.UpdateFileOptions{Columns: []string{"scan_error"}}))

	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FilePath: epubPath, LibraryID: 1}, nil)
	require.NoError(t, err)

	files = tc.listFiles()
	require.Len(t, files, 1)
	assert.Nil(t, files[0].ScanError, "a flagged file must be re-parsed even when size and mtime are unchanged")
}

func TestScanErrorMessage(t *testing.T) {
	t.Parallel()

	t.Run("uses the innermost cause", func(t *testing.T) {
		t.Parallel()
		err := errors.Wrap(errors.Wrap(errors.New("zip: not a valid zip file"), "failed to parse file"), "failed to parse file metadata")
		assert.Equal(t, "zip: not a valid zip file", scanErrorMessage(err))
	})

	t.Run("keeps only the first line of a multi-line error", func(t *testing.T) {
		t.Parallel()
		err := errors.New("TypeError: cannot read property\n    at parse (plugin.js:12)\n    at run (plugin.js:40)")
		assert.Equal(t, "TypeError: cannot read property", scanErrorMessage(err))
	})

	t.Run("caps very long messages", func(t *testing.T) {
		t.Parallel()
		err := errors.New(strings.Repeat("x", 2000))
		msg := scanErrorMessage(err)
		assert.LessOrEqual(t, len(msg), maxScanErrorLength)
		assert.True(t, strings.HasSuffix(msg, "..."))
	})

	t.Run("falls back to a generic message when the error text is empty", func(t *testing.T) {
		t.Parallel()
		blank := "   \n"
		assert.Equal(t, "file could not be parsed", scanErrorMessage(errors.New(blank)))
	})
}
