package books

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newTestEchoBooks(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	return e
}

func seedUserWithLibAccess(t *testing.T, db *bun.DB, username string, lib *models.Library) *models.User {
	t.Helper()
	u := &models.User{
		Username:     username,
		PasswordHash: "x",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(u).Exec(context.Background())
	require.NoError(t, err)

	access := &models.UserLibraryAccess{UserID: u.ID, LibraryID: &lib.ID}
	_, err = db.NewInsert().Model(access).Exec(context.Background())
	require.NoError(t, err)

	// Reload so u.LibraryAccess is populated for HasLibraryAccess /
	// GetAccessibleLibraryIDs checks performed inside the handler.
	reloaded := &models.User{}
	err = db.NewSelect().
		Model(reloaded).
		Relation("LibraryAccess").
		Where("u.id = ?", u.ID).
		Scan(context.Background())
	require.NoError(t, err)
	return reloaded
}

// TestListHandler_ExplicitSortWins verifies the sort query param is parsed
// and passed through, overriding any stored preference.
func TestListHandler_ExplicitSortWins(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Library A")
	user := seedUserWithLibAccess(t, db, "alice", lib)

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	// Stored preference is date_added:desc; should be overridden.
	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID)+"&sort=title:asc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []*models.Book `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	// title:asc → Apple before Cheese.
	assert.Equal(t, apple.ID, resp.Items[0].ID)
	assert.Equal(t, cheese.ID, resp.Items[1].ID)
}

// TestListHandler_StoredPreferenceUsed verifies that when no URL sort is
// provided, the stored preference drives ordering. Books are chosen so
// the builtin default (date_added:desc → banana, apple) disagrees with
// the stored title:asc (apple, banana) — this asymmetry is what makes
// the test actually prove the resolver fired rather than silently
// inheriting the builtin default.
func TestListHandler_StoredPreferenceUsed(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Library B")
	user := seedUserWithLibAccess(t, db, "bob", lib)

	now := time.Now()
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-2*time.Hour)) // older
	banana := seedBook(t, db, lib, "Banana", "Banana", now)                // newer

	settingsSvc := settings.NewService(db)
	stored := "title:asc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))

	var resp struct {
		Items []*models.Book `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	// title:asc → apple (alphabetical) before banana. If the resolver
	// hadn't fired, the builtin default (date_added:desc) would return
	// banana (newer) first and the assertion would fail.
	assert.Equal(t, apple.ID, resp.Items[0].ID)
	assert.Equal(t, banana.ID, resp.Items[1].ID)
}

// TestListHandler_InvalidSortReturns400 verifies sort validation.
func TestListHandler_InvalidSortReturns400(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Library C")
	user := seedUserWithLibAccess(t, db, "carol", lib)

	h := &handler{bookService: NewService(db), settingsService: settings.NewService(db)}

	e := newTestEchoBooks(t)
	req := httptest.NewRequest(
		http.MethodGet,
		"/books?library_id="+strconv.Itoa(lib.ID)+"&sort="+url.QueryEscape("bogus_field:asc"),
		nil,
	)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	err := h.list(c)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unknown sort field")
}

// TestListHandler_NoLibraryIDSkipsStoredLookup verifies the resolver only
// engages when scoped to a single library.
func TestListHandler_NoLibraryIDSkipsStoredLookup(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Library D")
	user := seedUserWithLibAccess(t, db, "dave", lib)

	now := time.Now()
	cheese := seedBook(t, db, lib, "Cheese", "Cheese", now)
	apple := seedBook(t, db, lib, "Apple", "Apple", now.Add(-time.Hour))

	// Stored preference title:asc would produce Apple first; the builtin
	// default (date_added:desc, applied by the books service when Sort is
	// nil) produces Cheese first. Picking stored-vs-default pairs that
	// order differently is what makes this test actually prove the
	// resolver was skipped — if both produced the same order we couldn't
	// tell whether the resolver ran.
	settingsSvc := settings.NewService(db)
	stored := "title:asc"
	_, err := settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	h := &handler{bookService: NewService(db), settingsService: settingsSvc}

	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books", nil) // no library_id
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))

	var resp struct {
		Items []*models.Book `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)
	// Without library_id the resolver is skipped, so the stored title:asc
	// preference is ignored and the builtin default (date_added:desc) wins
	// → Cheese (newer) before Apple (older).
	assert.Equal(t, cheese.ID, resp.Items[0].ID)
	assert.Equal(t, apple.ID, resp.Items[1].ID)
}

// TestListHandler_ResponseEnvelope asserts the list endpoint returns exactly
// the { items, total } envelope (the ListBooksResponse shape, ADR 0004), with
// no extra top-level keys.
func TestListHandler_ResponseEnvelope(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "EnvelopeLib")
	user := seedUserWithLibAccess(t, db, "grace", lib)

	now := time.Now()
	apple := seedBook(t, db, lib, "Apple", "Apple", now)

	h := &handler{bookService: NewService(db), settingsService: settings.NewService(db)}
	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Top-level envelope must be { items, total } only.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	_, hasItems := raw["items"]
	_, hasTotal := raw["total"]
	assert.True(t, hasItems, "list response must have 'items' key")
	assert.True(t, hasTotal, "list response must have 'total' key")
	assert.Len(t, raw, 2, "list response must have exactly 'items' and 'total' keys")

	var resp struct {
		Items []struct {
			ID int `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, apple.ID, resp.Items[0].ID)
	assert.Equal(t, 1, resp.Total)
}

func seedFile(t *testing.T, db *bun.DB, book *models.Book, fileType string, hasCover bool) *models.File {
	t.Helper()
	f := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      fileType,
		FileRole:      models.FileRoleMain,
		Filepath:      book.Filepath,
		FilesizeBytes: 1000,
	}
	if hasCover {
		cover := "test.cover.jpg"
		mime := "image/jpeg"
		f.CoverImageFilename = &cover
		f.CoverMimeType = &mime
	}
	_, err := db.NewInsert().Model(f).Exec(context.Background())
	require.NoError(t, err)
	return f
}

func TestListHandler_IncludesCoverCacheKey(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "CoverLib")
	user := seedUserWithLibAccess(t, db, "eve", lib)

	now := time.Now()
	book := seedBook(t, db, lib, "WithCover", "WithCover", now)
	file := seedFile(t, db, book, models.FileTypeEPUB, true)

	h := &handler{bookService: NewService(db), settingsService: settings.NewService(db)}
	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []struct {
			ID            int    `json:"id"`
			CoverCacheKey string `json:"cover_cache_key"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)

	// Reload the file to get its DB-assigned UpdatedAt.
	reloaded := &models.File{}
	err := db.NewSelect().Model(reloaded).Where("f.id = ?", file.ID).Scan(context.Background())
	require.NoError(t, err)

	expected := fmt.Sprintf("%d-%d", file.ID, reloaded.UpdatedAt.Unix())
	assert.Equal(t, expected, resp.Items[0].CoverCacheKey)
}

func TestListHandler_CoverCacheKeyEmptyWhenNoCover(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "NoCoverLib")
	user := seedUserWithLibAccess(t, db, "frank", lib)

	now := time.Now()
	book := seedBook(t, db, lib, "NoCover", "NoCover", now)
	seedFile(t, db, book, models.FileTypeEPUB, false)

	h := &handler{bookService: NewService(db), settingsService: settings.NewService(db)}
	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?library_id="+strconv.Itoa(lib.ID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", user)

	require.NoError(t, h.list(c))

	var resp struct {
		Items []struct {
			CoverCacheKey string `json:"cover_cache_key"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Empty(t, resp.Items[0].CoverCacheKey)
}
