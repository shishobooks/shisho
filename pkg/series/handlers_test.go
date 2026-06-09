package series

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newSeriesHandler builds a handler wired with the services the list/retrieve
// endpoints exercise (series, books, libraries, aliases).
func newSeriesHandler(db *bun.DB) *handler {
	return &handler{
		seriesService:  NewService(db),
		bookService:    books.NewService(db),
		libraryService: libraries.NewService(db),
		aliasService:   aliases.NewService(db),
	}
}

// seedSeriesWithAliases creates a library, a series with a book, and two
// aliases for that series. Returns the series ID.
func seedSeriesWithAliases(ctx context.Context, t *testing.T, db *bun.DB) int {
	t.Helper()

	library := &models.Library{
		Name:                     "Alias Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	seriesRecord := &models.Series{
		LibraryID:      library.ID,
		Name:           "Mistborn",
		NameSource:     string(models.DataSourceFilepath),
		SortName:       "Mistborn",
		SortNameSource: string(models.DataSourceFilepath),
	}
	_, err = db.NewInsert().Model(seriesRecord).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "The Final Empire",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "The Final Empire",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	sn := 1.0
	_, err = db.NewInsert().Model(&models.BookSeries{
		BookID: book.ID, SeriesID: seriesRecord.ID, SeriesNumber: &sn, SortOrder: 1,
	}).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO series_aliases (created_at, series_id, name, library_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		time.Now(), seriesRecord.ID, "Final Empire", library.ID,
		time.Now(), seriesRecord.ID, "Era 1", library.ID,
	).Exec(ctx)
	require.NoError(t, err)

	return seriesRecord.ID
}

func TestSeriesList_ResponseEnvelopeAndAliasesSerializeAsStringArray(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	seedSeriesWithAliases(ctx, t, db)

	h := newSeriesHandler(db)
	e := newTestEchoSeries(t)
	req := httptest.NewRequest(http.MethodGet, "/series?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.list(c))
	require.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasSeries := raw["series"]
	assert.True(t, hasItems, "list response must have 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.False(t, hasSeries, "list response must NOT use 'series' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	// Each item must carry book_count and serialize aliases as a JSON array of
	// strings (the #324 fix at the wire level), not relation objects.
	var resp struct {
		Items []struct {
			ID        int             `json:"id"`
			Name      string          `json:"name"`
			BookCount int             `json:"book_count"`
			Aliases   json.RawMessage `json:"aliases"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Mistborn", resp.Items[0].Name)
	assert.Equal(t, 1, resp.Items[0].BookCount)

	var aliasStrings []string
	require.NoError(t, json.Unmarshal(resp.Items[0].Aliases, &aliasStrings),
		"aliases must unmarshal into []string, proving it is a JSON array of strings")
	assert.ElementsMatch(t, []string{"Final Empire", "Era 1"}, aliasStrings)
}

func TestSeriesRetrieve_ResponseAliasesSerializeAsStringArray(t *testing.T) {
	t.Parallel()

	db := setupSeriesTestDB(t)
	ctx := context.Background()
	seriesID := seedSeriesWithAliases(ctx, t, db)

	h := newSeriesHandler(db)
	e := newTestEchoSeries(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(seriesID))

	require.NoError(t, h.retrieve(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		ID        int             `json:"id"`
		Name      string          `json:"name"`
		BookCount int             `json:"book_count"`
		Aliases   json.RawMessage `json:"aliases"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Mistborn", resp.Name)
	assert.Equal(t, 1, resp.BookCount)

	var aliasStrings []string
	require.NoError(t, json.Unmarshal(resp.Aliases, &aliasStrings),
		"aliases must unmarshal into []string, proving it is a JSON array of strings")
	assert.ElementsMatch(t, []string{"Final Empire", "Era 1"}, aliasStrings)
}
