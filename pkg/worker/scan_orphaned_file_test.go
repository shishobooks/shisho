package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orphanBook deletes a Book on a connection with foreign keys off, as a
// replacement connection did before pragmas ran on every connection, so its
// files and join rows stay behind pointing at it.
func (tc *testContext) orphanBook(bookID int) {
	tc.t.Helper()
	conn, err := tc.db.Conn(tc.ctx)
	require.NoError(tc.t, err)
	defer conn.Close()
	_, err = conn.ExecContext(tc.ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(tc.t, err)
	_, err = conn.ExecContext(tc.ctx, "DELETE FROM books WHERE id = ?", bookID)
	require.NoError(tc.t, err)
	_, err = conn.ExecContext(tc.ctx, "PRAGMA foreign_keys = ON")
	require.NoError(tc.t, err)
}

func (tc *testContext) countRows(query string, args ...any) int {
	tc.t.Helper()
	var count int
	require.NoError(tc.t, tc.db.NewRaw(query, args...).Scan(tc.ctx, &count))
	return count
}

func TestScanFileByID_OrphanedFile_MissingOnDisk_DeletesRow(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Emily Henry] Beach Read")
	testgen.GenerateEPUB(t, bookDir, "Beach Read.epub", testgen.EPUBOptions{
		Title:   "Beach Read",
		Authors: []string{"Emily Henry"},
	})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	file := files[0]
	tc.orphanBook(file.BookID)
	require.NoError(t, os.Remove(file.Filepath))

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.FileDeleted)

	assert.Empty(t, tc.listFiles())
	assert.Zero(t, tc.countRows("SELECT COUNT(*) FROM authors WHERE book_id = ?", file.BookID),
		"the missing Book's join rows are removed with its file")
}

func TestScanFileByID_OrphanedFile_OnDisk_ImportsAsNewBook(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Emily Henry] Beach Read")
	testgen.GenerateEPUB(t, bookDir, "Beach Read.epub", testgen.EPUBOptions{
		Title:   "Beach Read",
		Authors: []string{"Emily Henry"},
	})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	orphaned := files[0]
	tc.orphanBook(orphaned.BookID)

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: orphaned.ID}, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Book)
	require.NotNil(t, result.File)
	assert.True(t, result.FileCreated, "callers organize it like any new file")
	assert.NotEqual(t, orphaned.ID, result.File.ID)
	assert.Equal(t, orphaned.Filepath, result.File.Filepath)
	assert.Equal(t, result.Book.ID, result.File.BookID)

	assert.Zero(t, tc.countRows("SELECT COUNT(*) FROM files WHERE id = ?", orphaned.ID))
	assert.Zero(t, tc.countRows("SELECT COUNT(*) FROM authors WHERE book_id = ?", orphaned.BookID))

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, "Beach Read", allBooks[0].Title)
	assert.Equal(t, result.Book.ID, allBooks[0].ID)
	remaining := tc.listFiles()
	require.Len(t, remaining, 1)
	assert.Equal(t, models.FileRoleMain, remaining[0].FileRole)
}

// scanOrganizedFileThenMove scans one EPUB into an organized library, then
// moves it to download.epub in its folder (disk and database), so the next
// resync renames it back to its organized name.
func (tc *testContext) scanOrganizedFileThenMove() (*models.File, string) {
	tc.t.Helper()
	libraryPath := testgen.TempLibraryDir(tc.t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)
	bookDir := testgen.CreateSubDir(tc.t, libraryPath, "[Emily Henry] Beach Read")
	testgen.GenerateEPUB(tc.t, bookDir, "Beach Read.epub", testgen.EPUBOptions{
		Title:   "Beach Read",
		Authors: []string{"Emily Henry"},
	})
	require.NoError(tc.t, tc.runScan())

	files := tc.listFiles()
	require.Len(tc.t, files, 1)
	file := files[0]
	organized := file.Filepath
	moved := filepath.Join(filepath.Dir(organized), "download.epub")
	require.NoError(tc.t, os.Rename(organized, moved))
	_, err := tc.db.ExecContext(tc.ctx, "UPDATE files SET filepath = ? WHERE id = ?", moved, file.ID)
	require.NoError(tc.t, err)
	file.Filepath = moved
	return file, organized
}

func TestScanFileByID_OrganizeRename_SkipsPathClaimedByAnotherFile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	file, organized := tc.scanOrganizedFileThenMove()

	_, err := tc.db.ExecContext(tc.ctx, `INSERT INTO files (library_id, book_id, filepath, file_type, file_role)
		VALUES (?, ?, ?, 'epub', 'supplement')`, file.LibraryID, file.BookID, organized)
	require.NoError(t, err)

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	reloaded, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(organized), "Beach Read (1).epub"), reloaded.Filepath)
	assert.FileExists(t, reloaded.Filepath)
}

func TestScanFileByID_OrganizeRename_UndoesRenameWhenDatabaseUpdateFails(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	file, organized := tc.scanOrganizedFileThenMove()

	_, err := tc.db.ExecContext(tc.ctx, `CREATE TRIGGER fail_filepath_update BEFORE UPDATE OF filepath ON files
		BEGIN SELECT RAISE(ABORT, 'forced filepath update failure'); END`)
	require.NoError(t, err)

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: file.ID}, nil)
	require.NoError(t, err)

	assert.FileExists(t, file.Filepath, "the file is moved back")
	assert.NoFileExists(t, organized)
	reloaded, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, file.Filepath, reloaded.Filepath)
}

// The reported sequence: a Book is deleted while foreign keys are off, so its
// file row survives. The same EPUB is then imported again at the library root
// and organized into the destination the old row may still name. The
// reimported Book must survive the scans that follow.
func TestReimportAfterOrphanedDelete_SurvivesFullScans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// monitorProcessedRemoval runs the monitor's cleanup of the deleted
		// file before the reimport. Otherwise the orphaned row still claims
		// the organized path when the monitor imports and organizes the new
		// file.
		monitorProcessedRemoval bool
	}{
		{name: "monitor processed the removal", monitorProcessedRemoval: true},
		{name: "orphaned row still claims the destination", monitorProcessedRemoval: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tc := newTestContext(t)

			libraryPath := testgen.TempLibraryDir(t)
			tc.createLibraryWithOptions([]string{libraryPath}, true)
			testgen.GenerateEPUB(t, libraryPath, "Beach Read (z-library).epub", testgen.EPUBOptions{
				Title:   "Beach Read",
				Authors: []string{"Emily Henry"},
			})
			require.NoError(t, tc.runScan())
			files := tc.listFiles()
			require.Len(t, files, 1)
			original := files[0]
			organizedPath := original.Filepath
			require.Equal(t, filepath.Join(libraryPath, "[Emily Henry] Beach Read", "Beach Read.epub"), organizedPath)

			// Delete the Book and its folder, leaving the file row behind.
			tc.orphanBook(original.BookID)
			require.NoError(t, os.RemoveAll(filepath.Dir(organizedPath)))
			if tt.monitorProcessedRemoval {
				result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: original.ID}, nil)
				require.NoError(t, err)
				assert.True(t, result.FileDeleted)
			}

			// The monitor imports the same EPUB again at the library root and
			// organizes it.
			reimported := testgen.GenerateEPUB(t, libraryPath, "Beach Read (z-library).epub", testgen.EPUBOptions{
				Title:   "Beach Read",
				Authors: []string{"Emily Henry"},
			})
			result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FilePath: reimported, LibraryID: original.LibraryID}, nil)
			require.NoError(t, err)
			require.True(t, result.FileCreated)
			book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &result.Book.ID})
			require.NoError(t, err)
			require.NoError(t, tc.bookService.OrganizeBookFiles(tc.ctx, book))

			require.NoError(t, tc.runScan())
			require.NoError(t, tc.runScan())

			allBooks := tc.listBooks()
			require.Len(t, allBooks, 1, "the reimported Book survives")
			assert.Equal(t, result.Book.ID, allBooks[0].ID)
			remaining := tc.listFiles()
			require.Len(t, remaining, 1, "the orphaned row is gone")
			assert.Equal(t, result.File.ID, remaining[0].ID)
			assert.FileExists(t, remaining[0].Filepath, "the database row points where the file is")
			assert.Zero(t, tc.countRows("SELECT COUNT(*) FROM authors WHERE book_id = ?", original.BookID))
		})
	}
}
