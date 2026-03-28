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

func TestCleanupOrphanedFiles_FullOrphan_NoSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Single File")
	testgen.GenerateEPUB(t, bookDir, "only.epub", testgen.EPUBOptions{
		Title:   "Single File",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// No files in scannedPaths = all orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book and file should both be deleted
	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
}

func TestCleanupOrphanedFiles_FullOrphan_PromotesSupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] With Supplement")
	testgen.GenerateEPUB(t, bookDir, "main.epub", testgen.EPUBOptions{
		Title:   "With Supplement",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)

	allBooks := tc.listBooks()
	bookID := allBooks[0].ID

	// Manually add a supplement file in the DB (a CBZ supplement that can be promoted)
	supplement := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      filepath.Join(bookDir, "supplement.cbz"),
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 100,
	}
	err = tc.bookService.CreateFile(tc.ctx, supplement)
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1, "only main files are returned")

	// No main files in scannedPaths = all main files orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book should still exist
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)

	// Main file should be deleted, supplement should remain and be promoted
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, supplement.ID, remainingFiles[0].ID)
	assert.Equal(t, models.FileRoleMain, remainingFiles[0].FileRole)
}

func TestCleanupOrphanedFiles_NoOrphans(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Healthy Book")
	testgen.GenerateEPUB(t, bookDir, "file.epub", testgen.EPUBOptions{
		Title:   "Healthy Book",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// All files are in scannedPaths — no orphans
	scannedPaths := make(map[string]struct{})
	for _, f := range existingFiles {
		scannedPaths[f.Filepath] = struct{}{}
	}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Everything should remain unchanged
	assert.Len(t, tc.listBooks(), 1)
	assert.Len(t, tc.listFiles(), 1)
}

func TestCleanupOrphanedFiles_FullOrphan_NewMainFileAddedDuringScan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Replaced File")
	testgen.GenerateEPUB(t, bookDir, "original.epub", testgen.EPUBOptions{
		Title:   "Replaced File",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	allBooks := tc.listBooks()
	bookID := allBooks[0].ID

	// Snapshot existingFiles BEFORE the parallel scan adds a new file.
	// This simulates the pre-scan ListFilesForLibrary call.
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1)

	// Simulate what the parallel scan does: add a new main file to the same book.
	// In production, this happens when a user replaces original.epub with replacement.epub on disk.
	newFile := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      filepath.Join(bookDir, "replacement.epub"),
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 200,
	}
	err = tc.bookService.CreateFile(tc.ctx, newFile)
	require.NoError(t, err)

	// Original file is NOT in scannedPaths (deleted from disk).
	// replacement.epub IS on disk but was not in existingFiles (added during scan).
	scannedPaths := map[string]struct{}{
		newFile.Filepath: {},
	}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book should survive — it gained a new main file during the scan
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)

	// Original file deleted, new file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, newFile.ID, remainingFiles[0].ID)
	assert.Equal(t, models.FileRoleMain, remainingFiles[0].FileRole)
}

func intPtr(i int) *int {
	return &i
}
