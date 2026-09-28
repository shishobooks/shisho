package series

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newMergeTestHandler(db *bun.DB) *handler {
	return &handler{
		seriesService:  NewService(db),
		aliasService:   aliases.NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
		searchService:  search.NewService(db),
	}
}

// callSeriesHandler runs fn as user against the Series :id with a JSON body.
func callSeriesHandler(t *testing.T, fn echo.HandlerFunc, user *models.User, id int, body string) error {
	t.Helper()
	e := newTestEchoSeries(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(id))
	if user != nil {
		c.Set("user", user)
	}
	return fn(c)
}

func callSeriesMerge(t *testing.T, h *handler, user *models.User, targetID, sourceID int) error {
	t.Helper()
	return callSeriesHandler(t, h.merge, user, targetID, fmt.Sprintf(`{"source_id":%d}`, sourceID))
}

func seriesUserWithAccess(libraryID int) *models.User {
	return &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: &libraryID}}}
}

func requireSeriesErr(t *testing.T, err error, status int) {
	t.Helper()
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, status, codeErr.HTTPCode, "error: %s", codeErr.Message)
}

func seriesExists(t *testing.T, db *bun.DB, id int) bool {
	t.Helper()
	count, err := db.NewSelect().Model((*models.Series)(nil)).Where("id = ?", id).Count(context.Background())
	require.NoError(t, err)
	return count == 1
}

// A Book in both Series keeps one row, the target's, and the target row
// takes the source's number group when it has none of its own.
func TestMergeSeries_SharedBook_DropsSourceRow(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createSeriesDeleteLibrary(t, db)
	source := createNamedSeries(t, svc, lib, "Source")
	target := createNamedSeries(t, svc, lib, "Target")
	shared := createSeriesBook(t, db, lib, nil, source.ID, target.ID)
	moved := createSeriesBook(t, db, lib, nil, source.ID)
	number, end, unit := 3.0, 4.0, models.SeriesNumberUnitVolume
	_, err := db.NewUpdate().Model((*models.BookSeries)(nil)).
		Set("series_number = ?, series_number_end = ?, series_number_unit = ?", number, end, unit).
		Where("book_id = ? AND series_id = ?", shared, source.ID).Exec(ctx)
	require.NoError(t, err)

	_, err = svc.MergeSeries(ctx, target.ID, source.ID)
	require.NoError(t, err)

	var rows []*models.BookSeries
	require.NoError(t, db.NewSelect().Model(&rows).Where("bs.book_id = ?", shared).Scan(ctx))
	require.Len(t, rows, 1, "the shared Book keeps one membership")
	assert.Equal(t, target.ID, rows[0].SeriesID)
	require.NotNil(t, rows[0].SeriesNumber)
	assert.InDelta(t, number, *rows[0].SeriesNumber, 0)
	require.NotNil(t, rows[0].SeriesNumberEnd)
	assert.InDelta(t, end, *rows[0].SeriesNumberEnd, 0)
	require.NotNil(t, rows[0].SeriesNumberUnit)
	assert.Equal(t, unit, *rows[0].SeriesNumberUnit)

	_, seriesIDs := retrieveSeriesBook(t, db, moved)
	assert.Equal(t, []int{target.ID}, seriesIDs, "a Book only in the source moves to the target")
	assert.False(t, seriesExists(t, db, source.ID))
}

// The target's own number wins over the source's for a shared Book.
func TestMergeSeries_SharedBook_KeepsTargetNumber(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	ctx := context.Background()
	svc := NewService(db)
	lib := createSeriesDeleteLibrary(t, db)
	source := createNamedSeries(t, svc, lib, "Source")
	target := createNamedSeries(t, svc, lib, "Target")
	shared := createSeriesBook(t, db, lib, nil, source.ID, target.ID)
	_, err := db.NewUpdate().Model((*models.BookSeries)(nil)).Set("series_number = 7").Where("book_id = ? AND series_id = ?", shared, source.ID).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewUpdate().Model((*models.BookSeries)(nil)).Set("series_number = 2").Where("book_id = ? AND series_id = ?", shared, target.ID).Exec(ctx)
	require.NoError(t, err)

	_, err = svc.MergeSeries(ctx, target.ID, source.ID)
	require.NoError(t, err)

	var rows []*models.BookSeries
	require.NoError(t, db.NewSelect().Model(&rows).Where("bs.book_id = ?", shared).Scan(ctx))
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].SeriesNumber)
	assert.InDelta(t, 2.0, *rows[0].SeriesNumber, 0)
}

func TestMergeSeries_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	lib := createSeriesDeleteLibrary(t, db)
	series := createNamedSeries(t, h.seriesService, lib, "Only")
	bookID := createSeriesBook(t, db, lib, nil, series.ID)

	err := callSeriesMerge(t, h, seriesUserWithAccess(lib.ID), series.ID, series.ID)
	requireSeriesErr(t, err, http.StatusUnprocessableEntity)

	assert.True(t, seriesExists(t, db, series.ID), "the Series still exists")
	_, seriesIDs := retrieveSeriesBook(t, db, bookID)
	assert.Equal(t, []int{series.ID}, seriesIDs, "the Book keeps its membership")
}

// The service rejects a self-merge on its own, for callers that skip the
// handler's checks.
func TestMergeSeriesService_SelfMerge_Rejected(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	svc := NewService(db)
	lib := createSeriesDeleteLibrary(t, db)
	series := createNamedSeries(t, svc, lib, "Only")

	_, err := svc.MergeSeries(context.Background(), series.ID, series.ID)
	requireSeriesErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, seriesExists(t, db, series.ID))
}

func TestMergeSeries_SourceInInaccessibleLibrary_Forbidden(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	visible := createSeriesDeleteLibrary(t, db)
	hidden := createSeriesDeleteLibrary(t, db)
	target := createNamedSeries(t, h.seriesService, visible, "Target")
	source := createNamedSeries(t, h.seriesService, hidden, "Source")

	err := callSeriesMerge(t, h, seriesUserWithAccess(visible.ID), target.ID, source.ID)
	requireSeriesErr(t, err, http.StatusForbidden)
	assert.True(t, seriesExists(t, db, source.ID), "the source Series still exists")
}

func TestMergeSeries_SourceInOtherLibrary_Rejected(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	first := createSeriesDeleteLibrary(t, db)
	second := createSeriesDeleteLibrary(t, db)
	target := createNamedSeries(t, h.seriesService, first, "Target")
	source := createNamedSeries(t, h.seriesService, second, "Source")
	allAccess := &models.User{LibraryAccess: []*models.UserLibraryAccess{{LibraryID: nil}}}

	err := callSeriesMerge(t, h, allAccess, target.ID, source.ID)
	requireSeriesErr(t, err, http.StatusUnprocessableEntity)
	assert.True(t, seriesExists(t, db, source.ID), "the source Series still exists")
}

func TestMergeSeries_MissingSource_NotFound(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	lib := createSeriesDeleteLibrary(t, db)
	target := createNamedSeries(t, h.seriesService, lib, "Target")

	err := callSeriesMerge(t, h, seriesUserWithAccess(lib.ID), target.ID, target.ID+1000)
	requireSeriesErr(t, err, http.StatusNotFound)
	assert.True(t, seriesExists(t, db, target.ID), "the target Series still exists")
}

// Renaming a Series onto another Series' name (in any case) is rejected
// instead of tripping ux_series_name_library_id.
func TestUpdateSeries_RenameToExistingName(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	lib := createSeriesDeleteLibrary(t, db)
	createNamedSeries(t, h.seriesService, lib, "Stormlight")
	other := createNamedSeries(t, h.seriesService, lib, "Mistborn")

	err := callSeriesHandler(t, h.update, seriesUserWithAccess(lib.ID), other.ID, `{"name":"stormlight"}`)
	requireSeriesErr(t, err, http.StatusUnprocessableEntity)

	unchanged, err := h.seriesService.RetrieveSeries(context.Background(), RetrieveSeriesOptions{ID: &other.ID})
	require.NoError(t, err)
	assert.Equal(t, "Mistborn", unchanged.Name)
}

// Changing only the case of a Series' own name is not a collision.
func TestUpdateSeries_RenameCaseOnly_Succeeds(t *testing.T) {
	t.Parallel()
	db := setupSeriesTestDB(t)
	h := newMergeTestHandler(db)
	lib := createSeriesDeleteLibrary(t, db)
	series := createNamedSeries(t, h.seriesService, lib, "Mistborn")

	err := callSeriesHandler(t, h.update, seriesUserWithAccess(lib.ID), series.ID, `{"name":"MISTBORN"}`)
	require.NoError(t, err)

	renamed, err := h.seriesService.RetrieveSeries(context.Background(), RetrieveSeriesOptions{ID: &series.ID})
	require.NoError(t, err)
	assert.Equal(t, "MISTBORN", renamed.Name)
}
