package series

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

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

	assert.Equal(t, "New Target Name Old Source Name", readBooksFTSSeriesNames(ctx, t, db, book.ID),
		"books_fts.series_names must include the target name and the source name (now an alias)")
}

// TestDeleteSeries_HardDeletesRowAndFTS pins the post-soft-delete behavior:
// after DeleteSeries the row is gone from `series` (not just flagged with
// deleted_at), the join rows are gone, and DeleteFromSeriesIndex purges the
// FTS row. Catches accidental reintroduction of the soft_delete tag.
func TestDeleteSeries_HardDeletesRowAndFTS(t *testing.T) {
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

	// Index the series in FTS so we can assert it's purged after deletion.
	searchSvc := search.NewService(db)
	require.NoError(t, searchSvc.IndexSeries(ctx, s))

	// Sanity: FTS row exists before deletion. series_fts is a virtual FTS5
	// table, so use a raw query (Bun model queries don't work against it).
	var pre int
	require.NoError(t, db.NewRaw("SELECT count(*) FROM series_fts WHERE series_id = ?", s.ID).
		Scan(ctx, &pre))
	require.Equal(t, 1, pre, "series_fts should have a row before deletion")

	// Delete via the service. The service does NOT touch FTS — the handler
	// does. We invoke DeleteFromSeriesIndex explicitly to mirror the handler.
	svc := NewService(db)
	_, err = svc.DeleteSeries(ctx, s.ID)
	require.NoError(t, err)
	require.NoError(t, searchSvc.DeleteFromSeriesIndex(ctx, s.ID))

	// 1. The row is gone from `series` (not just flagged). This is the
	//    assertion that fails on the pre-change tree.
	count, err := db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", s.ID).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "series row must be hard-deleted, not soft-deleted")

	// 2. book_series rows for the deleted series are gone (CASCADE).
	bsCount, err := db.NewSelect().Model((*models.BookSeries)(nil)).
		Where("series_id = ?", s.ID).
		Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, bsCount, "book_series rows must be removed")

	// 3. FTS row is gone.
	var post int
	require.NoError(t, db.NewRaw("SELECT count(*) FROM series_fts WHERE series_id = ?", s.ID).
		Scan(ctx, &post))
	assert.Equal(t, 0, post, "series_fts row must be removed after DeleteFromSeriesIndex")
}

// TestCleanupOrphanedSeries_ReturnsDeletedIDs pins the requirement that
// CleanupOrphanedSeries returns the IDs of deleted series so callers can keep
// series_fts in sync. Without this, orphan-series cleanup silently leaves
// stale FTS rows that surface in search results pointing at non-existent
// series.
func TestCleanupOrphanedSeries_ReturnsDeletedIDs(t *testing.T) {
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

	// Series with a book — must NOT be cleaned up.
	keep := &models.Series{
		LibraryID:      library.ID,
		Name:           "Kept Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Kept Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(keep).Exec(ctx)
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

	bs := &models.BookSeries{BookID: book.ID, SeriesID: keep.ID, SortOrder: 1}
	_, err = db.NewInsert().Model(bs).Exec(ctx)
	require.NoError(t, err)

	// Orphan series with no book — must be cleaned up.
	orphan := &models.Series{
		LibraryID:      library.ID,
		Name:           "Orphan Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Orphan Series",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(orphan).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db)
	deletedIDs, err := svc.CleanupOrphanedSeries(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{orphan.ID}, deletedIDs,
		"CleanupOrphanedSeries must return the IDs of deleted series so callers can purge FTS")

	// Orphan row is gone.
	count, err := db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", orphan.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "orphan series row should be deleted")

	// Kept series remains.
	count, err = db.NewSelect().Model((*models.Series)(nil)).
		Where("id = ?", keep.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "series with books must not be cleaned up")
}

func TestFindOrCreateSeries_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:  library.ID,
		Name:       "Harry Potter",
		NameSource: models.DataSourceFilepath,
		SortName:   "Harry Potter",
	}
	err = svc.CreateSeries(ctx, s)
	require.NoError(t, err)

	found, err := svc.FindOrCreateSeries(ctx, "Harry Potter", library.ID, models.DataSourceFilepath)
	require.NoError(t, err)
	assert.Equal(t, s.ID, found.ID)
}

func TestFindOrCreateSeries_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	s := &models.Series{
		LibraryID:  library.ID,
		Name:       "A Song of Ice and Fire",
		NameSource: models.DataSourceFilepath,
		SortName:   "Song of Ice and Fire, A",
	}
	err = svc.CreateSeries(ctx, s)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO series_aliases (created_at, series_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), s.ID, "Game of Thrones", library.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreateSeries(ctx, "Game of Thrones", library.ID, models.DataSourceFilepath)
	require.NoError(t, err)
	assert.Equal(t, s.ID, found.ID)
	assert.Equal(t, "A Song of Ice and Fire", found.Name)
}

func TestFindOrCreateSeries_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreateSeries(ctx, "The Wheel of Time", library.ID, models.DataSourceFilepath)
	require.NoError(t, err)
	assert.Equal(t, "The Wheel of Time", found.Name)
	assert.Equal(t, library.ID, found.LibraryID)
}
