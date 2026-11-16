package bookmarks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type fixture struct {
	t         *testing.T
	db        *bun.DB
	library   *models.Library
	book      *models.Book
	audiobook *models.File
	epub      *models.File
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testdb.New(t)
	f := &fixture{t: t, db: db}
	f.library = &models.Library{Name: "Audiobooks", CoverAspectRatio: "book"}
	f.insert(f.library)
	f.book = &models.Book{
		LibraryID: f.library.ID, Title: "Dune", TitleSource: models.DataSourceFilepath,
		SortTitle: "Dune", AuthorSource: models.DataSourceFilepath, Filepath: "/test/Dune",
	}
	f.insert(f.book)
	f.audiobook = f.addFile(models.FileTypeM4B, "dune.m4b")
	f.epub = f.addFile(models.FileTypeEPUB, "dune.epub")
	return f
}

func (f *fixture) insert(model any) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(model).Exec(context.Background())
	require.NoError(f.t, err)
}

func (f *fixture) addFile(fileType, name string) *models.File {
	f.t.Helper()
	file := &models.File{
		LibraryID: f.library.ID, BookID: f.book.ID, FileType: fileType, FileRole: models.FileRoleMain,
		Filepath: "/test/Dune/" + name, FilesizeBytes: 1,
	}
	f.insert(file)
	return file
}

// user creates a user who can reach every library, or only libraryIDs when
// given.
func (f *fixture) user(username string, libraryIDs ...int) *models.User {
	f.t.Helper()
	user := &models.User{Username: username, PasswordHash: "test", RoleID: 2, IsActive: true}
	f.insert(user)
	if len(libraryIDs) == 0 {
		user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID}}
	}
	for _, id := range libraryIDs {
		user.LibraryAccess = append(user.LibraryAccess, &models.UserLibraryAccess{UserID: user.ID, LibraryID: &id})
	}
	return user
}

func (f *fixture) count() int {
	f.t.Helper()
	n, err := f.db.NewSelect().Model((*models.Bookmark)(nil)).Count(context.Background())
	require.NoError(f.t, err)
	return int(n)
}

type api struct {
	t *testing.T
	e *echo.Echo
}

func newAPI(t *testing.T, db *bun.DB, user *models.User) *api {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	g := e.Group("/reading", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth.SetUser(c, user)
			return next(c)
		}
	})
	RegisterRoutes(g, db)
	return &api{t: t, e: e}
}

func (a *api) do(method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(a.t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "/reading"+path, reader)
	if body != nil {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	a.e.ServeHTTP(rec, req)
	return rec
}

func (a *api) list(fileID int) []models.Bookmark {
	a.t.Helper()
	rec := a.do(http.MethodGet, fmt.Sprintf("/files/%d/bookmarks", fileID), nil)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var items []models.Bookmark
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &items))
	return items
}

func (a *api) create(fileID int, body map[string]any) models.Bookmark {
	a.t.Helper()
	rec := a.do(http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", fileID), body)
	require.Equal(a.t, http.StatusCreated, rec.Code, rec.Body.String())
	var bookmark models.Bookmark
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &bookmark))
	return bookmark
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body.Error.Code
}

func TestBookmarks_CreateListEditAndDelete(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := newAPI(t, f.db, f.user("listener"))

	assert.Empty(t, a.list(f.audiobook.ID))

	later := a.create(f.audiobook.ID, map[string]any{"seconds": 600.5})
	start := a.create(f.audiobook.ID, map[string]any{"seconds": 0})
	assert.InDelta(t, 0, start.Seconds, 0, "the very start can be marked")
	require.Equal(t, http.StatusNoContent, a.do(http.MethodDelete, fmt.Sprintf("/bookmarks/%d", start.ID), nil).Code)
	earlier := a.create(f.audiobook.ID, map[string]any{"seconds": 83, "note": "  test  "})
	assert.Equal(t, f.audiobook.ID, earlier.FileID)
	assert.InDelta(t, 83, earlier.Seconds, 1e-9)
	require.NotNil(t, earlier.Note)
	assert.Equal(t, "test", *earlier.Note)
	assert.Nil(t, later.Note)
	assert.False(t, earlier.CreatedAt.IsZero())

	items := a.list(f.audiobook.ID)
	require.Len(t, items, 2)
	assert.Equal(t, []int{earlier.ID, later.ID}, []int{items[0].ID, items[1].ID}, "in time order")

	rec := a.do(http.MethodPatch, fmt.Sprintf("/bookmarks/%d", later.ID), map[string]any{"note": "the duel"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var edited models.Bookmark
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &edited))
	require.NotNil(t, edited.Note)
	assert.Equal(t, "the duel", *edited.Note)
	assert.InDelta(t, 600.5, edited.Seconds, 1e-9)

	rec = a.do(http.MethodPatch, fmt.Sprintf("/bookmarks/%d", earlier.ID), map[string]any{"note": "   "})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	items = a.list(f.audiobook.ID)
	assert.Nil(t, items[0].Note, "a blank note clears it")
	require.NotNil(t, items[1].Note)
	assert.Equal(t, "the duel", *items[1].Note)

	rec = a.do(http.MethodPatch, fmt.Sprintf("/bookmarks/%d", later.ID), map[string]any{"note": ""})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Nil(t, a.list(f.audiobook.ID)[1].Note, "an empty note clears it")

	rec = a.do(http.MethodDelete, fmt.Sprintf("/bookmarks/%d", earlier.ID), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	items = a.list(f.audiobook.ID)
	require.Len(t, items, 1)
	assert.Equal(t, later.ID, items[0].ID)

	rec = a.do(http.MethodDelete, fmt.Sprintf("/bookmarks/%d", earlier.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestBookmarks_PrivateToTheirUser(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	owner := newAPI(t, f.db, f.user("owner"))
	other := newAPI(t, f.db, f.user("other"))

	mine := owner.create(f.audiobook.ID, map[string]any{"seconds": 83, "note": "mine"})
	theirs := other.create(f.audiobook.ID, map[string]any{"seconds": 10})

	listed := other.list(f.audiobook.ID)
	require.Len(t, listed, 1)
	assert.Equal(t, theirs.ID, listed[0].ID)

	rec := other.do(http.MethodPatch, fmt.Sprintf("/bookmarks/%d", mine.ID), map[string]any{"note": "changed"})
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = other.do(http.MethodDelete, fmt.Sprintf("/bookmarks/%d", mine.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	items := owner.list(f.audiobook.ID)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Note)
	assert.Equal(t, "mine", *items[0].Note, "another user's edit didn't land")
}

func TestBookmarks_GoWithTheirFile(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := newAPI(t, f.db, f.user("listener"))
	a.create(f.audiobook.ID, map[string]any{"seconds": 83})
	a.create(f.audiobook.ID, map[string]any{"seconds": 90})
	require.Equal(t, 2, f.count())

	require.NoError(t, books.NewService(f.db, nil).DeleteFile(context.Background(), f.audiobook.ID))
	assert.Equal(t, 0, f.count())

	rec := a.do(http.MethodGet, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestBookmarks_Rejections(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := newAPI(t, f.db, f.user("listener"))
	bookmark := a.create(f.audiobook.ID, map[string]any{"seconds": 83})

	elsewhere := &models.Library{Name: "Elsewhere", CoverAspectRatio: "book"}
	f.insert(elsewhere)
	restrictedUser := f.user("restricted", elsewhere.ID)
	restricted := newAPI(t, f.db, restrictedUser)
	// A Bookmark the restricted user made before losing the library.
	owned := newAPI(t, f.db, f.user("formerly", f.library.ID)).create(f.audiobook.ID, map[string]any{"seconds": 5})
	_, err := f.db.ExecContext(context.Background(), `UPDATE bookmarks SET user_id = ? WHERE id = ?`, restrictedUser.ID, owned.ID)
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		api    *api
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"an ebook", a, http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", f.epub.ID), map[string]any{"seconds": 1}, http.StatusUnprocessableEntity, "invalid_state"},
		{"listing an ebook", a, http.MethodGet, fmt.Sprintf("/files/%d/bookmarks", f.epub.ID), nil, http.StatusUnprocessableEntity, "invalid_state"},
		{"no time", a, http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), map[string]any{"note": "x"}, http.StatusUnprocessableEntity, "validation_error"},
		{"a negative time", a, http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), map[string]any{"seconds": -1}, http.StatusUnprocessableEntity, "validation_error"},
		{"a missing File", a, http.MethodPost, "/files/999999/bookmarks", map[string]any{"seconds": 1}, http.StatusNotFound, "not_found"},
		{"an unparseable File id", a, http.MethodGet, "/files/abc/bookmarks", nil, http.StatusNotFound, "not_found"},
		{"a missing Bookmark", a, http.MethodPatch, "/bookmarks/999999", map[string]any{"note": "x"}, http.StatusNotFound, "not_found"},
		{"an unknown field", a, http.MethodPatch, fmt.Sprintf("/bookmarks/%d", bookmark.ID), map[string]any{"seconds": 5}, http.StatusUnprocessableEntity, "unknown_parameter"},
		{"leaving the note out", a, http.MethodPatch, fmt.Sprintf("/bookmarks/%d", bookmark.ID), map[string]any{}, http.StatusUnprocessableEntity, "validation_error"},
		{"a null note", a, http.MethodPatch, fmt.Sprintf("/bookmarks/%d", bookmark.ID), map[string]any{"note": nil}, http.StatusUnprocessableEntity, "validation_error"},
		{"a note too long", a, http.MethodPatch, fmt.Sprintf("/bookmarks/%d", bookmark.ID), map[string]any{"note": strings.Repeat("é", 2001)}, http.StatusUnprocessableEntity, "validation_error"},
		{"a note too long on create", a, http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), map[string]any{"seconds": 1, "note": strings.Repeat("é", 2001)}, http.StatusUnprocessableEntity, "validation_error"},
		{"editing out of reach", restricted, http.MethodPatch, fmt.Sprintf("/bookmarks/%d", owned.ID), map[string]any{"note": "x"}, http.StatusForbidden, "forbidden"},
		{"deleting out of reach", restricted, http.MethodDelete, fmt.Sprintf("/bookmarks/%d", owned.ID), nil, http.StatusForbidden, "forbidden"},
		{"a library out of reach", restricted, http.MethodGet, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), nil, http.StatusForbidden, "forbidden"},
		{"creating out of reach", restricted, http.MethodPost, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), map[string]any{"seconds": 1}, http.StatusForbidden, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := tc.api.do(tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.code, errorCode(t, rec))
		})
	}
	assert.Equal(t, 2, f.count())
}

func TestBookmarks_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := newAPI(t, f.db, f.user("listener"))
	bookmark := a.create(f.audiobook.ID, map[string]any{"seconds": 83})
	_, err := f.db.ExecContext(context.Background(), `UPDATE bookmarks SET created_at = 'not a time' WHERE id = ?`, bookmark.ID)
	require.NoError(t, err)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID)},
		{http.MethodDelete, fmt.Sprintf("/bookmarks/%d", bookmark.ID)},
	} {
		rec := a.do(tc.method, tc.path, nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}

func TestBookmarks_FileLookupFailureIs500(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	a := newAPI(t, f.db, f.user("listener"))
	_, err := f.db.ExecContext(context.Background(), `DROP TABLE files`)
	require.NoError(t, err)

	for _, tc := range []struct {
		method string
		body   any
	}{
		{http.MethodGet, nil},
		{http.MethodPost, map[string]any{"seconds": 1}},
	} {
		rec := a.do(tc.method, fmt.Sprintf("/files/%d/bookmarks", f.audiobook.ID), tc.body)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "%s: %s", tc.method, rec.Body.String())
	}
}
