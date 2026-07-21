package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func seedSeriesBooks(t *testing.T, db *bun.DB, library *models.Library, seriesID int, entries []struct {
	Title        string
	SeriesNumber *float64
}) []*models.Book {
	t.Helper()
	ctx := context.Background()

	var books []*models.Book
	for i, e := range entries {
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           e.Title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       e.Title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        t.TempDir(),
		}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		bs := &models.BookSeries{
			BookID:       book.ID,
			SeriesID:     seriesID,
			SeriesNumber: e.SeriesNumber,
			SortOrder:    i + 1,
		}
		_, err = db.NewInsert().Model(bs).Exec(ctx)
		require.NoError(t, err)

		file := &models.File{
			LibraryID:     library.ID,
			BookID:        book.ID,
			FileType:      models.FileTypeEPUB,
			FileRole:      models.FileRoleMain,
			Filepath:      "/fake/" + e.Title + ".epub",
			FilesizeBytes: 100,
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)

		books = append(books, book)
	}
	return books
}

func ptrFloat64(v float64) *float64 { return &v }

func TestGetFirstBookInSeriesByID_PrefersWholeNumberOverPrequel(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db) // creates library + a throwaway book

	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Test Series",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Prequel", SeriesNumber: ptrFloat64(0.5)},
		{Title: "Book One", SeriesNumber: ptrFloat64(1)},
		{Title: "Book Two", SeriesNumber: ptrFloat64(2)},
	})

	svc := NewService(db)
	first, err := svc.GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[1].ID, first.ID, "should pick Book One (1) over Prequel (0.5)")
}

func TestGetFirstBookInSeriesByID_PrefersSingleNumberOverOmnibus(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)
	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Omnibus Series",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Omnibus Series",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Omnibus One to Three", SeriesNumber: ptrFloat64(1)},
		{Title: "Book Two", SeriesNumber: ptrFloat64(2)},
	})
	_, err = db.NewUpdate().Table("book_series").Set("series_number_end = 3").Where("book_id = ?", books[0].ID).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db)
	first, err := svc.GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[1].ID, first.ID)
}

func TestGetFirstBooksFilesForSeries_PrefersSingleNumberOverOmnibus(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)
	series := &models.Series{
		LibraryID: library.ID, Name: "Bulk Omnibus Series", NameSource: models.DataSourceFilepath,
		SortName: "Bulk Omnibus Series", SortNameSource: models.DataSourceFilepath,
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Omnibus One to Three", SeriesNumber: ptrFloat64(1)},
		{Title: "Book Two", SeriesNumber: ptrFloat64(2)},
	})
	_, err = db.NewUpdate().Table("book_series").Set("series_number_end = 3").Where("book_id = ?", books[0].ID).Exec(ctx)
	require.NoError(t, err)

	filesBySeries, err := NewService(db).GetFirstBooksFilesForSeries(ctx, []int{series.ID})
	require.NoError(t, err)
	require.Len(t, filesBySeries[series.ID], 1)
	assert.Equal(t, "/fake/Book Two.epub", filesBySeries[series.ID][0].Filepath)
}

func TestGetFirstBookInSeriesByID_UsesRangeEndpointTieBreaker(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)
	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Omnibus Only",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Omnibus Only",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "A Longer Omnibus", SeriesNumber: ptrFloat64(1)},
		{Title: "Z Shorter Omnibus", SeriesNumber: ptrFloat64(1)},
	})
	_, err = db.NewUpdate().Table("book_series").Set("series_number_end = 4").Where("book_id = ?", books[0].ID).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewUpdate().Table("book_series").Set("series_number_end = 3").Where("book_id = ?", books[1].ID).Exec(ctx)
	require.NoError(t, err)

	first, err := NewService(db).GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[1].ID, first.ID)
}

func TestGetFirstBookInSeriesByID_FallsBackToFractionalWhenNoWholeNumbers(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)

	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Novellas",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Novellas",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Novella 0.5", SeriesNumber: ptrFloat64(0.5)},
		{Title: "Novella 1.5", SeriesNumber: ptrFloat64(1.5)},
	})

	svc := NewService(db)
	first, err := svc.GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[0].ID, first.ID, "should pick the earliest fractional (0.5) when no whole numbers exist")
}

func TestGetFirstBookInSeriesByID_PicksLowestWholeNumber(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)

	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Normal Series",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Normal Series",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Book One", SeriesNumber: ptrFloat64(1)},
		{Title: "Book Two", SeriesNumber: ptrFloat64(2)},
		{Title: "Book Three", SeriesNumber: ptrFloat64(3)},
	})

	svc := NewService(db)
	first, err := svc.GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[0].ID, first.ID, "should pick the first whole number (1)")
}

func TestGetFirstBookInSeriesByID_NullSeriesNumberTreatedAsFractional(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library, _ := setupTestLibraryAndBook(t, db)

	series := &models.Series{
		LibraryID:      library.ID,
		Name:           "Mixed",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Mixed",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err := db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	books := seedSeriesBooks(t, db, library, series.ID, []struct {
		Title        string
		SeriesNumber *float64
	}{
		{Title: "Unnumbered", SeriesNumber: nil},
		{Title: "Book One", SeriesNumber: ptrFloat64(1)},
	})

	svc := NewService(db)
	first, err := svc.GetFirstBookInSeriesByID(ctx, series.ID)
	require.NoError(t, err)
	assert.Equal(t, books[1].ID, first.ID, "should prefer Book One (whole) over unnumbered (nil)")
}
