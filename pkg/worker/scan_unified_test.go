package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_ZeroEntryPoints(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with no entry points set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{})

	// Should return validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
}

func TestScan_MultipleEntryPoints(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with multiple entry points set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: 1,
		BookID: 2,
	})

	// Should return validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
}

func TestScan_SingleEntryPoint_FileID(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with just FileID set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: 1,
	})

	// Should not return validation error
	// May return other errors (like file not found), but not the validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
	}
}

func TestScan_SingleEntryPoint_BookID(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with just BookID set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID: 1,
	})

	// Should not return validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
	}
}

func TestScan_SingleEntryPoint_FilePath(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with just FilePath set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  "/some/path/to/file.epub",
		LibraryID: 1,
	})

	// Should not return validation error
	if err != nil {
		assert.NotContains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
	}
}

func TestScan_MultipleEntryPoints_AllThree(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with all three entry points set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath: "/some/path",
		FileID:   1,
		BookID:   2,
	})

	// Should return validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
}

func TestScan_MultipleEntryPoints_FilePathAndFileID(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with FilePath and FileID set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath: "/some/path",
		FileID:   1,
	})

	// Should return validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
}

func TestScan_MultipleEntryPoints_FilePathAndBookID(t *testing.T) {
	tc := newTestContext(t)

	// Call scanInternal with FilePath and BookID set (tests internal validation logic)
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath: "/some/path",
		BookID:   1,
	})

	// Should return validation error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of FilePath, FileID, or BookID must be set")
}

// =============================================================================
// scanFileByID tests
// =============================================================================

func TestScanFileByID_MissingFile_DeletesFile(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Multi File Book")
	testgen.GenerateEPUB(t, bookDir, "file1.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "file2.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and files in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify both files were created
	files := tc.listFiles()
	require.Len(t, files, 2)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Find the file that will be deleted (file1.epub)
	var fileToDelete *models.File
	for _, f := range files {
		if filepath.Base(f.Filepath) == "file1.epub" {
			fileToDelete = f
			break
		}
	}
	require.NotNil(t, fileToDelete, "file1.epub should exist")

	// Delete the physical file from disk
	err = os.Remove(fileToDelete.Filepath)
	require.NoError(t, err)

	// Call Scan with FileID of the deleted file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: fileToDelete.ID,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// File should be deleted but book should remain
	assert.True(t, result.FileDeleted, "FileDeleted should be true")
	assert.False(t, result.BookDeleted, "BookDeleted should be false since book has other files")

	// Verify file is gone from DB
	_, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, fileToDelete.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify book still exists
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.NotNil(t, book)

	// Verify one file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
}

func TestScanFileByID_MissingFile_LastFile_DeletesBook(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with 1 EPUB file
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Single File Book")
	testgen.GenerateEPUB(t, bookDir, "only-file.epub", testgen.EPUBOptions{
		Title:   "Single File Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and file in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file and book were created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Delete the physical file from disk
	err = os.Remove(files[0].Filepath)
	require.NoError(t, err)

	// Call Scan with FileID of the deleted file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: fileID,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Both file and book should be deleted
	assert.True(t, result.FileDeleted, "FileDeleted should be true")
	assert.True(t, result.BookDeleted, "BookDeleted should be true since it was the last file")

	// Verify file is gone from DB
	_, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify book is gone from DB
	_, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify no files or books remain
	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

func TestScanFileByID_NotFound(t *testing.T) {
	tc := newTestContext(t)

	// Call Scan with a non-existent FileID
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: 99999,
	})

	// Should return error containing "not found"
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestScanFileByID_UnreadableFile(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Unreadable Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "unreadable.epub", testgen.EPUBOptions{
		Title:   "Unreadable Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and file in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file was created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	// Make the file unreadable (chmod 000)
	err = os.Chmod(epubPath, 0000)
	require.NoError(t, err)

	// Ensure we restore permissions for cleanup
	t.Cleanup(func() {
		os.Chmod(epubPath, 0644)
	})

	// Call Scan with FileID of the unreadable file
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: fileID,
	})

	// Should return error (file exists but can't be read)
	require.Error(t, err)
}

func TestScanFileByID_CorruptFile(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Corrupt Book")

	// Create a corrupt EPUB file (just random bytes, not a valid ZIP/EPUB)
	corruptPath := filepath.Join(bookDir, "corrupt.epub")
	err := os.WriteFile(corruptPath, []byte("this is not a valid epub file"), 0644)
	require.NoError(t, err)

	// Run initial scan - this will create the book/file but may fail on parsing
	// We need to manually create the file record to test the scan behavior
	// Note: The scan may fail due to the corrupt file, but we ignore the error
	_ = tc.runScan()

	// Check if file was created (it might have been skipped due to parse errors during initial scan)
	files := tc.listFiles()
	if len(files) == 0 {
		// If no file was created, we need to create one manually for this test
		// First create a valid EPUB, scan it, then replace with corrupt content
		testgen.GenerateEPUB(t, bookDir, "valid.epub", testgen.EPUBOptions{
			Title:   "Corrupt Book",
			Authors: []string{"Test Author"},
		})

		err = tc.runScan()
		require.NoError(t, err)

		files = tc.listFiles()
		require.Len(t, files, 1)

		// Now corrupt the file
		err = os.WriteFile(files[0].Filepath, []byte("this is not a valid epub file"), 0644)
		require.NoError(t, err)
	}

	fileID := files[0].ID

	// Call Scan with FileID of the corrupt file
	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: fileID,
	})

	// Should return error containing parse failure details
	require.Error(t, err)
}

// =============================================================================
// scanFileCore tests
// =============================================================================

func TestScanFileCore_BookTitle_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with title from filepath source (lower priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Old",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Old",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source (epub_metadata > filepath)
	metadata := &mediafile.ParsedMetadata{
		Title:      "New",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should be updated
	assert.Equal(t, "New", result.Book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, result.Book.TitleSource)
}

func TestScanFileCore_BookTitle_LowerPriority_Skipped(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with title from manual source (highest priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Manual",
		TitleSource:  models.DataSourceManual,
		SortTitle:    "Manual",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with lower priority source (epub_metadata < manual)
	metadata := &mediafile.ParsedMetadata{
		Title:      "New",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore without forceRefresh
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should remain unchanged
	assert.Equal(t, "Manual", result.Book.Title)
	assert.Equal(t, models.DataSourceManual, result.Book.TitleSource)
}

func TestScanFileCore_BookTitle_ForceRefresh(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with title from manual source (highest priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Manual",
		TitleSource:  models.DataSourceManual,
		SortTitle:    "Manual",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with lower priority source (epub_metadata < manual)
	metadata := &mediafile.ParsedMetadata{
		Title:      "New",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore with forceRefresh=true
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, true, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should be updated despite lower priority (forceRefresh bypasses priority)
	assert.Equal(t, "New", result.Book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, result.Book.TitleSource)
}

func TestScanFileCore_BookSortTitle_Regenerated(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with title from filepath source
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Old Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Old Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with title starting with "The" (will be transformed for sort)
	metadata := &mediafile.ParsedMetadata{
		Title:      "The Hobbit",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Title should be updated
	assert.Equal(t, "The Hobbit", result.Book.Title)
	// SortTitle should be regenerated using sortname.ForTitle
	assert.Equal(t, "Hobbit, The", result.Book.SortTitle)
}

func TestScanFileCore_BookSubtitle_Updated(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book without subtitle
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Main Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Main Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with subtitle
	metadata := &mediafile.ParsedMetadata{
		Subtitle:   "A Great Subtitle",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Subtitle should be set
	require.NotNil(t, result.Book.Subtitle)
	assert.Equal(t, "A Great Subtitle", *result.Book.Subtitle)
	require.NotNil(t, result.Book.SubtitleSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *result.Book.SubtitleSource)
}

func TestScanFileCore_BookDescription_Updated(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book without description
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Main Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Main Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with description
	metadata := &mediafile.ParsedMetadata{
		Description: "This is a great book about many things.",
		DataSource:  models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Description should be set
	require.NotNil(t, result.Book.Description)
	assert.Equal(t, "This is a great book about many things.", *result.Book.Description)
	require.NotNil(t, result.Book.DescriptionSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *result.Book.DescriptionSource)
}

func TestScanFileCore_NilMetadata(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library and book in DB
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Original",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Original",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Call scanFileCore with nil metadata
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, nil, false, true)

	// Should succeed but make no changes
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book should be unchanged
	assert.Equal(t, "Original", result.Book.Title)
}

func TestScanFileCore_Authors_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with author from filepath source (lower priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create the old author
	oldPerson, err := tc.personService.FindOrCreatePerson(tc.ctx, "Old Author", 1)
	require.NoError(t, err)
	oldAuthor := &models.Author{
		BookID:    book.ID,
		PersonID:  oldPerson.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, oldAuthor)
	require.NoError(t, err)

	// Reload book with authors
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.Authors, 1)
	require.NotNil(t, book.Authors[0].Person)
	assert.Equal(t, "Old Author", book.Authors[0].Person.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source (epub_metadata > filepath)
	metadata := &mediafile.ParsedMetadata{
		Authors: []mediafile.ParsedAuthor{
			{Name: "New Author"},
		},
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify author update
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should have 1 author "New Author"
	require.Len(t, updatedBook.Authors, 1)
	require.NotNil(t, updatedBook.Authors[0].Person)
	assert.Equal(t, "New Author", updatedBook.Authors[0].Person.Name)
	assert.Equal(t, models.DataSourceEPUBMetadata, updatedBook.AuthorSource)
}

func TestScanFileCore_Authors_LowerPriority_Skipped(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with author from manual source (highest priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceManual,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create the manual author
	manualPerson, err := tc.personService.FindOrCreatePerson(tc.ctx, "Manual Author", 1)
	require.NoError(t, err)
	manualAuthor := &models.Author{
		BookID:    book.ID,
		PersonID:  manualPerson.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, manualAuthor)
	require.NoError(t, err)

	// Reload book with authors
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.Authors, 1)
	require.NotNil(t, book.Authors[0].Person)
	assert.Equal(t, "Manual Author", book.Authors[0].Person.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with lower priority source (epub_metadata < manual)
	metadata := &mediafile.ParsedMetadata{
		Authors: []mediafile.ParsedAuthor{
			{Name: "New Author"},
		},
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore without forceRefresh
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify author was NOT updated
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should still have 1 author "Manual Author" (unchanged)
	require.Len(t, updatedBook.Authors, 1)
	require.NotNil(t, updatedBook.Authors[0].Person)
	assert.Equal(t, "Manual Author", updatedBook.Authors[0].Person.Name)
	assert.Equal(t, models.DataSourceManual, updatedBook.AuthorSource)
}

func TestScanFileCore_Authors_ForceRefresh(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with author from manual source (highest priority)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceManual,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create the manual author
	manualPerson, err := tc.personService.FindOrCreatePerson(tc.ctx, "Manual Author", 1)
	require.NoError(t, err)
	manualAuthor := &models.Author{
		BookID:    book.ID,
		PersonID:  manualPerson.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, manualAuthor)
	require.NoError(t, err)

	// Reload book with authors
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.Authors, 1)
	require.NotNil(t, book.Authors[0].Person)
	assert.Equal(t, "Manual Author", book.Authors[0].Person.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with lower priority source (epub_metadata < manual)
	metadata := &mediafile.ParsedMetadata{
		Authors: []mediafile.ParsedAuthor{
			{Name: "New Author"},
		},
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore with forceRefresh=true
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, true, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify author WAS updated despite lower priority
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should now have 1 author "New Author" (updated despite lower priority)
	require.Len(t, updatedBook.Authors, 1)
	require.NotNil(t, updatedBook.Authors[0].Person)
	assert.Equal(t, "New Author", updatedBook.Authors[0].Person.Name)
	assert.Equal(t, models.DataSourceEPUBMetadata, updatedBook.AuthorSource)
}

func TestScanFileCore_Series_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create old series with filepath source (lower priority)
	oldSeries, err := tc.seriesService.FindOrCreateSeries(tc.ctx, "Old Series", 1, models.DataSourceFilepath)
	require.NoError(t, err)
	oldNumber := 1.0
	oldBookSeries := &models.BookSeries{
		BookID:       book.ID,
		SeriesID:     oldSeries.ID,
		SeriesNumber: &oldNumber,
		SortOrder:    1,
	}
	err = tc.bookService.CreateBookSeries(tc.ctx, oldBookSeries)
	require.NoError(t, err)

	// Reload book with series
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.BookSeries, 1)
	require.NotNil(t, book.BookSeries[0].Series)
	assert.Equal(t, "Old Series", book.BookSeries[0].Series.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source (epub_metadata > filepath)
	newNumber := 2.0
	metadata := &mediafile.ParsedMetadata{
		Series:       "New Series",
		SeriesNumber: &newNumber,
		DataSource:   models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify series update
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should be in "New Series" at position 2.0
	require.Len(t, updatedBook.BookSeries, 1)
	require.NotNil(t, updatedBook.BookSeries[0].Series)
	assert.Equal(t, "New Series", updatedBook.BookSeries[0].Series.Name)
	require.NotNil(t, updatedBook.BookSeries[0].SeriesNumber)
	assert.InDelta(t, 2.0, *updatedBook.BookSeries[0].SeriesNumber, 0.0001)
}

func TestScanFileCore_Genres_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with genre from filepath source (lower priority)
	genreSource := models.DataSourceFilepath
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
		GenreSource:  &genreSource,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create old genre
	oldGenre, err := tc.worker.genreService.FindOrCreateGenre(tc.ctx, "Old Genre", 1)
	require.NoError(t, err)
	oldBookGenre := &models.BookGenre{
		BookID:  book.ID,
		GenreID: oldGenre.ID,
	}
	err = tc.bookService.CreateBookGenre(tc.ctx, oldBookGenre)
	require.NoError(t, err)

	// Reload book with genres
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.BookGenres, 1)
	require.NotNil(t, book.BookGenres[0].Genre)
	assert.Equal(t, "Old Genre", book.BookGenres[0].Genre.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source (epub_metadata > filepath)
	metadata := &mediafile.ParsedMetadata{
		Genres:     []string{"New Genre"},
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify genre update
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should have 1 genre "New Genre"
	require.Len(t, updatedBook.BookGenres, 1)
	require.NotNil(t, updatedBook.BookGenres[0].Genre)
	assert.Equal(t, "New Genre", updatedBook.BookGenres[0].Genre.Name)
	require.NotNil(t, updatedBook.GenreSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedBook.GenreSource)
}

func TestScanFileCore_Tags_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with tag from filepath source (lower priority)
	tagSource := models.DataSourceFilepath
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
		TagSource:    &tagSource,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create old tag
	oldTag, err := tc.worker.tagService.FindOrCreateTag(tc.ctx, "Old Tag", 1)
	require.NoError(t, err)
	oldBookTag := &models.BookTag{
		BookID: book.ID,
		TagID:  oldTag.ID,
	}
	err = tc.bookService.CreateBookTag(tc.ctx, oldBookTag)
	require.NoError(t, err)

	// Reload book with tags
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Len(t, book.BookTags, 1)
	require.NotNil(t, book.BookTags[0].Tag)
	assert.Equal(t, "Old Tag", book.BookTags[0].Tag.Name)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source (epub_metadata > filepath)
	metadata := &mediafile.ParsedMetadata{
		Tags:       []string{"New Tag"},
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify tag update
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book should have 1 tag "New Tag"
	require.Len(t, updatedBook.BookTags, 1)
	require.NotNil(t, updatedBook.BookTags[0].Tag)
	assert.Equal(t, "New Tag", updatedBook.BookTags[0].Tag.Name)
	require.NotNil(t, updatedBook.TagSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedBook.TagSource)
}

// =============================================================================
// scanFileCore file updates tests (Task 6)
// =============================================================================

func TestScanFileCore_Narrators_HigherPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Audiobook",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Audiobook",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file with narrator from filepath source (lower priority)
	narratorSource := models.DataSourceFilepath
	file := &models.File{
		LibraryID:      1,
		BookID:         book.ID,
		Filepath:       filepath.Join(libraryPath, "test.m4b"),
		FileType:       models.FileTypeM4B,
		FilesizeBytes:  1000,
		NarratorSource: &narratorSource,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Create old narrator
	oldPerson, err := tc.personService.FindOrCreatePerson(tc.ctx, "Old", 1)
	require.NoError(t, err)
	oldNarrator := &models.Narrator{
		FileID:    file.ID,
		PersonID:  oldPerson.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateNarrator(tc.ctx, oldNarrator)
	require.NoError(t, err)

	// Reload file with narrators
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)
	require.Len(t, file.Narrators, 1)
	require.NotNil(t, file.Narrators[0].Person)
	assert.Equal(t, "Old", file.Narrators[0].Person.Name)

	// Metadata with higher priority source (m4b_metadata > filepath)
	metadata := &mediafile.ParsedMetadata{
		Narrators:  []string{"New"},
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify narrator update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File should have 1 narrator "New"
	require.Len(t, updatedFile.Narrators, 1)
	require.NotNil(t, updatedFile.Narrators[0].Person)
	assert.Equal(t, "New", updatedFile.Narrators[0].Person.Name)
	require.NotNil(t, updatedFile.NarratorSource)
	assert.Equal(t, models.DataSourceM4BMetadata, *updatedFile.NarratorSource)
}

func TestScanFileCore_FileName_CBZ(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Comic",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Comic",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create CBZ file with nil Name
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "comic.cbz"),
		FileType:      models.FileTypeCBZ,
		FilesizeBytes: 1000,
		// Name is nil
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with Title, Series, and SeriesNumber (should generate "Series v1")
	seriesNumber := 1.0
	metadata := &mediafile.ParsedMetadata{
		Title:        "Comic Title",
		Series:       "Series",
		SeriesNumber: &seriesNumber,
		DataSource:   models.DataSourceCBZMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be set based on Series + SeriesNumber
	// generateCBZFileName prefers Title first (if not a filename pattern), then Series+Number
	// Since "Comic Title" doesn't have brackets, it will be used
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "Comic Title", *updatedFile.Name)
	require.NotNil(t, updatedFile.NameSource)
	assert.Equal(t, models.DataSourceCBZMetadata, *updatedFile.NameSource)
}

func TestScanFileCore_FileName_CBZ_UsesSeriesWhenNoTitle(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Comic",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Comic",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create CBZ file with nil Name
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "[Author] comic.cbz"),
		FileType:      models.FileTypeCBZ,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with no Title but has Series and SeriesNumber
	// generateCBZFileName will fall back to Series + Number
	seriesNumber := 1.0
	metadata := &mediafile.ParsedMetadata{
		// No Title
		Series:       "Series",
		SeriesNumber: &seriesNumber,
		DataSource:   models.DataSourceCBZMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be "Series v1" (from generateCBZFileName fallback)
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "Series v1", *updatedFile.Name)
}

func TestScanFileCore_WritesSidecars(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with a real temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "test.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Simple metadata
	metadata := &mediafile.ParsedMetadata{
		Title:      "Test Book",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	_, err = tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)
	require.NoError(t, err)

	// Verify book sidecar exists: <bookpath>/<dirname>.metadata.json
	bookSidecarPath := filepath.Join(bookDir, "Test Book.metadata.json")
	_, err = os.Stat(bookSidecarPath)
	require.NoError(t, err, "book sidecar should exist at %s", bookSidecarPath)

	// Verify file sidecar exists: <filepath>.metadata.json
	fileSidecarPath := filePath + ".metadata.json"
	_, err = os.Stat(fileSidecarPath)
	require.NoError(t, err, "file sidecar should exist at %s", fileSidecarPath)
}

func TestScanFileCore_UpdatesSearchIndex(t *testing.T) {
	tc := newTestContextWithSearchService(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Old Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Old Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with higher priority source
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Title",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	_, err = tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)
	require.NoError(t, err)

	// Verify search index was updated by checking the FTS table directly
	var count int
	err = tc.db.NewSelect().
		TableExpr("books_fts").
		ColumnExpr("COUNT(*)").
		Where("book_id = ?", book.ID).
		Where("title = ?", "New Title").
		Scan(tc.ctx, &count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "book should be indexed with new title in FTS table")
}

// =============================================================================
// scanFileByID integration tests
// =============================================================================

func TestScanFileByID_Integration_UpdatesMetadata(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file containing "File Title"
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")
	testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:   "File Title",
		Authors: []string{"File Author"},
	})

	// Run initial scan to create book and file in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file and book were created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Update book title in DB to "DB Title" (different from file metadata)
	// Use manual source so we can verify ForceRefresh overrides it
	allBooks[0].Title = "DB Title"
	allBooks[0].TitleSource = models.DataSourceManual
	err = tc.bookService.UpdateBook(tc.ctx, allBooks[0], books.UpdateBookOptions{Columns: []string{"title", "title_source"}})
	require.NoError(t, err)

	// Verify the title was updated in DB
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Equal(t, "DB Title", book.Title)

	// Call Scan with FileID and ForceRefresh=true to override the manual title
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID:       fileID,
		ForceRefresh: true,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.File)
	require.NotNil(t, result.Book)

	// Book title should be updated from file metadata
	assert.Equal(t, "File Title", result.Book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, result.Book.TitleSource)

	// Verify book was updated in DB
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Equal(t, "File Title", updatedBook.Title)

	// Verify authors were updated (the file has "File Author")
	require.Len(t, updatedBook.Authors, 1)
	require.NotNil(t, updatedBook.Authors[0].Person)
	assert.Equal(t, "File Author", updatedBook.Authors[0].Person.Name)
}

// =============================================================================
// scanBook tests
// =============================================================================

func TestScanBook_NoFiles_DeletesBook(t *testing.T) {
	tc := newTestContextWithSearchService(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book with no files in DB
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Orphan Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Orphan Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)
	bookID := book.ID

	// Verify book was created (with no files)
	createdBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Empty(t, createdBook.Files)

	// Call Scan with BookID
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID: bookID,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book should be deleted
	assert.True(t, result.BookDeleted, "BookDeleted should be true")

	// Verify book is gone from DB
	_, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestScanBook_NotFound(t *testing.T) {
	tc := newTestContext(t)

	// Call Scan with a non-existent BookID
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID: 99999,
	})

	// Should return error containing "not found"
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestScanBook_MultipleFiles(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Multi File Book")
	testgen.GenerateEPUB(t, bookDir, "file1.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "file2.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and files in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify both files were created
	files := tc.listFiles()
	require.Len(t, files, 2)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Call Scan with BookID
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID: bookID,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have 2 file results
	assert.Len(t, result.Files, 2, "Should have 2 file results")

	// Book should be populated
	require.NotNil(t, result.Book, "result.Book should be populated")
	assert.Equal(t, bookID, result.Book.ID)

	// Book should not be deleted
	assert.False(t, result.BookDeleted, "BookDeleted should be false")
}

func TestScanBook_FileError_ContinuesWithOthers(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Error Book")
	readableEPUBPath := testgen.GenerateEPUB(t, bookDir, "readable.epub", testgen.EPUBOptions{
		Title:   "Error Book",
		Authors: []string{"Test Author"},
	})
	unreadableEPUBPath := testgen.GenerateEPUB(t, bookDir, "unreadable.epub", testgen.EPUBOptions{
		Title:   "Error Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and files in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify both files were created
	files := tc.listFiles()
	require.Len(t, files, 2)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Make one file unreadable (chmod 000)
	err = os.Chmod(unreadableEPUBPath, 0000)
	require.NoError(t, err)

	// Ensure we restore permissions for cleanup
	t.Cleanup(func() {
		os.Chmod(unreadableEPUBPath, 0644)
	})

	// Make sure readable file is readable
	err = os.Chmod(readableEPUBPath, 0644)
	require.NoError(t, err)

	// Call Scan with BookID
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		BookID: bookID,
	})

	// Should succeed (doesn't error out)
	require.NoError(t, err)
	require.NotNil(t, result)

	// At least the readable file should have been processed
	// (The unreadable one may have been skipped due to error)
	assert.GreaterOrEqual(t, len(result.Files), 1, "Should have at least 1 file result")

	// Book should still exist
	assert.False(t, result.BookDeleted, "BookDeleted should be false")
	require.NotNil(t, result.Book, "result.Book should be populated")
}

// =============================================================================
// scanFileByPath tests (Task 9)
// =============================================================================

func TestScanFileByPath_FileNotOnDisk(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Call Scan with FilePath for a non-existent file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  "/nonexistent/path/file.epub",
		LibraryID: 1,
	})

	// Should succeed with nil result (skip silently)
	require.NoError(t, err)
	assert.Nil(t, result, "result should be nil for non-existent file")
}

func TestScanFileByPath_MissingLibraryID(t *testing.T) {
	tc := newTestContext(t)

	// Call Scan with FilePath but no LibraryID
	_, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath: "/some/file.epub",
		// LibraryID not set (default 0)
	})

	// Should return error about missing LibraryID
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LibraryID required")
}

func TestScanFileByPath_ExistingFile_DelegatesToScanFileByID(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:   "Test Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and file in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file was created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID
	originalTitle := "Test Book"

	// Call Scan with FilePath for the existing file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// File should be returned (same file was found and processed)
	require.NotNil(t, result.File, "result.File should be populated")
	assert.Equal(t, fileID, result.File.ID, "should return the same file ID")

	// Book should also be returned
	require.NotNil(t, result.Book, "result.Book should be populated")
	assert.Equal(t, originalTitle, result.Book.Title)

	// FileCreated should be false (file already existed)
	assert.False(t, result.FileCreated, "FileCreated should be false for existing file")
}

func TestScanFileByPath_NewFile(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file (no initial scan)
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] New Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "new.epub", testgen.EPUBOptions{
		Title:   "New Book",
		Authors: []string{"Test Author"},
	})

	// Verify file does not exist in DB
	files := tc.listFiles()
	require.Empty(t, files, "no files should exist in DB before scan")

	// Call Scan with FilePath for the new file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// FileCreated should be true
	assert.True(t, result.FileCreated, "FileCreated should be true for new file")

	// File and book should be populated
	require.NotNil(t, result.File, "result.File should be populated")
	require.NotNil(t, result.Book, "result.Book should be populated")

	// Verify file now exists in DB
	files = tc.listFiles()
	require.Len(t, files, 1, "file should now exist in DB")
}

// TestScanFileByPath_CreatesBookAndFile tests that scanning a new file creates
// both a book record and a file record with proper metadata extraction.
func TestScanFileByPath_CreatesBookAndFile(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:    "Test Book",
		Authors:  []string{"Test Author"},
		HasCover: true,
	})

	// Verify no books or files exist before scan
	require.Empty(t, tc.listBooks(), "no books should exist before scan")
	require.Empty(t, tc.listFiles(), "no files should exist before scan")

	// Call Scan with FilePath for the new file
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  epubPath,
		LibraryID: 1,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// FileCreated should be true
	assert.True(t, result.FileCreated, "FileCreated should be true")

	// Verify book was created with correct metadata
	require.NotNil(t, result.Book, "result.Book should be populated")
	assert.Equal(t, "Test Book", result.Book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, result.Book.TitleSource)

	// Verify authors were created
	// Note: The book needs to be reloaded to get full relations
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &result.Book.ID})
	require.NoError(t, err)
	require.Len(t, book.Authors, 1, "book should have 1 author")
	require.NotNil(t, book.Authors[0].Person)
	assert.Equal(t, "Test Author", book.Authors[0].Person.Name)

	// Verify file was created
	require.NotNil(t, result.File, "result.File should be populated")
	assert.Equal(t, epubPath, result.File.Filepath)
	assert.Equal(t, models.FileTypeEPUB, result.File.FileType)

	// Verify cover file exists on disk
	coverDir := bookDir
	coverBaseName := "test.epub.cover"
	coverPath := fileutils.CoverExistsWithBaseName(coverDir, coverBaseName)
	assert.NotEmpty(t, coverPath, "cover file should exist on disk")

	// Verify DB state
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "should have exactly 1 book in DB")

	allFiles := tc.listFiles()
	require.Len(t, allFiles, 1, "should have exactly 1 file in DB")
}

// =============================================================================
// ProcessScanJob orphan cleanup tests (Task 12)
// =============================================================================

func TestProcessScanJob_CleansUpOrphanedFiles(t *testing.T) {
	tc := newTestContextWithSearchService(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Orphan Book")
	epubPath := testgen.GenerateEPUB(t, bookDir, "orphan.epub", testgen.EPUBOptions{
		Title:   "Orphan Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and file in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file and book were created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Delete the physical file from disk (simulating file deleted outside of app)
	err = os.Remove(epubPath)
	require.NoError(t, err)

	// Also remove the book directory (since it only contained one file)
	err = os.RemoveAll(bookDir)
	require.NoError(t, err)

	// Run scan again - this should clean up the orphaned file
	err = tc.runScan()
	require.NoError(t, err)

	// Verify file is gone from DB
	_, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, fileID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify book is gone from DB (was the last file)
	_, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify no files or books remain
	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

func TestProcessScanJob_CleansUpOrphanedFile_KeepsBookWithOtherFiles(t *testing.T) {
	tc := newTestContextWithSearchService(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Multi File Book")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})
	orphanPath := testgen.GenerateEPUB(t, bookDir, "orphan.epub", testgen.EPUBOptions{
		Title:   "Multi File Book",
		Authors: []string{"Test Author"},
	})

	// Run initial scan to create book and files in DB
	err := tc.runScan()
	require.NoError(t, err)

	// Verify both files were created
	files := tc.listFiles()
	require.Len(t, files, 2)
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID

	// Find the file that will be orphaned (orphan.epub)
	var orphanFileID int
	for _, f := range files {
		if filepath.Base(f.Filepath) == "orphan.epub" {
			orphanFileID = f.ID
			break
		}
	}
	require.NotZero(t, orphanFileID, "orphan.epub should exist")

	// Delete the physical file from disk
	err = os.Remove(orphanPath)
	require.NoError(t, err)

	// Run scan again - this should clean up the orphaned file but keep the book
	err = tc.runScan()
	require.NoError(t, err)

	// Verify orphaned file is gone from DB
	_, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, orphanFileID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Verify book still exists (has other file)
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.NotNil(t, book)

	// Verify one file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, "keep.epub", filepath.Base(remainingFiles[0].Filepath))
}

// =============================================================================
// Regression tests for unified scan refactor
// These tests ensure functionality that was previously missing doesn't regress.
// =============================================================================

// TestScanFileCore_FileLevelFields_Publisher verifies that publisher metadata is extracted
// from files and stored on the file record (regression test for file-level fields).
func TestScanFileCore_FileLevelFields_Publisher(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with publisher
	metadata := &mediafile.ParsedMetadata{
		Publisher:  "Test Publisher",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify publisher update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File should have publisher set
	require.NotNil(t, updatedFile.Publisher)
	assert.Equal(t, "Test Publisher", updatedFile.Publisher.Name)
	require.NotNil(t, updatedFile.PublisherSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedFile.PublisherSource)
}

// TestScanFileCore_FileLevelFields_Imprint verifies that imprint metadata is extracted
// from files and stored on the file record (regression test for file-level fields).
func TestScanFileCore_FileLevelFields_Imprint(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with imprint
	metadata := &mediafile.ParsedMetadata{
		Imprint:    "Test Imprint",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify imprint update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File should have imprint set
	require.NotNil(t, updatedFile.Imprint)
	assert.Equal(t, "Test Imprint", updatedFile.Imprint.Name)
	require.NotNil(t, updatedFile.ImprintSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedFile.ImprintSource)
}

// TestScanFileCore_FileLevelFields_ReleaseDate verifies that release date metadata is extracted
// from files and stored on the file record (regression test for file-level fields).
func TestScanFileCore_FileLevelFields_ReleaseDate(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filepath.Join(libraryPath, "test.epub"),
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with release date
	releaseDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	metadata := &mediafile.ParsedMetadata{
		ReleaseDate: &releaseDate,
		DataSource:  models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify release date update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File should have release date set
	require.NotNil(t, updatedFile.ReleaseDate)
	assert.Equal(t, "2024-06-15", updatedFile.ReleaseDate.Format("2006-01-02"))
	require.NotNil(t, updatedFile.ReleaseDateSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedFile.ReleaseDateSource)
}

// TestScanFileCore_SidecarReading_BookTitle verifies that book sidecar files are read
// and their values override filepath-sourced data (regression test for sidecar reading).
func TestScanFileCore_SidecarReading_BookTitle(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with a real temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book with filepath source (lower priority than sidecar)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Filepath Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Filepath Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "test.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Create book sidecar file with different title
	bookSidecarPath := filepath.Join(bookDir, "Test Book.metadata.json")
	sidecarContent := `{"version":1,"title":"Sidecar Title"}`
	err = os.WriteFile(bookSidecarPath, []byte(sidecarContent), 0644)
	require.NoError(t, err)

	// Empty metadata (to test that sidecar is read and applied)
	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should be updated from sidecar
	assert.Equal(t, "Sidecar Title", result.Book.Title)
	assert.Equal(t, models.DataSourceSidecar, result.Book.TitleSource)
}

// TestScanFileCore_SidecarPriority_OverridesFileMetadata verifies that sidecar
// files DO override data from file metadata sources (sidecar has higher priority).
func TestScanFileCore_SidecarPriority_OverridesFileMetadata(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with a real temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book with epub_metadata source (lower priority than sidecar)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "EPUB Title",
		TitleSource:  models.DataSourceEPUBMetadata,
		SortTitle:    "EPUB Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "test.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Create book sidecar file with different title
	// Sidecar has higher priority than epub_metadata
	bookSidecarPath := filepath.Join(bookDir, "Test Book.metadata.json")
	sidecarContent := `{"version":1,"title":"Sidecar Title"}`
	err = os.WriteFile(bookSidecarPath, []byte(sidecarContent), 0644)
	require.NoError(t, err)

	// Empty metadata (to isolate sidecar behavior)
	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore without forceRefresh
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should be updated from sidecar (sidecar overrides file metadata)
	assert.Equal(t, "Sidecar Title", result.Book.Title)
	assert.Equal(t, models.DataSourceSidecar, result.Book.TitleSource)
}

// TestScanFileCore_SidecarPriority_OverridesLowerPriority verifies that sidecar
// files DO override data from sources with lower priority (regression test for
// sidecar priority logic).
func TestScanFileCore_SidecarPriority_OverridesLowerPriority(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with a real temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book with filepath source (lower priority than sidecar)
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Filepath Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Filepath Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "test.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Create book sidecar file with different title
	bookSidecarPath := filepath.Join(bookDir, "Test Book.metadata.json")
	sidecarContent := `{"version":1,"title":"Sidecar Title"}`
	err = os.WriteFile(bookSidecarPath, []byte(sidecarContent), 0644)
	require.NoError(t, err)

	// Empty metadata (to isolate sidecar behavior)
	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore without forceRefresh
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Book title should be updated from sidecar (sidecar overrides lower priority filepath)
	assert.Equal(t, "Sidecar Title", result.Book.Title)
	assert.Equal(t, models.DataSourceSidecar, result.Book.TitleSource)
}

// TestScanFileByID_CoverRecovery verifies that missing cover files are re-extracted
// from the media file during resync (regression test for cover recovery logic).
func TestScanFileByID_CoverRecovery(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create a library with temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with an EPUB file that has a cover
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Cover Book")
	testgen.GenerateEPUB(t, bookDir, "test.epub", testgen.EPUBOptions{
		Title:    "Cover Book",
		Authors:  []string{"Test Author"},
		HasCover: true,
	})

	// Run initial scan to create book and file in DB (which also extracts the cover)
	err := tc.runScan()
	require.NoError(t, err)

	// Verify file was created
	files := tc.listFiles()
	require.Len(t, files, 1)
	fileID := files[0].ID

	// Verify cover file was created
	coverBaseName := "test.epub.cover"
	existingCoverPath := fileutils.CoverExistsWithBaseName(bookDir, coverBaseName)
	require.NotEmpty(t, existingCoverPath, "cover file should exist after initial scan")

	// Delete the cover file from disk (simulating missing cover)
	err = os.Remove(existingCoverPath)
	require.NoError(t, err)

	// Verify cover is gone
	existingCoverPath = fileutils.CoverExistsWithBaseName(bookDir, coverBaseName)
	require.Empty(t, existingCoverPath, "cover file should be deleted")

	// Call Scan with FileID to trigger resync (which should recover the cover)
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FileID: fileID,
	})

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify cover file was recovered
	recoveredCoverPath := fileutils.CoverExistsWithBaseName(bookDir, coverBaseName)
	assert.NotEmpty(t, recoveredCoverPath, "cover file should be recovered after resync")
}

// TestScanFileCore_SidecarReading_FileLevelFields verifies that file sidecar files
// are read and their values (publisher, imprint, release date) override filepath-sourced
// data (regression test for sidecar reading of file-level fields).
func TestScanFileCore_SidecarReading_FileLevelFields(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with a real temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "test.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Create file sidecar with publisher, imprint, and release date
	fileSidecarPath := filePath + ".metadata.json"
	sidecarContent := `{"version":1,"publisher":"Sidecar Publisher","imprint":"Sidecar Imprint","release_date":"2024-12-25"}`
	err = os.WriteFile(fileSidecarPath, []byte(sidecarContent), 0644)
	require.NoError(t, err)

	// Empty metadata (to test that sidecar is read and applied)
	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify sidecar values were applied
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Publisher should be set from sidecar
	require.NotNil(t, updatedFile.Publisher)
	assert.Equal(t, "Sidecar Publisher", updatedFile.Publisher.Name)

	// Imprint should be set from sidecar
	require.NotNil(t, updatedFile.Imprint)
	assert.Equal(t, "Sidecar Imprint", updatedFile.Imprint.Name)

	// Release date should be set from sidecar
	require.NotNil(t, updatedFile.ReleaseDate)
	assert.Equal(t, "2024-12-25", updatedFile.ReleaseDate.Format("2006-01-02"))
}

// =============================================================================
// file.name from metadata title tests (regression tests for M4B/EPUB file name)
// =============================================================================

// TestScanFileCore_FileName_M4B verifies that M4B files get their file.name set
// from the metadata title (regression test for file.name update from metadata).
func TestScanFileCore_FileName_M4B(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "audiobook.m4b")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with title
	metadata := &mediafile.ParsedMetadata{
		Title:      "Audiobook Title From Metadata",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be set from metadata title
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "Audiobook Title From Metadata", *updatedFile.Name)
	require.NotNil(t, updatedFile.NameSource)
	assert.Equal(t, models.DataSourceM4BMetadata, *updatedFile.NameSource)
}

// TestScanFileCore_FileName_EPUB verifies that EPUB files get their file.name set
// from the metadata title (regression test for file.name update from metadata).
func TestScanFileCore_FileName_EPUB(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file for the book
	filePath := filepath.Join(bookDir, "book.epub")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with title
	metadata := &mediafile.ParsedMetadata{
		Title:      "EPUB Title From Metadata",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be set from metadata title
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "EPUB Title From Metadata", *updatedFile.Name)
	require.NotNil(t, updatedFile.NameSource)
	assert.Equal(t, models.DataSourceEPUBMetadata, *updatedFile.NameSource)
}

// TestScanFileCore_FileName_ForceRefresh_OverridesExisting verifies that forceRefresh
// causes file.name to be updated even if it already has a value from the same source
// (regression test for forceRefresh behavior with file.name).
func TestScanFileCore_FileName_ForceRefresh_OverridesExisting(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create file with existing name from same source
	existingName := "Old Name"
	existingNameSource := models.DataSourceM4BMetadata
	filePath := filepath.Join(bookDir, "audiobook.m4b")
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
		Name:          &existingName,
		NameSource:    &existingNameSource,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Metadata with different title
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Name From Refresh",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore with forceRefresh
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, true, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be updated to new value with forceRefresh
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "New Name From Refresh", *updatedFile.Name)
}

// =============================================================================
// File organization tests (file rename on disk when name changes)
// =============================================================================

// TestScanFileCore_FileOrganization_RenamesFileOnDisk verifies that when file.name
// changes during resync and the library has OrganizeFileStructure enabled, the actual
// file on disk is renamed (regression test for file organization during resync).
func TestScanFileCore_FileOrganization_RenamesFileOnDisk(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a book directory with author
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")

	// Create a book with author
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author for the book
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create actual M4B file on disk with old name
	oldFilePath := filepath.Join(bookDir, "Old Title.m4b")
	testgen.GenerateM4B(t, bookDir, "Old Title.m4b", testgen.M4BOptions{
		Title: "New Title From Metadata",
	})

	// Create file record in DB
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      oldFilePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata with new title
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Title From Metadata",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore - this should update file.name and rename the file on disk
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify path update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, result.File.ID)
	require.NoError(t, err)

	// File name should be set from metadata
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "New Title From Metadata", *updatedFile.Name)

	// File path should be updated (file was renamed on disk)
	// When book title changes, BOTH the book folder AND the file are renamed
	// Format is "[Author] Title/[Author] Title.ext"
	expectedNewBookDir := filepath.Join(libraryPath, "[Test Author] New Title From Metadata")
	expectedNewPath := filepath.Join(expectedNewBookDir, "New Title From Metadata.m4b")
	assert.Equal(t, expectedNewPath, updatedFile.Filepath)

	// Old file should not exist
	_, err = os.Stat(oldFilePath)
	assert.True(t, os.IsNotExist(err), "old file should not exist after rename")

	// New file should exist
	_, err = os.Stat(expectedNewPath)
	require.NoError(t, err, "new file should exist after rename")

	// Book filepath should also be updated
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Equal(t, expectedNewBookDir, updatedBook.Filepath)
}

// TestScanFileCore_FileOrganization_SkipsWhenOrganizeDisabled verifies that when
// OrganizeFileStructure is disabled, files are NOT renamed on disk even when file.name
// changes (regression test for file organization opt-in behavior).
func TestScanFileCore_FileOrganization_SkipsWhenOrganizeDisabled(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library WITHOUT OrganizeFileStructure (default is false)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create actual M4B file on disk
	oldFilePath := filepath.Join(bookDir, "Old Title.m4b")
	testgen.GenerateM4B(t, bookDir, "Old Title.m4b", testgen.M4BOptions{
		Title: "New Title From Metadata",
	})

	// Create file record in DB
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      oldFilePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata with new title
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Title From Metadata",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore - this should update file.name but NOT rename the file on disk
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, result.File.ID)
	require.NoError(t, err)

	// File name should be updated in DB
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "New Title From Metadata", *updatedFile.Name)

	// File path should NOT be updated (OrganizeFileStructure is disabled)
	assert.Equal(t, oldFilePath, updatedFile.Filepath)

	// Original file should still exist at old path
	_, err = os.Stat(oldFilePath)
	assert.NoError(t, err, "original file should still exist when OrganizeFileStructure is disabled")
}

// TestScanFileCore_FileOrganization_SidecarNameChange verifies that when file.name
// is updated from a sidecar file and the library has OrganizeFileStructure enabled,
// the actual file on disk is renamed (regression test for sidecar-triggered file organization).
func TestScanFileCore_FileOrganization_SidecarNameChange(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a book directory with author
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Test Book")

	// Create a book with author
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author for the book
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create actual M4B file on disk with old name
	oldFilePath := filepath.Join(bookDir, "Old Title.m4b")
	testgen.GenerateM4B(t, bookDir, "Old Title.m4b", testgen.M4BOptions{})

	// Create file sidecar with new name
	fileSidecarPath := filepath.Join(bookDir, "Old Title.m4b.metadata.json")
	sidecarContent := `{"version":1,"name":"Sidecar New Title"}`
	err = os.WriteFile(fileSidecarPath, []byte(sidecarContent), 0644)
	require.NoError(t, err)

	// Create file record in DB (no name set, so filepath source)
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      oldFilePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Empty metadata (sidecar will provide the name)
	metadata := &mediafile.ParsedMetadata{
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore - this should update file.name from sidecar and rename the file on disk
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify path update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, result.File.ID)
	require.NoError(t, err)

	// File name should be set from sidecar
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "Sidecar New Title", *updatedFile.Name)

	// File path should be updated (file was renamed on disk)
	// Format is "[Author] Title.ext"
	expectedNewPath := filepath.Join(bookDir, "Sidecar New Title.m4b")
	assert.Equal(t, expectedNewPath, updatedFile.Filepath)

	// Old file should not exist
	_, err = os.Stat(oldFilePath)
	assert.True(t, os.IsNotExist(err), "old file should not exist after rename")

	// New file should exist
	_, err = os.Stat(expectedNewPath)
	require.NoError(t, err, "new file should exist after rename")
}

// TestScanFileCore_FileOrganization_StripsAuthorPrefixOnResync verifies that when a file
// has an author prefix in its filename (like "[Author] Title.epub") but the database
// file.Name already matches the title, a resync will still strip the author prefix
// from the filename on disk. This ensures existing files migrate to the new naming convention.
func TestScanFileCore_FileOrganization_StripsAuthorPrefixOnResync(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a book directory with author prefix
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] My Book")

	// Create a book
	book := &models.Book{
		LibraryID:    1,
		Filepath:     bookDir,
		Title:        "My Book",
		TitleSource:  models.DataSourceEPUBMetadata,
		SortTitle:    "My Book",
		AuthorSource: models.DataSourceEPUBMetadata,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author for the book
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create actual EPUB file on disk WITH author prefix (old naming convention)
	oldFilePath := filepath.Join(bookDir, "[Test Author] My Book.epub")
	testgen.GenerateEPUB(t, bookDir, "[Test Author] My Book.epub", testgen.EPUBOptions{
		Title:   "My Book",
		Authors: []string{"Test Author"},
	})

	// Create file record in DB with name already set to just the title (no author prefix)
	// This simulates a file that was already scanned and has the correct DB name,
	// but the file on disk still has the old naming convention with author prefix
	fileName := "My Book"
	fileNameSource := models.DataSourceEPUBMetadata
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      oldFilePath,
		FileType:      models.FileTypeEPUB,
		FilesizeBytes: 1000,
		Name:          &fileName,
		NameSource:    &fileNameSource,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata matches what's already in the DB (title = "My Book")
	metadata := &mediafile.ParsedMetadata{
		Title:      "My Book",
		DataSource: models.DataSourceEPUBMetadata,
	}

	// Call scanFileCore with isResync=true
	// Even though the DB name matches, the file on disk should be renamed
	// to strip the author prefix
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify path update
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, result.File.ID)
	require.NoError(t, err)

	// File name should still be the title (no change in DB)
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "My Book", *updatedFile.Name)

	// File path should be updated - author prefix stripped from filename
	expectedNewPath := filepath.Join(bookDir, "My Book.epub")
	assert.Equal(t, expectedNewPath, updatedFile.Filepath, "file path should be updated to strip author prefix")

	// Old file (with author prefix) should not exist
	_, err = os.Stat(oldFilePath)
	assert.True(t, os.IsNotExist(err), "old file with author prefix should not exist after rename")

	// New file (without author prefix) should exist
	_, err = os.Stat(expectedNewPath)
	require.NoError(t, err, "new file without author prefix should exist after rename")
}

// =============================================================================
// Book organization tests (book folder rename on disk when title changes)
// =============================================================================

// TestScanFileCore_BookOrganization_RenamesFolderOnDisk verifies that when book title
// changes during resync (not full scan) and the library has OrganizeFileStructure enabled,
// the book folder is renamed on disk (regression test for book organization during resync).
func TestScanFileCore_BookOrganization_RenamesFolderOnDisk(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a book directory with old name
	oldBookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Old Title")

	// Create a book with old title
	book := &models.Book{
		LibraryID:    1,
		Filepath:     oldBookDir,
		Title:        "Old Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Old Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author for the book
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create actual M4B file on disk
	filePath := filepath.Join(oldBookDir, "audiobook.m4b")
	testgen.GenerateM4B(t, oldBookDir, "audiobook.m4b", testgen.M4BOptions{
		Title: "New Title From Metadata",
	})

	// Create file record in DB
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata with new title
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Title From Metadata",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore with isResync=true (simulating a resync, not a full scan)
	// This should trigger book organization because title changed
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify path update
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book title should be updated
	assert.Equal(t, "New Title From Metadata", updatedBook.Title)

	// Book folder should be renamed
	expectedNewBookDir := filepath.Join(libraryPath, "[Test Author] New Title From Metadata")
	assert.Equal(t, expectedNewBookDir, updatedBook.Filepath)

	// Old folder should not exist
	_, err = os.Stat(oldBookDir)
	assert.True(t, os.IsNotExist(err), "old book folder should not exist after rename")

	// New folder should exist
	_, err = os.Stat(expectedNewBookDir)
	require.NoError(t, err, "new book folder should exist after rename")
}

// TestScanFileCore_BookOrganization_SkippedDuringFullScan verifies that book organization
// is skipped during full scans (when jobLog is not nil) to avoid renaming directories
// while other files are still being discovered/processed.
func TestScanFileCore_BookOrganization_SkippedDuringFullScan(t *testing.T) {
	tc := newTestContext(t)

	// Setup: Create library with OrganizeFileStructure enabled
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a book directory with old name
	oldBookDir := testgen.CreateSubDir(t, libraryPath, "[Test Author] Old Title")

	// Create a book with old title
	book := &models.Book{
		LibraryID:    1,
		Filepath:     oldBookDir,
		Title:        "Old Title",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Old Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author for the book
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create actual M4B file on disk
	filePath := filepath.Join(oldBookDir, "audiobook.m4b")
	testgen.GenerateM4B(t, oldBookDir, "audiobook.m4b", testgen.M4BOptions{
		Title: "New Title From Metadata",
	})

	// Create file record in DB
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      filePath,
		FileType:      models.FileTypeM4B,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata with new title
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Title From Metadata",
		DataSource: models.DataSourceM4BMetadata,
	}

	// Call scanFileCore with isResync=false (simulating a full scan)
	// This should NOT trigger book organization
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, false)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify path was NOT updated
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book title should be updated
	assert.Equal(t, "New Title From Metadata", updatedBook.Title)

	// But book folder should NOT be renamed (organization skipped during full scan)
	assert.Equal(t, oldBookDir, updatedBook.Filepath)

	// Old folder should still exist
	_, err = os.Stat(oldBookDir)
	require.NoError(t, err, "old book folder should still exist when organization is skipped")
}

// TestScanFileCore_Supplement_SetsFileNameFromFilename verifies that supplement files
// (like PDFs) can be rescanned and get their file.Name set from the filename on disk.
func TestScanFileCore_Supplement_SetsFileNameFromFilename(t *testing.T) {
	tc := newTestContext(t)
	tc.createLibrary([]string{"/library"})

	// Create book
	book := &models.Book{
		LibraryID:    1,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/library/[Author] Test Book",
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create a supplement file (PDF)
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      "/library/[Author] Test Book/companion-guide.pdf",
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata derived from filename (as would happen in scanFileByID for supplements)
	metadata := &mediafile.ParsedMetadata{
		Title:      "companion-guide",
		DataSource: models.DataSourceFilepath,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify name was set
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be set from the filename
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "companion-guide", *updatedFile.Name)
}

// TestScanFileCore_Supplement_DoesNotUpdateBookMetadata verifies that supplements
// don't update book-level metadata (title, authors, series).
func TestScanFileCore_Supplement_DoesNotUpdateBookMetadata(t *testing.T) {
	tc := newTestContext(t)
	tc.createLibrary([]string{"/library"})

	// Create book with existing title and author
	book := &models.Book{
		LibraryID:    1,
		Title:        "Original Book Title",
		TitleSource:  models.DataSourceFilepath,
		Filepath:     "/library/[Author] Original Book Title",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Original Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create a supplement file (PDF)
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      "/library/[Author] Original Book Title/different-title.pdf",
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata derived from filename - has a different "title" than the book
	metadata := &mediafile.ParsedMetadata{
		Title:      "different-title",
		DataSource: models.DataSourceFilepath,
	}

	// Call scanFileCore
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload book to verify it was NOT modified
	updatedBook, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Book title should NOT have changed
	assert.Equal(t, "Original Book Title", updatedBook.Title)

	// Authors should NOT have changed
	assert.Len(t, updatedBook.Authors, 1)
	assert.Equal(t, "Original Author", updatedBook.Authors[0].Person.Name)

	// File name should have been set though
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "different-title", *updatedFile.Name)
}

// TestScanFileCore_Supplement_FileOrganization_NoAuthorPrefix verifies that supplements
// are renamed WITHOUT the author prefix to avoid duplication like "[Author] [Author] name.pdf".
func TestScanFileCore_Supplement_FileOrganization_NoAuthorPrefix(t *testing.T) {
	tc := newTestContext(t)

	// Create a temp directory for the library
	tempDir := t.TempDir()
	libraryPath := filepath.Join(tempDir, "library")
	bookDir := filepath.Join(libraryPath, "[Test Author] Test Book")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	// Create library with OrganizeFileStructure enabled
	tc.createLibraryWithOptions([]string{libraryPath}, true)

	// Create a real supplement file on disk
	oldFilePath := filepath.Join(bookDir, "old-supplement-name.pdf")
	require.NoError(t, os.WriteFile(oldFilePath, []byte("PDF content"), 0644))

	// Create book
	book := &models.Book{
		LibraryID:    1,
		Title:        "Test Book",
		TitleSource:  models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     bookDir,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// Create author
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Test Author", 1)
	require.NoError(t, err)
	author := &models.Author{
		BookID:    book.ID,
		PersonID:  person.ID,
		SortOrder: 1,
	}
	err = tc.bookService.CreateAuthor(tc.ctx, author)
	require.NoError(t, err)

	// Reload book with relations
	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Create a supplement file (PDF)
	file := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      oldFilePath,
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 1000,
	}
	err = tc.bookService.CreateFile(tc.ctx, file)
	require.NoError(t, err)

	// Reload file with relations
	file, err = tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// Metadata with a new name for the supplement
	metadata := &mediafile.ParsedMetadata{
		Title:      "New Supplement Name",
		DataSource: models.DataSourceFilepath,
	}

	// Call scanFileCore - this should update file.Name and rename the file WITHOUT author prefix
	result, err := tc.worker.scanFileCore(tc.ctx, file, book, metadata, false, true)

	// Should succeed
	require.NoError(t, err)
	require.NotNil(t, result)

	// Reload file to verify it was renamed
	updatedFile, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, file.ID)
	require.NoError(t, err)

	// File name should be updated
	require.NotNil(t, updatedFile.Name)
	assert.Equal(t, "New Supplement Name", *updatedFile.Name)

	// The file should be renamed on disk WITHOUT the author prefix
	// Should be "New Supplement Name.pdf", NOT "[Test Author] New Supplement Name.pdf"
	expectedNewPath := filepath.Join(bookDir, "New Supplement Name.pdf")
	assert.Equal(t, expectedNewPath, updatedFile.Filepath, "Supplement should not have author prefix in filename")

	// Verify old file no longer exists
	_, err = os.Stat(oldFilePath)
	assert.True(t, os.IsNotExist(err), "old file should no longer exist")

	// Verify new file exists
	_, err = os.Stat(expectedNewPath)
	require.NoError(t, err, "new file should exist at expected path")
}
