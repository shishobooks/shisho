package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/sharelinks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every book-carrying endpoint returns each file's display_name, resolved on
// the server. Bun runs no row hooks for has-many relations, so Book.Files
// loaders must call models.ResolveBookFileDisplayNames; this test catches a
// loader that forgets. A supplement shows its filename, and a main file
// without a name its filename (except in the Share Link payload).
func TestBookPayloadsCarryFileDisplayNames(t *testing.T) {
	t.Parallel()
	f := newShareLinksFixture(t)
	ctx := context.Background()

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
	get := func(t *testing.T, path string, out any) {
		t.Helper()
		rec := f.do(f.admin, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out))
	}

	t.Run("book detail", func(t *testing.T) {
		t.Parallel()
		var book models.Book
		get(t, fmt.Sprintf("/api/books/%d", f.bookA.ID), &book)
		assertNames(t, book.Files)
	})

	t.Run("book list", func(t *testing.T) {
		t.Parallel()
		var resp books.ListBooksResponse
		get(t, fmt.Sprintf("/api/books?library_id=%d", f.libA.ID), &resp)
		require.Len(t, resp.Items, 1)
		assertNames(t, resp.Items[0].Files)
	})

	t.Run("series books", func(t *testing.T) {
		t.Parallel()
		var s models.Series
		require.NoError(t, f.db.NewSelect().Model(&s).Where("name = ?", "Saga").Scan(ctx))
		var resp series.ListSeriesBooksResponse
		get(t, fmt.Sprintf("/api/series/%d/books", s.ID), &resp)
		require.Len(t, resp.Items, 1)
		assertNames(t, resp.Items[0].Files)
	})

	t.Run("list books", func(t *testing.T) {
		t.Parallel()
		rec := f.do(f.admin, http.MethodPost, "/api/lists", `{"name":"Display names"}`)
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		var list models.List
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		rec = f.do(f.admin, http.MethodPost, fmt.Sprintf("/api/lists/%d/books", list.ID), fmt.Sprintf(`{"book_ids":[%d]}`, f.bookA.ID))
		require.Less(t, rec.Code, 300, rec.Body.String())

		var resp lists.ListListBooksResponse
		get(t, fmt.Sprintf("/api/lists/%d/books", list.ID), &resp)
		require.Len(t, resp.Items, 1)
		require.NotNil(t, resp.Items[0].Book)
		assertNames(t, resp.Items[0].Book.Files)
	})

	t.Run("share link payload", func(t *testing.T) {
		t.Parallel()
		f.setSharing(true, false)
		link := f.mustCreate(f.sharer, f.bookA, fmt.Sprintf(`{"expires_at":%q}`, futureJSON(24*time.Hour)))
		rec := f.do(nil, http.MethodGet, "/api/share/"+link.Token, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var shared sharelinks.SharedBookResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &shared))
		require.Len(t, shared.Files, 2)
		for _, file := range shared.Files {
			if file.FileRole == models.FileRoleSupplement {
				assert.Equal(t, "notes.pdf", file.DisplayName)
			} else {
				// A nameless main file gets no label for a recipient, not its
				// on-disk filename; the page shows its type.
				assert.Empty(t, file.DisplayName)
			}
		}
	})
}
