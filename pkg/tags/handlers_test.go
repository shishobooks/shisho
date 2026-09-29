package tags

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
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

// noopReviewRecomputer stands in for *books.Service, which this package
// cannot import. pkg/server/resource_delete_test.go covers the real recompute.
type noopReviewRecomputer struct{}

func (noopReviewRecomputer) RecomputeReviewedForBooks(context.Context, []int) {}

func newTestHandler(db *bun.DB) *handler {
	return &handler{
		tagService:       NewService(db),
		aliasService:     aliases.NewService(db),
		searchService:    search.NewService(db),
		reviewRecomputer: noopReviewRecomputer{},
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
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	// Seed one more book than the default limit so the test pins the
	// default-limit=24 contract instead of passing for any bind default.
	titles := make([]string, 25)
	for i := range titles {
		titles[i] = fmt.Sprintf("Book %02d", i+1)
	}
	tag := seedTagWithBooks(t, db, lib, "Fantasy", titles)

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAllAccessUser(c)
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
	assert.Equal(t, 25, total)

	var items []json.RawMessage
	err = json.Unmarshal(resp["items"], &items)
	require.NoError(t, err)
	require.Len(t, items, 24, "default limit must be 24")
}

func TestBooks_ExplicitLimitOffset(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	tag := seedTagWithBooks(t, db, lib, "Sci-Fi", []string{"B1", "B2", "B3", "B4", "B5"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAllAccessUser(c)
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
	db := testdb.New(t)
	lib := createTestLibrary(t, db)
	h := newTestHandler(db)

	tag := seedTagWithBooks(t, db, lib, "Horror", []string{"H1"})

	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAllAccessUser(c)
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
	db := testdb.New(t)
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
	setAllAccessUser(c)

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

func TestList_ResponseAliasesSerializeAsStringArray(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	e := newTestEcho(t)
	lib := createTestLibrary(t, db)
	tag := seedTagWithBooks(t, db, lib, "Science Fiction", []string{"Book1"})
	h := newTestHandler(db)

	// Seed two aliases for the tag so we can assert they round-trip as strings.
	_, err := db.NewRaw(
		"INSERT INTO tag_aliases (created_at, tag_id, name, library_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)",
		time.Now(), tag.ID, "SciFi", lib.ID,
		time.Now(), tag.ID, "SF", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAllAccessUser(c)

	err = h.list(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	assert.True(t, hasItems, "list response must have 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	// Each item's aliases must serialize as a JSON array of strings (the #324 fix
	// at the wire level), and book_count must be present.
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
	assert.Equal(t, "Science Fiction", resp.Items[0].Name)
	assert.Equal(t, 1, resp.Items[0].BookCount)

	// aliases must be a JSON array whose elements are strings, not objects.
	var aliasStrings []string
	require.NoError(t, json.Unmarshal(resp.Items[0].Aliases, &aliasStrings),
		"aliases must unmarshal into []string, proving it is a JSON array of strings")
	assert.ElementsMatch(t, []string{"SciFi", "SF"}, aliasStrings)
}

// setAllAccessUser stands in for the Authenticate middleware in handler tests
// that call a handler directly: a user with access to every library. The user
// is not in the database and has no Role, so a test that checks permissions
// or writes rows referencing the user must load a real one instead.
func setAllAccessUser(c echo.Context) {
	auth.SetUser(c, &models.User{ID: 1, LibraryAccess: []*models.UserLibraryAccess{{}}})
}

// serveWithoutUser serves GET path through a router that registers the
// handler without the Authenticate middleware, so no user is in context.
func serveWithoutUser(e *echo.Echo, route string, h echo.HandlerFunc, path string) int {
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.GET(route, h)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

// A handler reached with no user in context rejects the request with 401
// instead of skipping the library access check.
func TestRetrieve_NoUserInContext_Returns401(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	tag := seedTagWithBooks(t, db, createTestLibrary(t, db), "Favorites", nil)

	code := serveWithoutUser(newTestEcho(t), "/tags/:id", newTestHandler(db).retrieve, fmt.Sprintf("/tags/%d", tag.ID))
	assert.Equal(t, http.StatusUnauthorized, code)
}
