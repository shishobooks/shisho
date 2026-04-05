package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestDeleteFilesByIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library and book
	_, book := setupTestLibraryAndBook(t, db)

	// Create two files using CreateFile (which also sets the primary file)
	file1 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook1.m4b",
		FilesizeBytes: 1000,
	}
	err := svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook2.m4b",
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Create persons for narrators
	now := time.Now()
	person1 := &models.Person{
		LibraryID:      book.LibraryID,
		Name:           "Narrator One",
		SortName:       "One, Narrator",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	person2 := &models.Person{
		LibraryID:      book.LibraryID,
		Name:           "Narrator Two",
		SortName:       "Two, Narrator",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = db.NewInsert().Model(person1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(person2).Exec(ctx)
	require.NoError(t, err)

	// Add narrators to each file (SortOrder must be > 0 due to nullzero constraint)
	narrator1 := &models.Narrator{FileID: file1.ID, PersonID: person1.ID, SortOrder: 1}
	narrator2 := &models.Narrator{FileID: file2.ID, PersonID: person2.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(narrator1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(narrator2).Exec(ctx)
	require.NoError(t, err)

	// Add identifiers to each file
	identifier1 := &models.FileIdentifier{
		FileID:    file1.ID,
		Type:      models.IdentifierTypeISBN13,
		Value:     "9781234567890",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	identifier2 := &models.FileIdentifier{
		FileID:    file2.ID,
		Type:      models.IdentifierTypeASIN,
		Value:     "B001234567",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(identifier1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(identifier2).Exec(ctx)
	require.NoError(t, err)

	// Add chapters to each file
	chapter1 := &models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	chapter2 := &models.Chapter{FileID: file2.ID, Title: "Ch2", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(chapter2).Exec(ctx)
	require.NoError(t, err)

	// Batch delete both files
	err = svc.DeleteFilesByIDs(ctx, []int{file1.ID, file2.ID})
	require.NoError(t, err)

	// Verify file records are gone
	var fileCount int
	err = db.NewSelect().TableExpr("files").
		Where("id IN (?)", bun.List([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount, "both file records should be deleted")

	// Verify narrators are gone
	var narratorCount int
	err = db.NewSelect().TableExpr("narrators").
		Where("file_id IN (?)", bun.List([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &narratorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, narratorCount, "all narrators should be deleted")

	// Verify identifiers are gone
	var identifierCount int
	err = db.NewSelect().TableExpr("file_identifiers").
		Where("file_id IN (?)", bun.List([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &identifierCount)
	require.NoError(t, err)
	assert.Equal(t, 0, identifierCount, "all identifiers should be deleted")

	// Verify chapters are gone
	var chapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id IN (?)", bun.List([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &chapterCount)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount, "all chapters should be deleted")
}

func TestDeleteFilesByIDs_EmptySlice(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// nil slice should be a no-op
	err := svc.DeleteFilesByIDs(ctx, nil)
	require.NoError(t, err)

	// empty slice should be a no-op
	err = svc.DeleteFilesByIDs(ctx, []int{})
	require.NoError(t, err)
}

func TestDeleteBooksByIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	now := time.Now()

	// Create library and two books
	library, book1 := setupTestLibraryAndBook(t, db)
	book2 := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book 2",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book 2",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	// Create files for both books
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book1.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/book1.m4b",
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book2.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/book2.m4b",
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Create persons for narrators/authors
	person1 := &models.Person{
		LibraryID:      library.ID,
		Name:           "Author One",
		SortName:       "One, Author",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	person2 := &models.Person{
		LibraryID:      library.ID,
		Name:           "Author Two",
		SortName:       "Two, Author",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = db.NewInsert().Model(person1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(person2).Exec(ctx)
	require.NoError(t, err)

	// Add narrators to files
	narrator1 := &models.Narrator{FileID: file1.ID, PersonID: person1.ID, SortOrder: 1}
	narrator2 := &models.Narrator{FileID: file2.ID, PersonID: person2.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(narrator1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(narrator2).Exec(ctx)
	require.NoError(t, err)

	// Add identifiers to files
	identifier1 := &models.FileIdentifier{
		FileID:    file1.ID,
		Type:      models.IdentifierTypeISBN13,
		Value:     "9781234567890",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	identifier2 := &models.FileIdentifier{
		FileID:    file2.ID,
		Type:      models.IdentifierTypeASIN,
		Value:     "B001234567",
		Source:    models.DataSourceM4BMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(identifier1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(identifier2).Exec(ctx)
	require.NoError(t, err)

	// Add chapters to files
	chapter1 := &models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	chapter2 := &models.Chapter{FileID: file2.ID, Title: "Ch2", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(chapter2).Exec(ctx)
	require.NoError(t, err)

	// Add authors for both books
	author1 := &models.Author{BookID: book1.ID, PersonID: person1.ID, SortOrder: 1}
	author2 := &models.Author{BookID: book2.ID, PersonID: person2.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(author1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(author2).Exec(ctx)
	require.NoError(t, err)

	// Add genres for both books
	genre1 := &models.Genre{LibraryID: library.ID, Name: "Fiction", CreatedAt: now, UpdatedAt: now}
	genre2 := &models.Genre{LibraryID: library.ID, Name: "Sci-Fi", CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(genre1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(genre2).Exec(ctx)
	require.NoError(t, err)
	bookGenre1 := &models.BookGenre{BookID: book1.ID, GenreID: genre1.ID}
	bookGenre2 := &models.BookGenre{BookID: book2.ID, GenreID: genre2.ID}
	_, err = db.NewInsert().Model(bookGenre1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(bookGenre2).Exec(ctx)
	require.NoError(t, err)

	// Add tags for both books
	tag1 := &models.Tag{LibraryID: library.ID, Name: "tag1", CreatedAt: now, UpdatedAt: now}
	tag2 := &models.Tag{LibraryID: library.ID, Name: "tag2", CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(tag1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(tag2).Exec(ctx)
	require.NoError(t, err)
	bookTag1 := &models.BookTag{BookID: book1.ID, TagID: tag1.ID}
	bookTag2 := &models.BookTag{BookID: book2.ID, TagID: tag2.ID}
	_, err = db.NewInsert().Model(bookTag1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(bookTag2).Exec(ctx)
	require.NoError(t, err)

	// Add series for both books
	series1 := &models.Series{LibraryID: library.ID, Name: "Series One", NameSource: models.DataSourceFilepath, SortName: "One, Series", SortNameSource: models.DataSourceFilepath, CreatedAt: now, UpdatedAt: now}
	series2 := &models.Series{LibraryID: library.ID, Name: "Series Two", NameSource: models.DataSourceFilepath, SortName: "Two, Series", SortNameSource: models.DataSourceFilepath, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(series1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(series2).Exec(ctx)
	require.NoError(t, err)
	bookSeries1 := &models.BookSeries{BookID: book1.ID, SeriesID: series1.ID, SortOrder: 1}
	bookSeries2 := &models.BookSeries{BookID: book2.ID, SeriesID: series2.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bookSeries1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(bookSeries2).Exec(ctx)
	require.NoError(t, err)

	// Batch delete both books
	err = svc.DeleteBooksByIDs(ctx, []int{book1.ID, book2.ID})
	require.NoError(t, err)

	bookIDs := []int{book1.ID, book2.ID}
	fileIDs := []int{file1.ID, file2.ID}

	// Verify book records are gone
	var bookCount int
	err = db.NewSelect().TableExpr("books").
		Where("id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &bookCount)
	require.NoError(t, err)
	assert.Equal(t, 0, bookCount, "all book records should be deleted")

	// Verify file records are gone
	var fileCount int
	err = db.NewSelect().TableExpr("files").
		Where("book_id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &fileCount)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount, "all file records should be deleted")

	// Verify narrators are gone
	var narratorCount int
	err = db.NewSelect().TableExpr("narrators").
		Where("file_id IN (?)", bun.List(fileIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &narratorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, narratorCount, "all narrators should be deleted")

	// Verify identifiers are gone
	var identifierCount int
	err = db.NewSelect().TableExpr("file_identifiers").
		Where("file_id IN (?)", bun.List(fileIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &identifierCount)
	require.NoError(t, err)
	assert.Equal(t, 0, identifierCount, "all identifiers should be deleted")

	// Verify chapters are gone
	var chapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id IN (?)", bun.List(fileIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &chapterCount)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount, "all chapters should be deleted")

	// Verify authors are gone
	var authorCount int
	err = db.NewSelect().TableExpr("authors").
		Where("book_id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &authorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, authorCount, "all authors should be deleted")

	// Verify book genres are gone
	var bookGenreCount int
	err = db.NewSelect().TableExpr("book_genres").
		Where("book_id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &bookGenreCount)
	require.NoError(t, err)
	assert.Equal(t, 0, bookGenreCount, "all book genres should be deleted")

	// Verify book tags are gone
	var bookTagCount int
	err = db.NewSelect().TableExpr("book_tags").
		Where("book_id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &bookTagCount)
	require.NoError(t, err)
	assert.Equal(t, 0, bookTagCount, "all book tags should be deleted")

	// Verify book series associations are gone
	var bookSeriesCount int
	err = db.NewSelect().TableExpr("book_series").
		Where("book_id IN (?)", bun.List(bookIDs)).
		ColumnExpr("count(*)").
		Scan(ctx, &bookSeriesCount)
	require.NoError(t, err)
	assert.Equal(t, 0, bookSeriesCount, "all book series associations should be deleted")

	// Verify Person records still exist (orphaned entities cleaned up separately)
	var personCount int
	err = db.NewSelect().TableExpr("persons").
		Where("id IN (?)", bun.List([]int{person1.ID, person2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &personCount)
	require.NoError(t, err)
	assert.Equal(t, 2, personCount, "person records should still exist")
}

func TestDeleteBooksByIDs_EmptySlice(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// nil slice should be a no-op
	err := svc.DeleteBooksByIDs(ctx, nil)
	require.NoError(t, err)

	// empty slice should be a no-op
	err = svc.DeleteBooksByIDs(ctx, []int{})
	require.NoError(t, err)
}

func TestDeleteFile_DeletesChapters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	now := time.Now()

	// Create a file
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
	}
	err := svc.CreateFile(ctx, file)
	require.NoError(t, err)

	// Create a second file so the book isn't empty after deletion
	file2 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook2.m4b",
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Add chapters to the file being deleted
	chapter1 := &models.Chapter{FileID: file.ID, Title: "Chapter 1", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	chapter2 := &models.Chapter{FileID: file.ID, Title: "Chapter 2", SortOrder: 1, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(chapter2).Exec(ctx)
	require.NoError(t, err)

	// Add a chapter to file2 (should NOT be deleted)
	chapter3 := &models.Chapter{FileID: file2.ID, Title: "Other Chapter", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter3).Exec(ctx)
	require.NoError(t, err)

	// Delete file1
	err = svc.DeleteFile(ctx, file.ID)
	require.NoError(t, err)

	// Verify chapters for deleted file are gone
	var chapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id = ?", file.ID).
		ColumnExpr("count(*)").
		Scan(ctx, &chapterCount)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount, "chapters for deleted file should be removed")

	// Verify chapter for other file still exists
	var otherChapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id = ?", file2.ID).
		ColumnExpr("count(*)").
		Scan(ctx, &otherChapterCount)
	require.NoError(t, err)
	assert.Equal(t, 1, otherChapterCount, "chapters for other file should still exist")
}

func TestDeleteBook_DeletesChapters(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)
	now := time.Now()

	// Create two files for the book
	file1 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook1.m4b",
		FilesizeBytes: 1000,
	}
	err := svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/book.epub",
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Add chapters to both files
	chapter1 := &models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	chapter2 := &models.Chapter{FileID: file2.ID, Title: "Ch2", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(chapter1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(chapter2).Exec(ctx)
	require.NoError(t, err)

	// Delete the book
	err = svc.DeleteBook(ctx, book.ID)
	require.NoError(t, err)

	// Verify all chapters are gone
	var chapterCount int
	err = db.NewSelect().TableExpr("chapters").
		Where("file_id IN (?)", bun.List([]int{file1.ID, file2.ID})).
		ColumnExpr("count(*)").
		Scan(ctx, &chapterCount)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount, "all chapters should be deleted when book is deleted")
}

func TestPromoteNextPrimaryFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)

	now := time.Now()

	// Create 3 files: supplement (oldest), main (middle), main (newest)
	supplementFile := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleSupplement,
		Filepath:      "/test/supplement.epub",
		FilesizeBytes: 500,
		CreatedAt:     now.Add(-2 * time.Hour),
	}
	err := svc.CreateFile(ctx, supplementFile)
	require.NoError(t, err)

	mainFileOld := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audio_old.m4b",
		FilesizeBytes: 1000,
		CreatedAt:     now.Add(-1 * time.Hour),
	}
	err = svc.CreateFile(ctx, mainFileOld)
	require.NoError(t, err)

	mainFileNew := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audio_new.m4b",
		FilesizeBytes: 2000,
		CreatedAt:     now,
	}
	err = svc.CreateFile(ctx, mainFileNew)
	require.NoError(t, err)

	// Call promote
	err = svc.PromoteNextPrimaryFile(ctx, book.ID)
	require.NoError(t, err)

	// Verify the oldest main file was chosen (not the supplement, not the newer main)
	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedBook.PrimaryFileID, "primary_file_id should be set")
	assert.Equal(t, mainFileOld.ID, *updatedBook.PrimaryFileID, "oldest main file should be chosen")
}

func TestPromoteNextPrimaryFile_NoFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	_, book := setupTestLibraryAndBook(t, db)

	// Manually set a primary_file_id on the book (simulating a stale pointer)
	fakeID := 9999
	book.PrimaryFileID = &fakeID
	_, err := db.NewUpdate().Model(book).Column("primary_file_id").Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)

	// Call promote with no files in the book
	err = svc.PromoteNextPrimaryFile(ctx, book.ID)
	require.NoError(t, err)

	// Verify primary_file_id is now NULL
	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Nil(t, updatedBook.PrimaryFileID, "primary_file_id should be NULL when no files remain")
}
