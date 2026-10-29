package books

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestRetrieveBook_LoadsChaptersForEachFile(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
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
	bookSvc := NewService(db, appsettings.NewService(db))
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
	db := testdb.New(t)
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
	bookSvc := NewService(db, appsettings.NewService(db))
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
	db := testdb.New(t)
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
	bookSvc := NewService(db, appsettings.NewService(db))
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
	db := testdb.New(t)
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
	bookSvc := NewService(db, appsettings.NewService(db))
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

func TestBookLoaders_OrderGenresAndTagsByName(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	_, book := setupTestLibraryAndBook(t, db)
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/book.epub",
		FilesizeBytes: 1000,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Link in neither alphabetical nor case-sensitive order, so only a
	// case-insensitive name sort produces the expected result.
	names := []string{"Zombies", "action", "Mystery"}
	want := []string{"action", "Mystery", "Zombies"}
	for _, name := range names {
		genre := &models.Genre{LibraryID: book.LibraryID, Name: name}
		_, err := db.NewInsert().Model(genre).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genre.ID}).Exec(ctx)
		require.NoError(t, err)

		tag := &models.Tag{LibraryID: book.LibraryID, Name: name}
		_, err = db.NewInsert().Model(tag).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewInsert().Model(&models.BookTag{BookID: book.ID, TagID: tag.ID}).Exec(ctx)
		require.NoError(t, err)
	}

	assertOrdered := func(t *testing.T, b *models.Book) {
		t.Helper()
		var genres, tags []string
		for _, bg := range b.BookGenres {
			genres = append(genres, bg.Genre.Name)
		}
		for _, bt := range b.BookTags {
			tags = append(tags, bt.Tag.Name)
		}
		assert.Equal(t, want, genres)
		assert.Equal(t, want, tags)
	}

	svc := NewService(db, appsettings.NewService(db))

	retrieved, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assertOrdered(t, retrieved)

	byPath, err := svc.RetrieveBookByFilePath(ctx, file.Filepath, book.LibraryID)
	require.NoError(t, err)
	assertOrdered(t, byPath)

	listed, err := svc.ListBooks(ctx, ListBooksOptions{LibraryID: &book.LibraryID})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assertOrdered(t, listed[0])
}

// ptrInt64 is a helper to create a pointer to an int64.
func ptrInt64(v int64) *int64 {
	return &v
}

func TestDeleteBookAndFiles_DeletesBookFilesAndDiskFiles(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library with OrganizeFileStructure=false (files at root level)
	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: false,
		CoverAspectRatio:      "book",
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

	// CoverImageFilename stores just the filename; the full path is
	// resolved at runtime relative to the book directory.
	coverFilename := "test.cover.jpg"
	coverPath := filepath.Join(tmpDir, coverFilename)
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
		CoverImageFilename: &coverFilename,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesDeleted)

	// Verify book is deleted from DB
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify file is deleted from DB
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify files are deleted from disk
	_, err = os.Stat(mainFilePath)
	assert.True(t, os.IsNotExist(err), "main file should be deleted")
	_, err = os.Stat(coverPath)
	assert.True(t, os.IsNotExist(err), "cover should be deleted")
	_, err = os.Stat(sidecarPath)
	assert.True(t, os.IsNotExist(err), "sidecar should be deleted")
}

// TestDeleteBookAndFiles_RemovesCoverWhenMainFileAlreadyMissing is a
// regression test for a bug where deleteFileFromDisk resolved the cover
// path by stat'ing file.Filepath. If the main file had already been
// removed from disk (e.g. the user manually deleted it before triggering
// the app's delete), the stat would fail and the resulting cover path
// would be a junk location — silently leaving the cover file orphaned.
// The current implementation uses a pure-string join against
// filepath.Dir(file.Filepath) which is stat-race-free.
func TestDeleteBookAndFiles_RemovesCoverWhenMainFileAlreadyMissing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: false,
		CoverAspectRatio:      "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

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

	// Write the cover but NOT the main file, to simulate a user who has
	// already manually deleted the main file before running the app's
	// delete.
	mainFilePath := filepath.Join(tmpDir, "test.epub")
	coverFilename := "test.cover.jpg"
	coverPath := filepath.Join(tmpDir, coverFilename)
	require.NoError(t, os.WriteFile(coverPath, []byte("cover content"), 0644))

	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           mainFilePath,
		FilesizeBytes:      12,
		CoverImageFilename: &coverFilename,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles — main file absence is expected and fine.
	bookSvc := NewService(db, appsettings.NewService(db))
	_, err = bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	// The orphaned cover file must be cleaned up even though the main
	// file was already missing when the delete ran.
	_, err = os.Stat(coverPath)
	assert.True(t, os.IsNotExist(err), "cover should be deleted even when main file was already missing")
}

func TestDeleteBookAndFiles_OrganizedStructure_DeletesEntireDirectory(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for library
	tmpDir := t.TempDir()

	// Create library with OrganizeFileStructure=true (files in book directories)
	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: true,
		CoverAspectRatio:      "book",
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

	// CoverImageFilename stores just the filename; the full path is
	// resolved at runtime relative to the book directory.
	coverFilename := "test.cover.jpg"
	coverPath := filepath.Join(bookDir, coverFilename)
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
		CoverImageFilename: &coverFilename,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesDeleted)

	// Verify book is deleted from DB
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify file is deleted from DB
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify the entire book directory is deleted
	_, err = os.Stat(bookDir)
	assert.True(t, os.IsNotExist(err), "book directory should be deleted")
}

// insertOrganizedLibraryBook inserts an organized library rooted at
// libraryDir and a book at bookPath holding one main EPUB at filePath, with a
// cover and a file sidecar beside it on disk.
func insertOrganizedLibraryBook(t *testing.T, db *bun.DB, libraryDir, bookPath, filePath string) (*models.Library, *models.Book) {
	t.Helper()
	ctx := context.Background()

	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: true,
		CoverAspectRatio:      "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)
	lp := &models.LibraryPath{LibraryID: library.ID, Filepath: libraryDir}
	_, err = db.NewInsert().Model(lp).Exec(ctx)
	require.NoError(t, err)
	library.LibraryPaths = []*models.LibraryPath{lp}

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookPath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filePath, []byte("test content"), 0644))
	coverFilename := filepath.Base(filePath) + ".cover.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(filePath), coverFilename), []byte("cover"), 0644))
	require.NoError(t, os.WriteFile(filePath+".metadata.json", []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(bookSidecarFor(filePath), []byte("{}"), 0644))
	_, err = db.NewInsert().Model(&models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           filePath,
		FilesizeBytes:      12,
		CoverImageFilename: &coverFilename,
	}).Exec(ctx)
	require.NoError(t, err)

	return library, book
}

// bookSidecarFor is the book sidecar beside a single-file book's file.
func bookSidecarFor(filePath string) string {
	return strings.TrimSuffix(filePath, filepath.Ext(filePath)) + ".metadata.json"
}

// TestDeleteBookAndFiles_OrganizedStructure_BookAtLibraryRoot covers a book
// recorded at the library root itself. Deleting it removes only its own
// files, never the library root and everything in it.
func TestDeleteBookAndFiles_OrganizedStructure_BookAtLibraryRoot(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	otherFile := filepath.Join(libraryDir, "Other Book.epub")
	require.NoError(t, os.WriteFile(otherFile, []byte("other"), 0644))
	require.NoError(t, os.WriteFile(bookSidecarFor(otherFile), []byte("{}"), 0644))
	filePath := filepath.Join(libraryDir, "Test Book.epub")
	library, book := insertOrganizedLibraryBook(t, db, libraryDir, libraryDir, filePath)
	// A book keyed by a directory writes its sidecar inside it, named after it.
	rootSidecar := filepath.Join(libraryDir, filepath.Base(libraryDir)+".metadata.json")
	require.NoError(t, os.WriteFile(rootSidecar, []byte("{}"), 0644))

	bookSvc := NewService(db, appsettings.NewService(db))
	_, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	_, err = os.Stat(otherFile)
	require.NoError(t, err, "files of other books at the library root should survive")
	require.FileExists(t, bookSidecarFor(otherFile), "other books' sidecars should survive")
	for _, p := range []string{filePath, filePath + ".cover.jpg", filePath + ".metadata.json", rootSidecar} {
		_, err = os.Stat(p)
		assert.True(t, os.IsNotExist(err), "%s should be deleted", p)
	}
}

// TestDeleteBookAndFiles_OrganizedStructure_BookAtFilepath covers a
// single-file book keyed by its file: the file's cover and sidecar go with it.
func TestDeleteBookAndFiles_OrganizedStructure_BookAtFilepath(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	libraryDir := t.TempDir()
	bookDir := filepath.Join(libraryDir, "Shared")
	require.NoError(t, os.MkdirAll(bookDir, 0755))
	otherFile := filepath.Join(bookDir, "Other Book.epub")
	require.NoError(t, os.WriteFile(otherFile, []byte("other"), 0644))
	require.NoError(t, os.WriteFile(bookSidecarFor(otherFile), []byte("{}"), 0644))
	filePath := filepath.Join(bookDir, "Test Book.epub")
	library, book := insertOrganizedLibraryBook(t, db, libraryDir, filePath, filePath)

	bookSvc := NewService(db, appsettings.NewService(db))
	_, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	_, err = os.Stat(otherFile)
	require.NoError(t, err, "files of other books in the folder should survive")
	require.FileExists(t, bookSidecarFor(otherFile), "other books' sidecars should survive")
	for _, p := range []string{filePath, filePath + ".cover.jpg", filePath + ".metadata.json", bookSidecarFor(filePath)} {
		_, err = os.Stat(p)
		assert.True(t, os.IsNotExist(err), "%s should be deleted", p)
	}
}

func TestDeleteFileAndCleanup_DeletesFileAndKeepsBook(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file1.ID, library, nil)
	require.NoError(t, err)

	assert.False(t, result.BookDeleted, "book should not be deleted")

	// Verify file1 is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file1.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify file1 is deleted from disk
	_, err = os.Stat(file1Path)
	assert.True(t, os.IsNotExist(err))

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify file2 still exists
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file2.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestDeleteFileAndCleanup_DeletesBookWhenLastFile(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library, nil)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify book is deleted from DB
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestDeleteFileAndCleanup_CleansUpDirectoryWithIgnoredFiles(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for the book
	bookDir := t.TempDir()

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                  "Test Library",
		CoverAspectRatio:      "book",
		OrganizeFileStructure: true,
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

	// Delete the only file; .DS_Store is on the fixed junk list
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library, nil)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify book directory is completely removed (including .DS_Store)
	_, err = os.Stat(bookDir)
	assert.True(t, os.IsNotExist(err), "book directory should be completely removed, including ignored files")
}

func TestDeleteFileAndCleanup_CleansUpCoverAndSidecarFiles(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for the book
	bookDir := t.TempDir()

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                  "Test Library",
		CoverAspectRatio:      "book",
		OrganizeFileStructure: true,
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

	// Create shisho-generated cover and sidecar files
	coverPath := filepath.Join(bookDir, "test.epub.cover.jpg")
	err = os.WriteFile(coverPath, []byte("fake cover"), 0644)
	require.NoError(t, err)

	sidecarPath := filepath.Join(bookDir, "test.epub.metadata.json")
	err = os.WriteFile(sidecarPath, []byte("{}"), 0644)
	require.NoError(t, err)

	bookSidecarPath := filepath.Join(bookDir, "Test Book.metadata.json")
	err = os.WriteFile(bookSidecarPath, []byte("{}"), 0644)
	require.NoError(t, err)

	// Verify files exist before deletion
	_, err = os.Stat(coverPath)
	require.NoError(t, err, "cover file should exist before deletion")
	_, err = os.Stat(sidecarPath)
	require.NoError(t, err, "sidecar file should exist before deletion")
	_, err = os.Stat(bookSidecarPath)
	require.NoError(t, err, "book sidecar file should exist before deletion")

	// Delete the only file
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library, nil)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify book directory is completely removed (including cover and sidecar files)
	_, err = os.Stat(bookDir)
	assert.True(t, os.IsNotExist(err), "book directory should be completely removed, including cover and sidecar files")
}

func TestDeleteFileAndCleanup_FileNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create library (needed as parameter)
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Try to delete a non-existent file
	bookSvc := NewService(db, appsettings.NewService(db))
	_, err = bookSvc.DeleteFileAndCleanup(ctx, 99999, library, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDeleteBooksAndFiles_DeletesMultipleBooks(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	// Create temp directory for files
	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteBooksAndFiles(ctx, []int{book1.ID, book2.ID}, library)
	require.NoError(t, err)

	assert.Equal(t, 2, result.BooksDeleted)
	assert.Equal(t, 3, result.FilesDeleted)

	// Verify all books deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.List([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

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
	db := testdb.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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

	supplementModTime := time.Now()
	supplementFile := &models.File{
		LibraryID:      library.ID,
		BookID:         book.ID,
		FileType:       models.FileTypeCBZ,
		FileRole:       models.FileRoleSupplement,
		Filepath:       supplementFilePath,
		FilesizeBytes:  18,
		FileModifiedAt: &supplementModTime,
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	// Define supported types (native types)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
		models.FileTypePDF:  {},
	}

	// Delete the main file
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes)
	require.NoError(t, err)

	// Book should NOT be deleted (supplement was promoted)
	assert.False(t, result.BookDeleted, "book should not be deleted when supplement can be promoted")

	// Verify main file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", mainFile.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify supplement was promoted to main
	var updatedSupplement models.File
	err = db.NewSelect().Model(&updatedSupplement).Where("id = ?", supplementFile.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.FileRoleMain, updatedSupplement.FileRole)
	assert.Nil(t, updatedSupplement.FileModifiedAt, "the next scan reads the promoted File in full")
}

func TestDeleteFileAndCleanup_DeletesBookWhenOnlyUnsupportedSupplementsRemain(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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

	// Create supplement file (txt - NOT a supported type)
	supplementFilePath := filepath.Join(tmpDir, "supplement.txt")
	err = os.WriteFile(supplementFilePath, []byte("supplement content"), 0644)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      "txt",
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
		models.FileTypePDF:  {},
	}

	// Delete the main file
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes)
	require.NoError(t, err)

	// Book SHOULD be deleted (no promotable supplements)
	assert.True(t, result.BookDeleted, "book should be deleted when no supplement can be promoted")

	// Verify both files are deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("book_id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify book is deleted
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify supplement file is deleted from disk
	_, err = os.Stat(supplementFilePath)
	assert.True(t, os.IsNotExist(err))
}

func TestDeleteFileAndCleanup_PromotesPDFSupplementWhenLastMainFileDeleted(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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

	// Create supplement file (pdf - a supported type for promotion)
	supplementFilePath := filepath.Join(tmpDir, "supplement.pdf")
	err = os.WriteFile(supplementFilePath, []byte("supplement content"), 0644)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		Filepath:      supplementFilePath,
		FilesizeBytes: 18,
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	// Define supported types (native types including PDF)
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
		models.FileTypePDF:  {},
	}

	// Delete the main file
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes)
	require.NoError(t, err)

	// Book should NOT be deleted (PDF supplement was promoted)
	assert.False(t, result.BookDeleted, "book should not be deleted when PDF supplement can be promoted")

	// Verify main file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", mainFile.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify PDF supplement was promoted to main
	var updatedSupplement models.File
	err = db.NewSelect().Model(&updatedSupplement).Where("id = ?", supplementFile.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.FileRoleMain, updatedSupplement.FileRole)
}

func TestDeleteFileAndCleanup_PromotesOldestSupplementFirst(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	// Create library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
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
		models.FileTypePDF:  {},
	}

	// Delete the main file
	bookSvc := NewService(db, appsettings.NewService(db))
	result, err := bookSvc.DeleteFileAndCleanup(ctx, mainFile.ID, library, supportedTypes)
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

func TestListAllFilesForLibrary_IncludesSupplements(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))

	library, book := setupTestLibraryAndBook(t, db)

	mainFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/main.epub",
		FilesizeBytes: 100,
	}
	_, err := db.NewInsert().Model(mainFile).Exec(ctx)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		Filepath:      "/test/supplement.pdf",
		FilesizeBytes: 200,
	}
	_, err = db.NewInsert().Model(supplementFile).Exec(ctx)
	require.NoError(t, err)

	// ListFilesForLibrary keeps returning main-only (unchanged invariant for orphan cleanup)
	mainOnly, err := svc.ListFilesForLibrary(ctx, library.ID)
	require.NoError(t, err)
	require.Len(t, mainOnly, 1)
	assert.Equal(t, mainFile.ID, mainOnly[0].ID)

	// New method returns both main and supplement files for the library
	all, err := svc.ListAllFilesForLibrary(ctx, library.ID)
	require.NoError(t, err)
	require.Len(t, all, 2)
	roles := map[string]struct{}{}
	for _, f := range all {
		roles[f.FileRole] = struct{}{}
	}
	assert.Contains(t, roles, models.FileRoleMain)
	assert.Contains(t, roles, models.FileRoleSupplement)
}
