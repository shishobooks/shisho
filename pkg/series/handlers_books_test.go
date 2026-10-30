package series

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// seedSeriesWithBooks creates a library and a series with the given book
// titles attached in order (series numbers 1..n). Returns the series ID.
func seedSeriesWithBooks(t *testing.T, db *bun.DB, bookTitles []string) int {
	t.Helper()
	ctx := context.Background()

	library := &models.Library{
		Name:             "Books Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	seriesRecord := &models.Series{
		LibraryID:      library.ID,
		Name:           "Test Series",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Test Series",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err = db.NewInsert().Model(seriesRecord).Exec(ctx)
	require.NoError(t, err)

	for i, title := range bookTitles {
		book := &models.Book{
			LibraryID:       library.ID,
			Title:           title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        t.TempDir(),
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		sn := float64(i + 1)
		_, err = db.NewInsert().Model(&models.BookSeries{
			BookID: book.ID, SeriesID: seriesRecord.ID, SeriesNumber: &sn, SortOrder: i + 1,
		}).Exec(ctx)
		require.NoError(t, err)
	}

	return seriesRecord.ID
}

func getSeriesBooks(t *testing.T, h *handler, seriesID int, query string) *httptest.ResponseRecorder {
	t.Helper()

	e := newTestEchoSeries(t)
	req := httptest.NewRequest(http.MethodGet, "/"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAllAccessUser(c)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(seriesID))

	err := h.seriesBooks(c)
	if err != nil {
		e.HTTPErrorHandler(err, c)
	}
	return rec
}

func TestSeriesBooks_ResponseShape(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seriesID := seedSeriesWithBooks(t, db, nil)
	h := newSeriesHandler(db)

	rec := getSeriesBooks(t, h, seriesID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	assert.True(t, hasItems, "response must have 'items' key")
	assert.True(t, hasTotal, "response must have 'total' key")
	assert.Len(t, raw, 2, "response must have exactly 2 keys")
}

func TestSeriesBooks_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	// Seed one more book than the default limit so the test pins the
	// default-limit=24 contract instead of passing for any bind default.
	titles := make([]string, 25)
	for i := range titles {
		titles[i] = fmt.Sprintf("Book %02d", i+1)
	}
	seriesID := seedSeriesWithBooks(t, db, titles)
	h := newSeriesHandler(db)

	rec := getSeriesBooks(t, h, seriesID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []models.Book `json:"items"`
		Total int           `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, 25, resp.Total)
	require.Len(t, resp.Items, 24, "default limit must be 24")
	assert.Equal(t, "Book 01", resp.Items[0].Title, "books must be ordered by series number")
}

func TestSeriesBooks_OmnibusOrderingMatrix(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	titles := []string{
		"Single One Beta", "Omnibus Long", "Single Two", "Omnibus Short",
		"Fractional Prequel", "Unnumbered", "Single One Alpha",
	}
	seriesID := seedSeriesWithBooks(t, db, titles)

	updates := []struct {
		title string
		start *float64
		end   *float64
	}{
		{title: "Single One Beta", start: float64Pointer(1)},
		{title: "Omnibus Long", start: float64Pointer(1), end: float64Pointer(4)},
		{title: "Single Two", start: float64Pointer(2)},
		{title: "Omnibus Short", start: float64Pointer(1), end: float64Pointer(3)},
		{title: "Fractional Prequel", start: float64Pointer(0.5)},
		{title: "Unnumbered"},
		{title: "Single One Alpha", start: float64Pointer(1)},
	}
	for _, update := range updates {
		_, err := db.NewUpdate().
			Table("book_series").
			Set("series_number = ?", update.start).
			Set("series_number_end = ?", update.end).
			Where("book_id = (SELECT id FROM books WHERE title = ?)", update.title).
			Exec(ctx)
		require.NoError(t, err)
	}

	h := newSeriesHandler(db)
	rec := getSeriesBooks(t, h, seriesID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []models.Book `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, len(titles))
	got := make([]string, len(resp.Items))
	for i := range resp.Items {
		got[i] = resp.Items[i].Title
	}
	assert.Equal(t, []string{
		"Unnumbered", "Fractional Prequel", "Single One Alpha", "Single One Beta",
		"Single Two", "Omnibus Short", "Omnibus Long",
	}, got)
}

func float64Pointer(value float64) *float64 { return &value }

func TestSeriesBooks_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seriesID := seedSeriesWithBooks(t, db, []string{"Alpha", "Beta", "Charlie", "Delta", "Echo"})
	h := newSeriesHandler(db)

	rec := getSeriesBooks(t, h, seriesID, "?limit=2&offset=1")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []models.Book `json:"items"`
		Total int           `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, 5, resp.Total)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "Beta", resp.Items[0].Title)
	assert.Equal(t, "Charlie", resp.Items[1].Title)
}

func seriesBookTitles(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ListSeriesBooksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	titles := make([]string, len(resp.Items))
	for i, b := range resp.Items {
		titles[i] = b.Title
	}
	return titles
}

func TestSeriesBooks_SortParamOverridesSeriesOrder(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	// Insertion (id) order is Bravo, Alpha, Charlie. Renumbering makes series
	// order Charlie, Alpha, Bravo, so neither id nor title order can pass for it.
	seriesID := seedSeriesWithBooks(t, db, []string{"Bravo", "Alpha", "Charlie"})
	for title, number := range map[string]float64{"Bravo": 3, "Alpha": 2, "Charlie": 1} {
		_, err := db.NewUpdate().Model((*models.BookSeries)(nil)).
			Set("series_number = ?", number).
			Where("series_id = ? AND book_id = (SELECT id FROM books WHERE title = ?)", seriesID, title).
			Exec(context.Background())
		require.NoError(t, err)
	}
	h := newSeriesHandler(db)

	// No sort keeps series order.
	assert.Equal(t, []string{"Charlie", "Alpha", "Bravo"}, seriesBookTitles(t, getSeriesBooks(t, h, seriesID, "")))
	// An explicit sort replaces it.
	assert.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, seriesBookTitles(t, getSeriesBooks(t, h, seriesID, "?sort=title:asc")))
}

func TestSeriesBooks_InvalidSortReturns422(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	seriesID := seedSeriesWithBooks(t, db, []string{"Alpha"})

	rec := getSeriesBooks(t, newSeriesHandler(db), seriesID, "?sort=book_count:desc")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

// A series' books carry the caller's own Reading Status, never another
// user's.
func TestSeriesBooks_AttachesOnlyTheCallersReadingStatus(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	seriesID := seedSeriesWithBooks(t, db, []string{"One", "Two"})
	h := newSeriesHandler(db)

	reader := &models.User{Username: "reader", PasswordHash: "x", RoleID: 2, IsActive: true}
	other := &models.User{Username: "other", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err := db.NewInsert().Model(reader).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)

	var first models.BookSeries
	require.NoError(t, db.NewSelect().Model(&first).Where("series_id = ? AND sort_order = 1", seriesID).Scan(ctx))
	require.NoError(t, reading.NewService(db).Start(ctx, reader.ID, first.BookID, "2026-03-01"))

	statuses := func(user *models.User) map[int]*string {
		e := newTestEchoSeries(t)
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		user.LibraryAccess = []*models.UserLibraryAccess{{}}
		auth.SetUser(c, user)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(seriesID))
		require.NoError(t, h.seriesBooks(c))
		var resp ListSeriesBooksResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		out := map[int]*string{}
		for _, b := range resp.Items {
			out[b.ID] = b.ReadingStatus
		}
		return out
	}

	got := statuses(reader)
	require.NotNil(t, got[first.BookID])
	assert.Equal(t, models.ReadingStatusCurrentlyReading, *got[first.BookID])
	assert.Nil(t, statuses(other)[first.BookID])
}

// Sorting a series' books by rating orders them by the caller's own Rating.
func TestSeriesBooks_SortsByTheCallersRating(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	seriesID := seedSeriesWithBooks(t, db, []string{"One", "Two", "Three"})
	h := newSeriesHandler(db)
	reader := &models.User{Username: "reader", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err := db.NewInsert().Model(reader).Exec(ctx)
	require.NoError(t, err)

	var bookIDs []int
	require.NoError(t, db.NewSelect().Model((*models.BookSeries)(nil)).Column("book_id").Where("series_id = ?", seriesID).Order("sort_order").Scan(ctx, &bookIDs))
	svc := reading.NewService(db)
	for i, units := range []int{8, 20, 12} {
		require.NoError(t, svc.SetRating(ctx, reader.ID, bookIDs[i], &units))
	}

	e := newTestEchoSeries(t)
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/?sort=rating:desc", nil), rec)
	reader.LibraryAccess = []*models.UserLibraryAccess{{}}
	auth.SetUser(c, reader)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(seriesID))
	require.NoError(t, h.seriesBooks(c))

	var resp ListSeriesBooksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	got := []int{}
	for _, b := range resp.Items {
		got = append(got, b.ID)
	}
	assert.Equal(t, []int{bookIDs[1], bookIDs[2], bookIDs[0]}, got)
}
