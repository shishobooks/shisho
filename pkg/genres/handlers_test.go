package genres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupHandlerTestDB(t *testing.T) *bun.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

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

func TestUpdateGenre_RenameWithoutAliasesDoesNotAutoAdd(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// Rename without sending aliases — backend should NOT auto-add old name
	newName := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Empty(t, aliasList, "backend should not auto-add old name as alias; frontend handles this")
}

func TestUpdateGenre_RenameWithAliasesIncludingOldName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// Rename and send old name in aliases (as the frontend now does)
	newName := "Sci-Fi"
	aliasPayload := []string{"Science Fiction"}
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName, Aliases: aliasPayload})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Science Fiction")
}

func TestUpdateGenre_RenameWithAliasesPreservesExisting(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	_, err := db.NewRaw(
		"INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), genre.ID, "SF", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	// Frontend sends full desired alias list including old name and existing aliases
	newName := "Sci-Fi"
	aliasPayload := []string{"SF", "Science Fiction"}
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName, Aliases: aliasPayload})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Science Fiction", "old name should be in aliases")
	assert.Contains(t, aliasList, "SF", "existing alias should be preserved")
}

func TestUpdateGenre_SequentialRenames_FrontendSendsAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// First rename: frontend sends old name as alias
	nameB := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &nameB, Aliases: []string{"Science Fiction"}})
	require.Equal(t, http.StatusOK, rec.Code)

	// Second rename: frontend sends both previous aliases
	nameC := "SF"
	rec = patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &nameC, Aliases: []string{"Science Fiction", "Sci-Fi"}})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Science Fiction", "Sci-Fi"}, aliasList)
}

func seedGenreWithBooks(t *testing.T, db *bun.DB, lib *models.Library, genreName string, bookTitles []string) *models.Genre {
	t.Helper()
	ctx := context.Background()

	genre := &models.Genre{LibraryID: lib.ID, Name: genreName}
	_, err := db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)

	for _, title := range bookTitles {
		book := &models.Book{
			LibraryID:       lib.ID,
			Title:           title,
			Filepath:        t.TempDir(),
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
		}
		_, err = db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		bg := &models.BookGenre{BookID: book.ID, GenreID: genre.ID}
		_, err = db.NewInsert().Model(bg).Exec(ctx)
		require.NoError(t, err)
	}

	return genre
}

func TestBooks_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	genre := seedGenreWithBooks(t, db, lib, "Fiction", []string{"Alpha", "Beta", "Charlie"})
	h := newTestHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(genre.ID))

	err := h.books(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []models.Book `json:"items"`
		Total int           `json:"total"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, 3, resp.Total)
	assert.Len(t, resp.Items, 3)
	assert.Equal(t, "Alpha", resp.Items[0].Title)
}

func TestBooks_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	genre := seedGenreWithBooks(t, db, lib, "Fiction", []string{"Alpha", "Beta", "Charlie", "Delta", "Echo"})
	h := newTestHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(genre.ID))

	err := h.books(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []models.Book `json:"items"`
		Total int           `json:"total"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, 5, resp.Total)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, "Beta", resp.Items[0].Title)
	assert.Equal(t, "Charlie", resp.Items[1].Title)
}

func TestBooks_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedGenreWithBooks(t, db, lib, "Empty Genre", nil)
	h := newTestHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("1")

	err := h.books(c)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &raw)
	require.NoError(t, err)

	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	assert.True(t, hasItems, "response must have 'items' key")
	assert.True(t, hasTotal, "response must have 'total' key")
	assert.Len(t, raw, 2, "response must have exactly 2 keys")
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	_ = seedGenreWithBooks(t, db, lib, "Fiction", []string{"Book1"})
	h := newTestHandler(t, db)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.list(c)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &raw)
	require.NoError(t, err)

	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasGenres := raw["genres"]
	assert.True(t, hasItems, "list response must use 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.False(t, hasGenres, "list response must NOT use 'genres' key")
}

func TestUpdateGenre_RenameBackToOriginalName_FrontendSendsAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	lib := createTestLibrary(t, db)
	h := newTestHandler(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	require.NoError(t, h.genreService.CreateGenre(ctx, genre))

	// First rename: "Science Fiction" → "Sci-Fi", frontend adds old name as alias
	newName := "Sci-Fi"
	rec := patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &newName, Aliases: []string{"Science Fiction"}})
	require.Equal(t, http.StatusOK, rec.Code)

	// Second rename back: "Sci-Fi" → "Science Fiction", frontend removes auto-added alias
	// and adds "Sci-Fi" as new alias
	originalName := "Science Fiction"
	rec = patchGenre(t, h, genre.ID, UpdateGenrePayload{Name: &originalName, Aliases: []string{"Sci-Fi"}})
	require.Equal(t, http.StatusOK, rec.Code)

	aliasList, err := h.aliasService.ListAliases(ctx, aliases.GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Contains(t, aliasList, "Sci-Fi", "previous name should be an alias")
	assert.NotContains(t, aliasList, "Science Fiction", "alias matching new primary name should be removed")
}
