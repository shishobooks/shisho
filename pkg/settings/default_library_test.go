package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func grantLibraryAccess(user *models.User, libraryIDs ...*int) {
	for _, id := range libraryIDs {
		user.LibraryAccess = append(user.LibraryAccess, &models.UserLibraryAccess{UserID: user.ID, LibraryID: id})
	}
}

func putUserSettings(t *testing.T, db *bun.DB, user *models.User, body string) (UserSettingsResponse, error) {
	t.Helper()
	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodPut, "/settings/user", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	var resp UserSettingsResponse
	if err := h.updateUserSettings(c); err != nil {
		return resp, err
	}
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp, nil
}

func getUserSettings(t *testing.T, db *bun.DB, user *models.User) UserSettingsResponse {
	t.Helper()
	e := newTestEcho(t)
	h := &handler{settingsService: NewService(db)}

	req := httptest.NewRequest(http.MethodGet, "/settings/user", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)

	require.NoError(t, h.getUserSettings(c))
	var resp UserSettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestGetUserSettings_DefaultsToNoDefaultLibrary(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	grantLibraryAccess(user, nil)

	resp := getUserSettings(t, db, user)
	assert.Nil(t, resp.DefaultLibraryID)
}

func TestUpdateUserSettings_SetsAndClearsDefaultLibrary(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Comics")
	grantLibraryAccess(user, &lib.ID)

	resp, err := putUserSettings(t, db, user, `{"default_library_id": `+jsonInt(lib.ID)+`}`)
	require.NoError(t, err)
	require.NotNil(t, resp.DefaultLibraryID)
	assert.Equal(t, lib.ID, *resp.DefaultLibraryID)

	// Omitting the field leaves it alone.
	resp, err = putUserSettings(t, db, user, `{"gallery_size": "l"}`)
	require.NoError(t, err)
	require.NotNil(t, resp.DefaultLibraryID)
	assert.Equal(t, lib.ID, *resp.DefaultLibraryID)

	// Sending null clears it.
	resp, err = putUserSettings(t, db, user, `{"default_library_id": null}`)
	require.NoError(t, err)
	assert.Nil(t, resp.DefaultLibraryID)
	assert.Nil(t, getUserSettings(t, db, user).DefaultLibraryID)
}

func TestUpdateUserSettings_RejectsDefaultLibraryWithoutAccess(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	allowed := createTestLibrary(t, db, "Allowed")
	forbidden := createTestLibrary(t, db, "Forbidden")
	grantLibraryAccess(user, &allowed.ID)

	_, err := putUserSettings(t, db, user, `{"default_library_id": `+jsonInt(forbidden.ID)+`}`)
	require.Error(t, err)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
}

func TestUpdateUserSettings_RejectsMissingDefaultLibrary(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	grantLibraryAccess(user, nil)

	_, err := putUserSettings(t, db, user, `{"default_library_id": 9999}`)
	require.Error(t, err)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
}

// A failed library lookup must surface as a 500, not the 404 for a missing
// library.
func TestUpdateUserSettings_DefaultLibraryLookupFailureIsServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	grantLibraryAccess(user, nil)

	_, err := db.ExecContext(context.Background(), "DROP TABLE libraries")
	require.NoError(t, err)

	_, err = putUserSettings(t, db, user, `{"default_library_id": 1}`)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr)
}

func TestGetUserSettings_HidesDefaultLibraryAfterAccessIsRevoked(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Comics")
	grantLibraryAccess(user, &lib.ID)

	_, err := putUserSettings(t, db, user, `{"default_library_id": `+jsonInt(lib.ID)+`}`)
	require.NoError(t, err)

	user.LibraryAccess = nil
	other := createTestLibrary(t, db, "Other")
	grantLibraryAccess(user, &other.ID)
	assert.Nil(t, getUserSettings(t, db, user).DefaultLibraryID)
}

func TestDeletingLibrary_ClearsDefaultLibrary(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Comics")

	_, err := svc.UpdateUserSettings(ctx, user.ID, UserSettingsUpdate{
		DefaultLibraryID: httputil.NullableInt{Value: &lib.ID, Set: true},
	})
	require.NoError(t, err)

	_, err = db.NewDelete().Model((*models.Library)(nil)).Where("id = ?", lib.ID).Exec(ctx)
	require.NoError(t, err)

	settings, err := svc.GetUserSettings(ctx, user.ID)
	require.NoError(t, err)
	assert.Nil(t, settings.DefaultLibraryID)
}

func jsonInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
