package worker

import (
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupOrphanedFiles_PartialOrphan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create library with temp path
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book dir with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Two Files")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "remove.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	files := tc.listFiles()
	require.Len(t, files, 2)

	// Identify which file to orphan
	var keepFile, removeFile *models.File
	for _, f := range files {
		if filepath.Base(f.Filepath) == "keep.epub" {
			keepFile = f
		} else {
			removeFile = f
		}
	}
	require.NotNil(t, keepFile)
	require.NotNil(t, removeFile)

	// Build existingFiles (what ListFilesForLibrary returns — main files)
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 2)

	// Build scannedPaths — only include the file we're keeping (simulates remove.epub being deleted from disk)
	scannedPaths := map[string]struct{}{
		keepFile.Filepath: {},
	}

	// Get library for the method call
	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	// Run orphan cleanup
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Verify: removed file is gone, kept file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, keepFile.ID, remainingFiles[0].ID)

	// Verify: book still exists
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)
}

func intPtr(i int) *int {
	return &i
}
