package devices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAppDevice_RecordsTheInstallation(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")

	device, err := service.CreateAppDevice(context.Background(), 1, AppDeviceOptions{
		Name: "iPhone", InstallationID: "install-1", AppVersion: "1.0.0 (3)",
	})
	require.NoError(t, err)

	stored, err := service.Retrieve(context.Background(), 1, device.ID)
	require.NoError(t, err)
	assert.Equal(t, DeviceTypeShishoApp, stored.DeviceType)
	assert.Equal(t, "iPhone", stored.Name)
	assert.Equal(t, ScopeAll, stored.ScopeType)
	require.NotNil(t, stored.InstallationID)
	assert.Equal(t, "install-1", *stored.InstallationID)
	require.NotNil(t, stored.AppVersion)
	assert.Equal(t, "1.0.0 (3)", *stored.AppVersion)
	assert.Len(t, stored.Credential, appCredentialLength)
	assert.Equal(t, []string{ConnectionShishoApp}, stored.ConnectionNames())
}

func TestRecordAppVersion_UpdatesOnlyAChangedVersion(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	device, err := service.CreateAppDevice(context.Background(), 1, AppDeviceOptions{Name: "iPhone", InstallationID: "i", AppVersion: "1.0.0"})
	require.NoError(t, err)

	require.NoError(t, service.RecordAppVersion(context.Background(), device, "1.1.0"))
	stored, err := service.Retrieve(context.Background(), 1, device.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", *stored.AppVersion)
	// An empty version, as a request without the header sends, keeps it.
	require.NoError(t, service.RecordAppVersion(context.Background(), stored, ""))
	stored, err = service.Retrieve(context.Background(), 1, device.ID)
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", *stored.AppVersion)
}

// An app Device's type, scope, Delivery Rules, and credential belong to the
// installation: the app can't learn a new credential, and ADR 0022 gives it
// every Library and no Delivery Rules.
func TestHandler_AppDevice_RefusesChangesTheAppOwns(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	device, err := f.h.service.CreateAppDevice(context.Background(), 1, AppDeviceOptions{Name: "iPhone", InstallationID: "i", AppVersion: "1.0.0"})
	require.NoError(t, err)
	user := libraryUser(1)

	for _, body := range []string{
		`{"device_type":"kobo"}`,
		`{"scope":{"type":"all"}}`,
		`{"delivery_rules":{"mode":"default"}}`,
	} {
		_, err := f.call(f.h.Update, user, body, device.ID)
		requireStatus(t, err, http.StatusUnprocessableEntity)
	}

	_, err = f.call(f.h.RegenerateCredential, user, "", device.ID)
	requireStatus(t, err, http.StatusUnprocessableEntity)

	// Renaming is the owner's to do.
	rec, err := f.call(f.h.Update, user, `{"name":"Work phone"}`, device.ID)
	require.NoError(t, err)
	var resp DeviceResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Work phone", resp.Name)
	// The app holds its credential; the web never shows it.
	assert.Empty(t, resp.Credential)
}

func TestHandler_Update_CantTurnAnotherDeviceIntoAnAppDevice(t *testing.T) {
	t.Parallel()
	f := newHandlerFixture(t)
	insertTestUser(t, f.db, 1, "reader")
	device, err := f.h.service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"device_type":"shisho_app"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := f.e.NewContext(req, httptest.NewRecorder())
	auth.SetUser(c, libraryUser(1))
	c.SetParamNames("id")
	c.SetParamValues(device.ID)
	err = f.h.Update(c)
	require.Error(t, err)
}

// runBearerAuth runs BearerAuth for the app connection, for a request with the
// Authorization header, and returns the Device the handler saw.
func runBearerAuth(t *testing.T, service *Service, authorization string) (*Device, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authorization != "" {
		req.Header.Set(echo.HeaderAuthorization, authorization)
	}
	c := echo.New().NewContext(req, httptest.NewRecorder())
	var seen *Device
	err := NewMiddleware(service, nil).BearerAuth(ConnectionShishoApp)(func(c echo.Context) error {
		device, err := RequireDevice(c)
		require.NoError(t, err)
		owner, err := auth.RequireUser(c)
		require.NoError(t, err)
		assert.Equal(t, device.UserID, owner.ID)
		seen = device
		return nil
	})(c)
	return seen, err
}

func TestBearerAuth(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	app, err := service.CreateAppDevice(context.Background(), 1, AppDeviceOptions{Name: "iPhone", InstallationID: "i", AppVersion: "1.0.0"})
	require.NoError(t, err)
	kobo, err := service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)

	seen, err := runBearerAuth(t, service, "Bearer "+app.Credential)
	require.NoError(t, err)
	assert.Equal(t, app.ID, seen.ID)

	_, err = runBearerAuth(t, service, "")
	requireStatus(t, err, http.StatusUnauthorized)
	_, err = runBearerAuth(t, service, "Bearer nope")
	requireStatus(t, err, http.StatusUnauthorized)
	_, err = runBearerAuth(t, service, "Basic "+app.Credential)
	requireStatus(t, err, http.StatusUnauthorized)
	// Another type's credential can't use the app's connection.
	_, err = runBearerAuth(t, service, "Bearer "+kobo.Credential)
	requireStatus(t, err, http.StatusForbidden)

	// Revoking ends it.
	require.NoError(t, service.Revoke(context.Background(), app.ID))
	_, err = runBearerAuth(t, service, "Bearer "+app.Credential)
	requireStatus(t, err, http.StatusUnauthorized)
}

func TestBearerAuth_DeactivatedOwnerIsRefused(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	app, err := service.CreateAppDevice(context.Background(), 1, AppDeviceOptions{Name: "iPhone", InstallationID: "i", AppVersion: "1.0.0"})
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `UPDATE users SET is_active = FALSE WHERE id = 1`)
	require.NoError(t, err)
	_, err = runBearerAuth(t, service, "Bearer "+app.Credential)
	requireStatus(t, err, http.StatusUnauthorized)
}
