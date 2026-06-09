package lists

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/models"
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

func newTestHandler(db *bun.DB) *handler {
	return &handler{listsService: NewService(db)}
}

// userWithUsersRead returns a User that has all-library access and users:read,
// so it passes the sharing permission checks used by checkVisibility.
func userWithUsersRead(t *testing.T, db *bun.DB, username string) *models.User {
	t.Helper()
	user := createTestUser(t, db, username)
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	user.Role = &models.Role{
		Permissions: []*models.Permission{
			{Resource: models.ResourceUsers, Operation: models.OperationRead},
		},
	}
	return user
}

func TestList_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	e := newTestEcho(t)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	h := newTestHandler(db)

	_, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "My List"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasLists := raw["lists"]
	assert.True(t, hasItems, "list response must use 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.False(t, hasLists, "list response must NOT use legacy 'lists' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	var resp ListListsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "My List", resp.Items[0].Name)
	assert.Equal(t, "owner", resp.Items[0].Permission)
}

func TestRetrieve_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	e := newTestEcho(t)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	h := newTestHandler(db)

	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "My List"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(list.ID))

	require.NoError(t, h.retrieve(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	// Embeds the List model (so its fields are flattened in) plus book_count and
	// permission. The legacy nested "list" key must be gone.
	_, hasList := raw["list"]
	assert.False(t, hasList, "retrieve response must NOT nest under a 'list' key")
	for _, key := range []string{"id", "name", "book_count", "permission"} {
		_, ok := raw[key]
		assert.Truef(t, ok, "retrieve response must have %q key", key)
	}

	var resp RetrieveListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "My List", resp.Name)
	assert.Equal(t, 0, resp.BookCount)
	assert.Equal(t, "owner", resp.Permission)
}

func TestListBooks_ResponseUsesItemsKey(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	e := newTestEcho(t)
	user := createTestUser(t, db, "owner")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	lib := createTestLibrary(t, db, "Lib")
	book := createTestBook(t, db, lib.ID, "Alpha")
	h := newTestHandler(db)

	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "My List"})
	require.NoError(t, err)
	require.NoError(t, h.listsService.AddBooks(t.Context(), AddBooksOptions{ListID: list.ID, BookIDs: []int{book.ID}, AddedByUserID: user.ID}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(list.ID))

	require.NoError(t, h.listBooks(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	_, hasBooks := raw["books"]
	assert.True(t, hasItems, "listBooks response must use 'items' key")
	assert.True(t, hasTotal, "listBooks response must have 'total' key")
	assert.False(t, hasBooks, "listBooks response must NOT use legacy 'books' key")
	assert.Len(t, raw, 2, "listBooks response must have exactly 'items' and 'total' keys")

	var resp ListListBooksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Items, 1)
	require.NotNil(t, resp.Items[0].Book)
	assert.Equal(t, "Alpha", resp.Items[0].Book.Title)
}

func TestCheckVisibility_ResponseShape(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	e := newTestEcho(t)
	user := userWithUsersRead(t, db, "owner")
	h := newTestHandler(db)

	list, err := h.listsService.CreateList(t.Context(), CreateListOptions{UserID: user.ID, Name: "My List"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/?user_id=1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(list.ID))

	require.NoError(t, h.checkVisibility(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasVisible := raw["visible"]
	_, hasTotal := raw["total"]
	assert.True(t, hasVisible, "checkVisibility response must have 'visible' key")
	assert.True(t, hasTotal, "checkVisibility response must have 'total' key")
	assert.Len(t, raw, 2, "checkVisibility response must have exactly 'visible' and 'total' keys")

	var resp CheckVisibilityResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Visible)
	assert.Equal(t, 0, resp.Total)
}

func TestTemplates_ResponseShape(t *testing.T) {
	t.Parallel()
	e := newTestEcho(t)
	h := &handler{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.templates(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp []ListTemplate
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp, 2)
	assert.Equal(t, "tbr", resp[0].Name)
	assert.Equal(t, "To Be Read", resp[0].DisplayName)
	assert.True(t, resp[0].IsOrdered)
	assert.Equal(t, models.ListSortManual, resp[0].DefaultSort)
	assert.Equal(t, "favorites", resp[1].Name)
	assert.Equal(t, models.ListSortAddedAtDesc, resp[1].DefaultSort)
}
