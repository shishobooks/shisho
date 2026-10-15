package genres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListGenres_SortsByBookCount(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	seedGenreWithBooks(t, db, lib, "One", []string{"A"})
	seedGenreWithBooks(t, db, lib, "Three", []string{"B", "C", "D"})
	seedGenreWithBooks(t, db, lib, "Two", []string{"E", "F"})

	genres, err := NewService(db).ListGenres(context.Background(), ListGenresOptions{
		LibraryID: &lib.ID,
		Sort:      []sortspec.SortLevel{{Field: sortspec.FieldBookCount, Direction: sortspec.DirDesc}},
	})
	require.NoError(t, err)
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	assert.Equal(t, []string{"Three", "Two", "One"}, names)
}

// The list handler passes the sort through, and date_added orders by the
// creation time rather than the name.
func TestListGenresHandler_SortsByDateAdded(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, seed := range []struct {
		name string
		age  time.Duration
	}{{"Oldest", 0}, {"Newest", 2 * time.Hour}, {"Middle", time.Hour}} {
		require.NoError(t, NewService(db).CreateGenre(ctx, &models.Genre{LibraryID: lib.ID, Name: seed.name, CreatedAt: base.Add(seed.age)}))
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?library_id=%d&sort=date_added:desc", lib.ID), nil)
	rec := httptest.NewRecorder()
	c := newTestEcho(t).NewContext(req, rec)
	setAllAccessUser(c)
	require.NoError(t, newTestHandler(t, db).list(c))

	var resp ListGenresResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	names := make([]string, len(resp.Items))
	for i, item := range resp.Items {
		names[i] = item.Name
	}
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, names)
}
