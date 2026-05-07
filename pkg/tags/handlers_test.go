package tags

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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

// setupHandlerTestDB creates an in-memory SQLite database using a named memory
// URI so that Bun's ScanAndCount (which opens a second connection for the COUNT
// query) sees the same database. Plain ":memory:" gives each connection its own
// private database, which causes "no such table" errors from ScanAndCount.
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

func newTestHandler(db *bun.DB) *handler {
	return &handler{
		tagService:    NewService(db),
		aliasService:  aliases.NewService(db),
		searchService: search.NewService(db),
	}
}

func seedTagWithBooks(t *testing.T, db *bun.DB, lib *models.Library, tagName string, bookTitles []string) *models.Tag {
	t.Helper()
	ctx := context.Background()

	tag := &models.Tag{LibraryID: lib.ID, Name: tagName}
	_, err := db.NewInsert().Model(tag).Exec(ctx)
	require.NoError(t, err)

	for _, title := range bookTitles {
		book := &models.Book{
			LibraryID:       lib.ID,
			Title:           title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        t.TempDir(),
		}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)

		bt := &models.BookTag{BookID: book.ID, TagID: tag.ID}
		_, err = db.NewInsert().Model(bt).Exec(ctx)
		require.NoError(t, err)
	}

	return tag
}

func TestBooks_DefaultPagination(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	tag := seedTagWithBooks(t, db, lib, "Fantasy", []string{"Book A", "Book B", "Book C"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(tag.ID))

	err := h.books(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 3)
}

func TestBooks_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	tag := seedTagWithBooks(t, db, lib, "Sci-Fi", []string{"B1", "B2", "B3", "B4", "B5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(tag.ID))

	err := h.books(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	var total int
	err = json.Unmarshal(resp["total"], &total)
	require.NoError(t, err)
	assert.Equal(t, 5, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestBooks_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	tag := seedTagWithBooks(t, db, lib, "Horror", []string{"H1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(tag.ID))

	err := h.books(c)
	require.NoError(t, err)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp, 2, "response should have exactly 2 keys")
	assert.Contains(t, resp, "items")
	assert.Contains(t, resp, "total")
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupHandlerTestDB(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		tag := &models.Tag{LibraryID: lib.ID, Name: fmt.Sprintf("Tag %d", i)}
		_, err := db.NewInsert().Model(tag).Exec(ctx)
		require.NoError(t, err)
	}

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp, "items", "response should use 'items' key")
	assert.NotContains(t, resp, "tags", "response should not use 'tags' key")
	assert.Contains(t, resp, "total")

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}
