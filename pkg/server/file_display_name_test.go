package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Book payloads carry each file's display_name, resolved on the server: a
// supplement shows its filename, a main file without a name its filename.
func TestBookPayloadsCarryFileDisplayNames(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)

	assertNames := func(t *testing.T, files []*models.File) {
		t.Helper()
		require.Len(t, files, 2)
		for _, file := range files {
			if file.FileRole == models.FileRoleSupplement {
				assert.Equal(t, "notes.pdf", file.DisplayName)
			} else {
				assert.Equal(t, "Alpha.epub", file.DisplayName)
			}
		}
	}

	t.Run("book detail", func(t *testing.T) {
		t.Parallel()
		rec := f.do(f.admin, http.MethodGet, fmt.Sprintf("/api/books/%d", f.bookA.ID), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var book models.Book
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &book))
		assertNames(t, book.Files)
	})

	t.Run("book list", func(t *testing.T) {
		t.Parallel()
		rec := f.do(f.admin, http.MethodGet, fmt.Sprintf("/api/books?library_id=%d", f.libA.ID), "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var resp books.ListBooksResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 1)
		assertNames(t, resp.Items[0].Files)
	})
}
