package genres

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newTestEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return e
}

func newTestHandler(t *testing.T, db *bun.DB) *handler {
	t.Helper()
	return &handler{
		genreService:  NewService(db),
		aliasService:  aliases.NewService(db),
		searchService: search.NewService(db),
	}
}

func patchGenre(t *testing.T, h *handler, genreID int, payload UpdateGenrePayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e := newTestEcho(t)
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(genreID))

	err = h.update(c)
	if err != nil {
		e.HTTPErrorHandler(err, c)
	}
	return rec
}

func TestUpdateGenre_RenameAddsOldNameAsAlias(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	newName := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Science Fiction")
}

func TestUpdateGenre_RenameWithPreExistingAlias_NoDuplicate(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// Pre-add "Science Fiction" as an alias manually (simulate user adding it before rename)
	_, err := db.NewRaw(
		"INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), genre.ID, "SF", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	newName := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Science Fiction", "old name should be added as alias")
	assert.Contains(t, aliasList, "SF", "existing alias should be preserved")
}

func TestUpdateGenre_RenameBackToOriginalName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// First rename: "Science Fiction" → "Sci-Fi"
	newName := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code)

	// Second rename back: "Sci-Fi" → "Science Fiction"
	originalName := "Science Fiction"
	rec = patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &originalName})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Sci-Fi", "previous name should be an alias")
	assert.NotContains(t, aliasList, "Science Fiction", "alias matching new primary name should be removed")
}
