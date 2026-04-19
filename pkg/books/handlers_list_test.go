package books

import (
	"context"
	"encoding/json"
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
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Books, 2)
	// title:asc → Apple before Cheese.
	assert.Equal(t, apple.ID, resp.Books[0].ID)
	assert.Equal(t, cheese.ID, resp.Books[1].ID)
}

// TestListHandler_StoredPreferenceUsed verifies that when no URL sort is
// provided, the stored preference drives ordering.
func TestListHandler_StoredPreferenceUsed(t *testing.T) {
	t.Parallel()

	db := setupBooksTestDB(t)
	lib := seedLibrary(t, db, "Library B")
	user := seedUserWithLibAccess(t, db, "bob", lib)

	now := time.Now()
	oldest := seedBook(t, db, lib, "Oldest", "Oldest", now.Add(-2*time.Hour))
	newest := seedBook(t, db, lib, "Newest", "Newest", now)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
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
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Books, 2)
	assert.Equal(t, newest.ID, resp.Books[0].ID)
	assert.Equal(t, oldest.ID, resp.Books[1].ID)
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

	// Stored preference would produce date_added:desc → cheese first,
	// but without library_id the resolver is skipped and the default
	// sort_title ASC → apple first applies.
	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
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
		Books []*models.Book `json:"books"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Books, 2)
	assert.Equal(t, apple.ID, resp.Books[0].ID) // alphabetical default
	assert.Equal(t, cheese.ID, resp.Books[1].ID)
}
