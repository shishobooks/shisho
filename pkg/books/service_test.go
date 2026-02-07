package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestRetrieveBook_LoadsChaptersForEachFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create chapters for the file directly in the database
	// Insert in order with sort_order 0, 1, 2 to test ordering
	now := time.Now()
	chaptersData := []*models.Chapter{
		{FileID: file.ID, SortOrder: 0, Title: "Chapter 1", StartTimestampMs: ptrInt64(0), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 1, Title: "Chapter 2", StartTimestampMs: ptrInt64(30000), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 2, Title: "Chapter 3", StartTimestampMs: ptrInt64(60000), CreatedAt: now, UpdatedAt: now},
	}
	for _, ch := range chaptersData {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBook
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Assert file.Chapters is populated
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")
	require.Len(t, retrievedBook.Files[0].Chapters, 3, "File should have 3 chapters")

	// Assert chapters are ordered by sort_order (0, 1, 2)
	assert.Equal(t, "Chapter 1", retrievedBook.Files[0].Chapters[0].Title, "First chapter by sort_order")
	assert.Equal(t, "Chapter 2", retrievedBook.Files[0].Chapters[1].Title, "Second chapter by sort_order")
	assert.Equal(t, "Chapter 3", retrievedBook.Files[0].Chapters[2].Title, "Third chapter by sort_order")
}

func TestRetrieveBook_LoadsNestedChaptersViaChildren(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a parent chapter (no parent_id)
	now := time.Now()
	parentChapter := &models.Chapter{
		FileID:           file.ID,
		SortOrder:        0,
		Title:            "Part 1",
		StartTimestampMs: ptrInt64(0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = db.NewInsert().Model(parentChapter).Exec(ctx)
	require.NoError(t, err)

	// Create child chapters with parent_id pointing to parent
	childChapters := []*models.Chapter{
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        0,
			Title:            "Chapter 1",
			StartTimestampMs: ptrInt64(0),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        1,
			Title:            "Chapter 2",
			StartTimestampMs: ptrInt64(30000),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	for _, ch := range childChapters {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBook
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)

	// Assert file has chapters loaded
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")

	// Find the parent chapter (the one with no ParentID)
	var loadedParent *models.Chapter
	for _, ch := range retrievedBook.Files[0].Chapters {
		if ch.ParentID == nil && ch.Title == "Part 1" {
			loadedParent = ch
			break
		}
	}
	require.NotNil(t, loadedParent, "Parent chapter should be found in file.Chapters")

	// Assert parent chapter's Children field is populated
	require.NotNil(t, loadedParent.Children, "Parent chapter should have Children loaded")
	require.Len(t, loadedParent.Children, 2, "Parent should have 2 child chapters")

	// Assert Children have correct data and are ordered by sort_order
	assert.Equal(t, "Chapter 1", loadedParent.Children[0].Title, "First child by sort_order")
	assert.Equal(t, "Chapter 2", loadedParent.Children[1].Title, "Second child by sort_order")

	// Assert child chapters have correct parent reference
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[0].ParentID, "First child has correct parent_id")
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[1].ParentID, "Second child has correct parent_id")
}

func TestRetrieveBookByFilePath_LoadsChaptersForEachFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	library, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	testFilePath := "/test/audiobook.m4b"
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      testFilePath,
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create chapters for the file directly in the database
	// Insert in order with sort_order 0, 1, 2 to test ordering
	now := time.Now()
	chaptersData := []*models.Chapter{
		{FileID: file.ID, SortOrder: 0, Title: "Chapter 1", StartTimestampMs: ptrInt64(0), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 1, Title: "Chapter 2", StartTimestampMs: ptrInt64(30000), CreatedAt: now, UpdatedAt: now},
		{FileID: file.ID, SortOrder: 2, Title: "Chapter 3", StartTimestampMs: ptrInt64(60000), CreatedAt: now, UpdatedAt: now},
	}
	for _, ch := range chaptersData {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBookByFilePath
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBookByFilePath(ctx, testFilePath, library.ID)
	require.NoError(t, err)

	// Assert file.Chapters is populated
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")
	require.Len(t, retrievedBook.Files[0].Chapters, 3, "File should have 3 chapters")

	// Assert chapters are ordered by sort_order (0, 1, 2)
	assert.Equal(t, "Chapter 1", retrievedBook.Files[0].Chapters[0].Title, "First chapter by sort_order")
	assert.Equal(t, "Chapter 2", retrievedBook.Files[0].Chapters[1].Title, "Second chapter by sort_order")
	assert.Equal(t, "Chapter 3", retrievedBook.Files[0].Chapters[2].Title, "Third chapter by sort_order")
}

func TestRetrieveBookByFilePath_LoadsNestedChapters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library and book
	library, book := setupTestLibraryAndBook(t, db)

	// Create a test file for the book
	testFilePath := "/test/audiobook.m4b"
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      testFilePath,
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a parent chapter (no parent_id)
	now := time.Now()
	parentChapter := &models.Chapter{
		FileID:           file.ID,
		SortOrder:        0,
		Title:            "Part 1",
		StartTimestampMs: ptrInt64(0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = db.NewInsert().Model(parentChapter).Exec(ctx)
	require.NoError(t, err)

	// Create child chapters with parent_id pointing to parent
	childChapters := []*models.Chapter{
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        0,
			Title:            "Chapter 1",
			StartTimestampMs: ptrInt64(0),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			FileID:           file.ID,
			ParentID:         &parentChapter.ID,
			SortOrder:        1,
			Title:            "Chapter 2",
			StartTimestampMs: ptrInt64(30000),
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}
	for _, ch := range childChapters {
		_, err := db.NewInsert().Model(ch).Exec(ctx)
		require.NoError(t, err)
	}

	// Call RetrieveBookByFilePath
	bookSvc := NewService(db)
	retrievedBook, err := bookSvc.RetrieveBookByFilePath(ctx, testFilePath, library.ID)
	require.NoError(t, err)

	// Assert file has chapters loaded
	require.Len(t, retrievedBook.Files, 1, "Book should have one file")
	require.NotNil(t, retrievedBook.Files[0].Chapters, "File should have chapters loaded")

	// Find the parent chapter (the one with no ParentID)
	var loadedParent *models.Chapter
	for _, ch := range retrievedBook.Files[0].Chapters {
		if ch.ParentID == nil && ch.Title == "Part 1" {
			loadedParent = ch
			break
		}
	}
	require.NotNil(t, loadedParent, "Parent chapter should be found in file.Chapters")

	// Assert parent chapter's Children field is populated
	require.NotNil(t, loadedParent.Children, "Parent chapter should have Children loaded")
	require.Len(t, loadedParent.Children, 2, "Parent should have 2 child chapters")

	// Assert Children have correct data and are ordered by sort_order
	assert.Equal(t, "Chapter 1", loadedParent.Children[0].Title, "First child by sort_order")
	assert.Equal(t, "Chapter 2", loadedParent.Children[1].Title, "Second child by sort_order")

	// Assert child chapters have correct parent reference
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[0].ParentID, "First child has correct parent_id")
	assert.Equal(t, parentChapter.ID, *loadedParent.Children[1].ParentID, "Second child has correct parent_id")
}

// ptrInt64 is a helper to create a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}

func TestDeleteBookAndFiles_DeletesBookFilesAndDiskFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library with OrganizeFileStructure=false (files at root level)
	library := &models.Library{
		Name:                     "Test Library",
		OrganizeFileStructure:    false,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        filepath.Join(tmpDir, "test-book"),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create test files on disk
	mainFilePath := filepath.Join(tmpDir, "test.epub")
	err = os.WriteFile(mainFilePath, []byte("test content"), 0644)
	require.NoError(t, err)

	coverPath := filepath.Join(tmpDir, "test.cover.jpg")
	err = os.WriteFile(coverPath, []byte("cover content"), 0644)
	require.NoError(t, err)

	sidecarPath := filepath.Join(tmpDir, "test.epub.metadata.json")
	err = os.WriteFile(sidecarPath, []byte("{}"), 0644)
	require.NoError(t, err)

	// Create file record
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           mainFilePath,
		FilesizeBytes:      12,
		CoverImageFilename: &coverPath,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesDeleted)

	// Verify book is deleted from DB
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file is deleted from DB
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify files are deleted from disk
	_, err = os.Stat(mainFilePath)
	assert.True(t, os.IsNotExist(err), "main file should be deleted")
	_, err = os.Stat(coverPath)
	assert.True(t, os.IsNotExist(err), "cover should be deleted")
	_, err = os.Stat(sidecarPath)
	assert.True(t, os.IsNotExist(err), "sidecar should be deleted")
}

func TestDeleteBookAndFiles_OrganizedStructure_DeletesEntireDirectory(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for library
	tmpDir := t.TempDir()

	// Create library with OrganizeFileStructure=true (files in book directories)
	library := &models.Library{
		Name:                     "Test Library",
		OrganizeFileStructure:    true,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book directory
	bookDir := filepath.Join(tmpDir, "Test Book")
	err = os.MkdirAll(bookDir, 0755)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create test files inside the book directory
	mainFilePath := filepath.Join(bookDir, "test.epub")
	err = os.WriteFile(mainFilePath, []byte("test content"), 0644)
	require.NoError(t, err)

	coverPath := filepath.Join(bookDir, "test.cover.jpg")
	err = os.WriteFile(coverPath, []byte("cover content"), 0644)
	require.NoError(t, err)

	sidecarPath := filepath.Join(bookDir, "test.epub.metadata.json")
	err = os.WriteFile(sidecarPath, []byte("{}"), 0644)
	require.NoError(t, err)

	// Create file record
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           mainFilePath,
		FilesizeBytes:      12,
		CoverImageFilename: &coverPath,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesDeleted)

	// Verify book is deleted from DB
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file is deleted from DB
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify the entire book directory is deleted
	_, err = os.Stat(bookDir)
	assert.True(t, os.IsNotExist(err), "book directory should be deleted")
}

func TestDeleteFileAndCleanup_DeletesFileAndKeepsBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files - one to delete, one to keep
	file1Path := filepath.Join(tmpDir, "test1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file1Path,
		FilesizeBytes: 8,
	}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(tmpDir, "test2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file2Path,
		FilesizeBytes: 8,
	}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Delete first file (pass nil for supportedTypes since another main file exists)
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file1.ID, library, nil, nil)
	require.NoError(t, err)

	assert.False(t, result.BookDeleted, "book should not be deleted")

	// Verify file1 is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file1.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file1 is deleted from disk
	_, err = os.Stat(file1Path)
	assert.True(t, os.IsNotExist(err))

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify file2 still exists
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file2.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestDeleteFileAndCleanup_DeletesBookWhenLastFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create single file
	filePath := filepath.Join(tmpDir, "test.epub")
	err = os.WriteFile(filePath, []byte("content"), 0644)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: 7,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Delete the only file (pass nil for supportedTypes since there are no supplements)
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library, nil, nil)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify book is deleted from DB
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestDeleteFileAndCleanup_CleansUpDirectoryWithIgnoredFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for the book
	bookDir := t.TempDir()

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		OrganizeFileStructure:    true,
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create single main file
	filePath := filepath.Join(bookDir, "test.epub")
	err = os.WriteFile(filePath, []byte("content"), 0644)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: 7,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a .DS_Store file (simulating macOS system file)
	dsStorePath := filepath.Join(bookDir, ".DS_Store")
	err = os.WriteFile(dsStorePath, []byte("fake ds_store"), 0644)
	require.NoError(t, err)

	// Verify .DS_Store exists before deletion
	_, err = os.Stat(dsStorePath)
	require.NoError(t, err, ".DS_Store should exist before deletion")

	// Delete the only file with ignored patterns that include .DS_Store
	bookSvc := NewService(db)
	ignoredPatterns := []string{".*", ".DS_Store", "Thumbs.db"}
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library, nil, ignoredPatterns)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify book directory is completely removed (including .DS_Store)
	_, err = os.Stat(bookDir)
	assert.True(t, os.IsNotExist(err), "book directory should be completely removed, including ignored files")
}

func TestDeleteFileAndCleanup_FileNotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library (needed as parameter)
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Try to delete a non-existent file
	bookSvc := NewService(db)
	_, err = bookSvc.DeleteFileAndCleanup(ctx, 99999, library, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteBooksAndFiles_DeletesMultipleBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create two books with files (each needs a unique filepath)
	book1Dir := filepath.Join(tmpDir, "book1")
	err = os.MkdirAll(book1Dir, 0755)
	require.NoError(t, err)
	book1 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 1",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 1",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        book1Dir,
	}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	book2Dir := filepath.Join(tmpDir, "book2")
	err = os.MkdirAll(book2Dir, 0755)
	require.NoError(t, err)
	book2 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Book 2",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Book 2",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        book2Dir,
	}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	// Create files
	file1Path := filepath.Join(book1Dir, "book1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book1.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(book2Dir, "book2a.epub")
	err = os.WriteFile(file2Path, []byte("content2a"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 9}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	file3Path := filepath.Join(book2Dir, "book2b.epub")
	err = os.WriteFile(file3Path, []byte("content2b"), 0644)
	require.NoError(t, err)
	file3 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file3Path, FilesizeBytes: 9}
	_, err = db.NewInsert().Model(file3).Exec(ctx)
	require.NoError(t, err)

	// Delete both books
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteBooksAndFiles(ctx, []int{book1.ID, book2.ID}, library)
	require.NoError(t, err)

	assert.Equal(t, 2, result.BooksDeleted)
	assert.Equal(t, 3, result.FilesDeleted)

	// Verify all books deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify all files deleted from disk
	_, err = os.Stat(file1Path)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(file2Path)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(file3Path)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFileAndCleanup_PromotesSupplementWhenLastMainDeleted(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create main file (epub)
	mainFilePath := filepath.Join(tmpDir, "book.epub")
	err = os.WriteFile(mainFilePath, []byte("main content"), 0644)
	require.NoError(t, err)

	mainFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      mainFilePath,
		FilesizeBytes: 12,
	}
	_, err = db.NewInsert().Model(mainFile).Exec(ctx)
	require.NoError(t, err)

	// Create supplement file (cbz - supported type)
	supplementFilePath := filepath.Join(tmpDir, "supplement.cbz")
	err = os.WriteFile(supplementFilePath, []byte("supplement content"), 0644)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleSupplement,
		Filepath:      supplementFilePath,
		FilesizeBytes: 18,
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	// Define supported types (native types)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
	}

	// Delete the main file
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes, nil)
	require.NoError(t, err)

	// Book should NOT be deleted (supplement was promoted)
	assert.False(t, result.BookDeleted, "book should not be deleted when supplement can be promoted")

	// Verify main file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", mainFile.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify supplement was promoted to main
	var updatedSupplement models.File
	err = db.NewSelect().Model(&updatedSupplement).Where("id = ?", supplementFile.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.FileRoleMain, updatedSupplement.FileRole)
}

func TestDeleteFileAndCleanup_DeletesBookWhenOnlyUnsupportedSupplementsRemain(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create main file (epub)
	mainFilePath := filepath.Join(tmpDir, "book.epub")
	err = os.WriteFile(mainFilePath, []byte("main content"), 0644)
	require.NoError(t, err)

	mainFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      mainFilePath,
		FilesizeBytes: 12,
	}
	_, err = db.NewInsert().Model(mainFile).Exec(ctx)
	require.NoError(t, err)

	// Create supplement file (pdf - NOT a supported type)
	supplementFilePath := filepath.Join(tmpDir, "supplement.pdf")
	err = os.WriteFile(supplementFilePath, []byte("supplement content"), 0644)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		Filepath:      supplementFilePath,
		FilesizeBytes: 18,
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	// Define supported types (native types only - pdf NOT included)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
	}

	// Delete the main file
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes, nil)
	require.NoError(t, err)

	// Book SHOULD be deleted (no promotable supplements)
	assert.True(t, result.BookDeleted, "book should be deleted when no supplement can be promoted")

	// Verify both files are deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("book_id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify book is deleted
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify supplement file is deleted from disk
	_, err = os.Stat(supplementFilePath)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFileAndCleanup_PromotesOldestSupplementFirst(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        tmpDir,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create main file (epub)
	mainFilePath := filepath.Join(tmpDir, "book.epub")
	err = os.WriteFile(mainFilePath, []byte("main content"), 0644)
	require.NoError(t, err)

	mainFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      mainFilePath,
		FilesizeBytes: 12,
	}
	_, err = db.NewInsert().Model(mainFile).Exec(ctx)
	require.NoError(t, err)

	// Create older supplement (cbz) - should be promoted
	olderSupplementPath := filepath.Join(tmpDir, "older.cbz")
	err = os.WriteFile(olderSupplementPath, []byte("older supplement"), 0644)
	require.NoError(t, err)

	olderSupplement := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleSupplement,
		Filepath:      olderSupplementPath,
		FilesizeBytes: 16,
		CreatedAt:     time.Now().Add(-2 * time.Hour), // Created 2 hours ago
	}
	_, err = db.NewInsert().Model(olderSupplement).Exec(ctx)
	require.NoError(t, err)

	// Create newer supplement (m4b) - should NOT be promoted
	newerSupplementPath := filepath.Join(tmpDir, "newer.m4b")
	err = os.WriteFile(newerSupplementPath, []byte("newer supplement"), 0644)
	require.NoError(t, err)

	newerSupplement := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleSupplement,
		Filepath:      newerSupplementPath,
		FilesizeBytes: 16,
		CreatedAt:     time.Now().Add(-1 * time.Hour), // Created 1 hour ago
	}
	_, err = db.NewInsert().Model(newerSupplement).Exec(ctx)
	require.NoError(t, err)

	// Define supported types (native types)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
	}

	// Delete the main file
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes, nil)
	require.NoError(t, err)

	// Book should NOT be deleted
	assert.False(t, result.BookDeleted)

	// Verify older supplement was promoted to main
	var updatedOlder models.File
	err = db.NewSelect().Model(&updatedOlder).Where("id = ?", olderSupplement.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.FileRoleMain, updatedOlder.FileRole, "older supplement should be promoted")

	// Verify newer supplement stays as supplement
	var updatedNewer models.File
	err = db.NewSelect().Model(&updatedNewer).Where("id = ?", newerSupplement.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.FileRoleSupplement, updatedNewer.FileRole, "newer supplement should remain as supplement")
}
