package genres

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shishobooks/shisho/pkg/bookpreviews"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func previewTitles(previews []bookpreviews.PreviewBook) []string {
	out := make([]string, len(previews))
	for i, p := range previews {
		out[i] = p.Title
	}
	return out
}

func TestList_IncludesPreviewBooks(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedGenreWithBooks(t, db, lib, "Fiction", []string{"One", "Two"})
	h := newTestHandler(t, db)

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	setAllAccessUser(c)

	require.NoError(t, h.list(c))

	var resp ListGenresResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.ElementsMatch(t, []string{"One", "Two"}, previewTitles(resp.Items[0].PreviewBooks))
}

func TestList_PreviewBooksIsAnEmptyArrayForABooklessGenre(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedGenreWithBooks(t, db, lib, "Empty", nil)
	h := newTestHandler(t, db)

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	setAllAccessUser(c)

	require.NoError(t, h.list(c))

	var raw struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	require.Len(t, raw.Items, 1)
	assert.JSONEq(t, "[]", string(raw.Items[0]["preview_books"]))
}
