package series

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// indexBookFTS retrieves a book with the relations IndexBook reads and writes
// the books_fts row for it. Used in tests to seed the FTS state for a book
// before exercising mutations that should refresh the row.
func indexBookFTS(ctx context.Context, t *testing.T, bookSvc *books.Service, searchSvc *search.Service, bookID int) {
	t.Helper()
	b, err := bookSvc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	require.NoError(t, searchSvc.IndexBook(ctx, b))
}

// readBooksFTSSeriesNames returns the books_fts.series_names column for a book.
// books_fts is a virtual FTS5 table, so we use a raw query.
func readBooksFTSSeriesNames(ctx context.Context, t *testing.T, db *bun.DB, bookID int) string {
	t.Helper()
	var seriesNames string
	err := db.NewRaw("SELECT series_names FROM books_fts WHERE book_id = ?", bookID).
		Scan(ctx, &seriesNames)
	require.NoError(t, err)
	return seriesNames
}

// TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed is a regression test for
// the bug where soft-deleting a series left orphan book_series rows behind,
// so the Reviewed-flag completeness check (len(book.BookSeries) > 0) would
// still consider the book to have a series even though no UI surfaces it.
//
// After deletion the join rows must be gone and a reviewed-recompute must
// flip the file back to reviewed=false when `series` is a required field.
func TestDeleteSeries_RemovesBookSeriesAndFlipsReviewed(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Test Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: s.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	// Configure review criteria so that `series` is the only thing keeping the
	// file reviewed=true. Then recompute and confirm reviewed=true.
	settings := appsettings.NewService(db)
	require.NoError(t, review.Save(ctx, settings, review.Criteria{
		BookFields:  []string{review.FieldSeries},
		AudioFields: nil,
	}))

	bookSvc := books.NewService(db).WithAppSettings(settings)
	bookSvc.RecomputeReviewedForBook(ctx, book.ID)

	var pre models.File
	require.NoError(t, db.NewSelect().Model(&pre).Where("f.id = ?", file.ID).Scan(ctx))
	require.NotNil(t, pre.Reviewed)
	require.True(t, *pre.Reviewed, "file should be reviewed when its only required field (series) is satisfied")

	// Delete the series. DeleteSeries should hard-delete the join rows and
	// return the affected book IDs so the caller can recompute.
	svc := NewService(db)
	affected, err := svc.DeleteSeries(ctx, s.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{book.ID}, affected)

	// book_series rows for this series must be gone.
	count, err := db.NewSelect().Model((*models.BookSeries)(nil)).Where("series_id = ?", s.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "book_series rows pointing at the deleted series must be removed")

	// Recompute (mirrors what the handler will do for each affected book).
	bookSvc.RecomputeReviewedForBook(ctx, book.ID)

	var post models.File
	require.NoError(t, db.NewSelect().Model(&post).Where("f.id = ?", file.ID).Scan(ctx))
	require.NotNil(t, post.Reviewed)
	assert.False(t, *post.Reviewed, "file should flip back to unreviewed once the series is gone")
}

// TestDeleteSeriesHandler_ReindexesAffectedBooks asserts that going through
// the deleteSeries HTTP handler refreshes the books_fts row for any book that
// was associated with the deleted series, so search results no longer surface
// the gone-series name as a token on those books.
func TestDeleteSeriesHandler_ReindexesAffectedBooks(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:      library.ID,
		Name:           "Distinctive Series Name",
		NameSource:     models.DataSourceManual,
		SortName:       "Distinctive Series Name",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(s).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: s.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	bookSvc := books.NewService(db).WithAppSettings(appsettings.NewService(db))
	searchSvc := search.NewService(db)

	// Seed the FTS state: index the book while the series association is alive.
	indexBookFTS(ctx, t, bookSvc, searchSvc, book.ID)
	require.Equal(t, "Distinctive Series Name", readBooksFTSSeriesNames(ctx, t, db, book.ID),
		"FTS row should contain the series name before deletion")

	// Build the handler and invoke deleteSeries. No user is set on the context,
	// so the library access check is skipped.
	h := &handler{
		seriesService:  NewService(db),
		bookService:    bookSvc,
		libraryService: libraries.NewService(db),
		searchService:  searchSvc,
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(s.ID))
	require.NoError(t, h.deleteSeries(c))
	require.Equal(t, http.StatusNoContent, rec.Code)

	assert.Empty(t, readBooksFTSSeriesNames(ctx, t, db, book.ID),
		"books_fts.series_names must not retain the deleted series name after deleteSeries")
}

// TestMergeSeriesHandler_ReindexesAffectedBooks asserts that merging a source
// series into a target updates the books_fts row of every book that moved, so
// the source series name is no longer a search token on those books and the
// target series name is.
func TestMergeSeriesHandler_ReindexesAffectedBooks(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceManual,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/tmp/test.epub",
		FilesizeBytes: 1,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	source := &models.Series{
		LibraryID:      library.ID,
		Name:           "Old Source Name",
		NameSource:     models.DataSourceManual,
		SortName:       "Old Source Name",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(source).Exec(ctx)
	require.NoError(t, err)

	target := &models.Series{
		LibraryID:      library.ID,
		Name:           "New Target Name",
		NameSource:     models.DataSourceManual,
		SortName:       "New Target Name",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(target).Exec(ctx)
	require.NoError(t, err)

	bs := &models.BookSeries{BookID: book.ID, SeriesID: source.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	bookSvc := books.NewService(db).WithAppSettings(appsettings.NewService(db))
	searchSvc := search.NewService(db)

	// Seed FTS while the book is on the source series.
	indexBookFTS(ctx, t, bookSvc, searchSvc, book.ID)
	require.Equal(t, "Old Source Name", readBooksFTSSeriesNames(ctx, t, db, book.ID))

	// Merge source into target via the HTTP handler.
	h := &handler{
		seriesService:  NewService(db),
		bookService:    bookSvc,
		libraryService: libraries.NewService(db),
		searchService:  searchSvc,
	}
	e := echo.New()
	body := `{"source_id":` + strconv.Itoa(source.ID) + `}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(target.ID))
	require.NoError(t, h.merge(c))
	require.Equal(t, http.StatusNoContent, rec.Code)

	assert.Equal(t, "New Target Name", readBooksFTSSeriesNames(ctx, t, db, book.ID),
		"books_fts.series_names must reflect the target series name after a merge")
}
