package series

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
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
		Name:                     "Books Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
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
	db := setupSeriesTestDB(t)
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
	db := setupSeriesTestDB(t)
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
	db := setupSeriesTestDB(t)
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
	db := setupSeriesTestDB(t)
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
