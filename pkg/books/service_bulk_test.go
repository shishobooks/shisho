package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_BulkCreateAuthors(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library, book, and persons
	library, book := setupTestLibraryAndBook(t, db)

	now := time.Now()
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
	_, err := db.NewInsert().Model(person1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(person2).Exec(ctx)
	require.NoError(t, err)

	// Test bulk create authors
	authors := []*models.Author{
		{BookID: book.ID, PersonID: person1.ID, SortOrder: 1},
		{BookID: book.ID, PersonID: person2.ID, SortOrder: 2},
	}

	err = svc.BulkCreateAuthors(ctx, authors)
	require.NoError(t, err)

	// Verify authors were created
	var createdAuthors []*models.Author
	err = db.NewSelect().Model(&createdAuthors).Where("book_id = ?", book.ID).Order("sort_order ASC").Scan(ctx)
	require.NoError(t, err)
	require.Len(t, createdAuthors, 2)
	assert.Equal(t, person1.ID, createdAuthors[0].PersonID)
	assert.Equal(t, person2.ID, createdAuthors[1].PersonID)
}

func TestService_BulkCreateNarrators(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library, book, file, and persons
	library, book := setupTestLibraryAndBook(t, db)

	now := time.Now()
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/audiobook.m4b",
		FilesizeBytes: 1000,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err := db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	person1 := &models.Person{
		LibraryID:      library.ID,
		Name:           "Narrator One",
		SortName:       "One, Narrator",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	person2 := &models.Person{
		LibraryID:      library.ID,
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

	// Test bulk create narrators
	narrators := []*models.Narrator{
		{FileID: file.ID, PersonID: person1.ID, SortOrder: 1},
		{FileID: file.ID, PersonID: person2.ID, SortOrder: 2},
	}

	err = svc.BulkCreateNarrators(ctx, narrators)
	require.NoError(t, err)

	// Verify narrators were created
	var createdNarrators []*models.Narrator
	err = db.NewSelect().Model(&createdNarrators).Where("file_id = ?", file.ID).Order("sort_order ASC").Scan(ctx)
	require.NoError(t, err)
	require.Len(t, createdNarrators, 2)
	assert.Equal(t, person1.ID, createdNarrators[0].PersonID)
	assert.Equal(t, person2.ID, createdNarrators[1].PersonID)
}

func TestService_BulkCreateBookGenres(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library, book, and genres
	library, book := setupTestLibraryAndBook(t, db)

	now := time.Now()
	genre1 := &models.Genre{
		LibraryID: library.ID,
		Name:      "Fantasy",
		CreatedAt: now,
		UpdatedAt: now,
	}
	genre2 := &models.Genre{
		LibraryID: library.ID,
		Name:      "Science Fiction",
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(genre1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(genre2).Exec(ctx)
	require.NoError(t, err)

	// Test bulk create book genres
	bookGenres := []*models.BookGenre{
		{BookID: book.ID, GenreID: genre1.ID},
		{BookID: book.ID, GenreID: genre2.ID},
	}

	err = svc.BulkCreateBookGenres(ctx, bookGenres)
	require.NoError(t, err)

	// Verify book genres were created
	var createdBookGenres []*models.BookGenre
	err = db.NewSelect().Model(&createdBookGenres).Where("book_id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.Len(t, createdBookGenres, 2)
}

func TestService_BulkCreateBookTags(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library, book, and tags
	library, book := setupTestLibraryAndBook(t, db)

	now := time.Now()
	tag1 := &models.Tag{
		LibraryID: library.ID,
		Name:      "Favorites",
		CreatedAt: now,
		UpdatedAt: now,
	}
	tag2 := &models.Tag{
		LibraryID: library.ID,
		Name:      "To Read",
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(tag1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(tag2).Exec(ctx)
	require.NoError(t, err)

	// Test bulk create book tags
	bookTags := []*models.BookTag{
		{BookID: book.ID, TagID: tag1.ID},
		{BookID: book.ID, TagID: tag2.ID},
	}

	err = svc.BulkCreateBookTags(ctx, bookTags)
	require.NoError(t, err)

	// Verify book tags were created
	var createdBookTags []*models.BookTag
	err = db.NewSelect().Model(&createdBookTags).Where("book_id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.Len(t, createdBookTags, 2)
}

func TestService_BulkCreateBookSeries(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Create library, book, and series
	library, book := setupTestLibraryAndBook(t, db)

	now := time.Now()
	series1 := &models.Series{
		LibraryID:      library.ID,
		Name:           "The Trilogy",
		SortName:       "Trilogy, The",
		NameSource:     models.DataSourceFilepath,
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	series2 := &models.Series{
		LibraryID:      library.ID,
		Name:           "Extended Universe",
		SortName:       "Extended Universe",
		NameSource:     models.DataSourceFilepath,
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := db.NewInsert().Model(series1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(series2).Exec(ctx)
	require.NoError(t, err)

	// Test bulk create book series
	seriesNum1 := 1.0
	seriesNum2 := 2.5
	bookSeries := []*models.BookSeries{
		{BookID: book.ID, SeriesID: series1.ID, SeriesNumber: &seriesNum1, SortOrder: 1},
		{BookID: book.ID, SeriesID: series2.ID, SeriesNumber: &seriesNum2, SortOrder: 2},
	}

	err = svc.BulkCreateBookSeries(ctx, bookSeries)
	require.NoError(t, err)

	// Verify book series were created
	var createdBookSeries []*models.BookSeries
	err = db.NewSelect().Model(&createdBookSeries).Where("book_id = ?", book.ID).Order("sort_order ASC").Scan(ctx)
	require.NoError(t, err)
	require.Len(t, createdBookSeries, 2)
	assert.Equal(t, series1.ID, createdBookSeries[0].SeriesID)
	assert.InDelta(t, 1.0, *createdBookSeries[0].SeriesNumber, 0.001)
	assert.Equal(t, series2.ID, createdBookSeries[1].SeriesID)
	assert.InDelta(t, 2.5, *createdBookSeries[1].SeriesNumber, 0.001)
}

func TestService_BulkCreate_EmptySlice(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Test that empty slices return nil without error
	err := svc.BulkCreateAuthors(ctx, []*models.Author{})
	require.NoError(t, err)

	err = svc.BulkCreateNarrators(ctx, []*models.Narrator{})
	require.NoError(t, err)

	err = svc.BulkCreateBookGenres(ctx, []*models.BookGenre{})
	require.NoError(t, err)

	err = svc.BulkCreateBookTags(ctx, []*models.BookTag{})
	require.NoError(t, err)

	err = svc.BulkCreateBookSeries(ctx, []*models.BookSeries{})
	require.NoError(t, err)
}

func TestService_BulkCreate_NilSlice(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	// Test that nil slices return nil without error
	err := svc.BulkCreateAuthors(ctx, nil)
	require.NoError(t, err)

	err = svc.BulkCreateNarrators(ctx, nil)
	require.NoError(t, err)

	err = svc.BulkCreateBookGenres(ctx, nil)
	require.NoError(t, err)

	err = svc.BulkCreateBookTags(ctx, nil)
	require.NoError(t, err)

	err = svc.BulkCreateBookSeries(ctx, nil)
	require.NoError(t, err)
}
