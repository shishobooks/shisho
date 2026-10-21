package publishers

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
	_ = seedPublisherWithFiles(t, db, lib, "Tor", []string{"One", "Two"})
	h := newTestHandler(db)

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	setAllAccessUser(c)

	require.NoError(t, h.list(c))

	var resp ListPublishersResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	// The seed puts both files on one book, which previews once.
	assert.Equal(t, []string{"Book for Tor"}, previewTitles(resp.Items[0].PreviewBooks))
}
