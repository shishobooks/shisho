package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
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

// buildGetRequest builds an Echo context for GET /settings/libraries/:library_id.
func buildGetRequest(t *testing.T, e *echo.Echo, user *models.User, libraryID int) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/settings/libraries/"+strconv.Itoa(libraryID), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("library_id")
	c.SetParamValues(strconv.Itoa(libraryID))
	c.Set("user", user)
	return c, rec
}

func buildPutRequest(t *testing.T, e *echo.Echo, user *models.User, libraryID int, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/settings/libraries/"+strconv.Itoa(libraryID), strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("library_id")
	c.SetParamValues(strconv.Itoa(libraryID))
	c.Set("user", user)
	return c, rec
}

// seedLibraryAccess writes a user_library_access row AND mutates the
// in-memory user so HasLibraryAccess sees the grant. HasLibraryAccess
// iterates user.LibraryAccess (the relation slice), so DB-only seeding
// isn't enough for the in-memory user used in these handler tests.
func seedLibraryAccess(t *testing.T, db *bun.DB, user *models.User, libraryID int) {
	t.Helper()
	libID := libraryID
	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: &libID,
	}
	_, err := db.NewInsert().Model(access).Exec(context.Background())
	require.NoError(t, err)
	user.LibraryAccess = append(user.LibraryAccess, access)
}

func TestGetLibrarySettings_NoRow(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho(t)
	c, rec := buildGetRequest(t, e, user, lib.ID)

	require.NoError(t, h.getLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var body LibrarySettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Nil(t, body.SortSpec)
}

func TestGetLibrarySettings_Forbidden(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books") // intentionally no seedLibraryAccess

	e := newTestEcho(t)
	c, _ := buildGetRequest(t, e, user, lib.ID)

	err := h.getLibrarySettings(c)
	require.Error(t, err)

	// errcodes.Forbidden produces an *errcodes.Error with Code "forbidden"
	// and Message "<action> is not allowed." — the literal string "Forbidden"
	// does not appear in err.Error(), so we assert against the Code field.
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "forbidden", codeErr.Code)
	assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
}

func TestUpdateLibrarySettings_PersistsSpec(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho(t)
	c, rec := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":"title:asc"}`)

	require.NoError(t, h.updateLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	stored, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, stored.SortSpec)
	assert.Equal(t, "title:asc", *stored.SortSpec)
}

func TestUpdateLibrarySettings_RejectsInvalidSpec(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	e := newTestEcho(t)
	c, _ := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":"bogus_field:asc"}`)

	err := h.updateLibrarySettings(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown sort field")
}

func TestUpdateLibrarySettings_AcceptsNullClear(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	h := &libraryHandler{settingsService: svc}

	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")
	seedLibraryAccess(t, db, user, lib.ID)

	// Seed a spec first.
	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	e := newTestEcho(t)
	c, rec := buildPutRequest(t, e, user, lib.ID, `{"sort_spec":null}`)

	require.NoError(t, h.updateLibrarySettings(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	stored, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Nil(t, stored.SortSpec)
}
