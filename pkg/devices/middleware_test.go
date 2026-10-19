package devices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func insertTestUser(t *testing.T, db *bun.DB, id int, username string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO users (id, username, password_hash, role_id) VALUES (?, ?, 'hash', 1)`, id, username)
	require.NoError(t, err)
}

// runDeviceAuth runs DeviceAuth(connection) for a request carrying
// credential, and returns the Device the handler saw, or the error.
func runDeviceAuth(t *testing.T, service *Service, connection, credential string) (*Device, error) {
	t.Helper()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.SetParamNames("credential")
	c.SetParamValues(credential)
	var seen *Device
	err := NewMiddleware(service).DeviceAuth(connection)(func(c echo.Context) error {
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

func requireStatus(t *testing.T, err error, status int) *errcodes.Error {
	t.Helper()
	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, status, ec.HTTPCode)
	return ec
}

// A credential is accepted only exactly as shown: the same letters in the
// same case, with nothing added.
func TestDeviceAuth_AcceptsOnlyTheExactCredential(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	device, err := service.Create(context.Background(), 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)

	seen, err := runDeviceAuth(t, service, ConnectionKoboSync, device.Credential)
	require.NoError(t, err)
	assert.Equal(t, device.ID, seen.ID)

	lookalike := map[byte]string{'a': "4", 'e': "3", 'g': "9", 'b': "6", 's': "5", 'z': "2", 'j': "i", 'k': "l"}
	var lookalikeCredential string
	for i := range len(device.Credential) {
		if replacement, ok := lookalike[device.Credential[i]]; ok {
			lookalikeCredential = device.Credential[:i] + replacement + device.Credential[i+1:]
			break
		}
	}
	if lookalikeCredential == "" {
		lookalikeCredential = "o" + device.Credential[1:]
	}

	for name, credential := range map[string]string{
		"hyphen added":       device.Credential[:4] + "-" + device.Credential[4:],
		"grouped in fours":   device.Credential[:4] + "-" + device.Credential[4:8] + "-" + device.Credential[8:12] + "-" + device.Credential[12:],
		"uppercase":          strings.ToUpper(device.Credential),
		"space added":        device.Credential + " ",
		"lookalike swapped":  lookalikeCredential,
		"last letter missed": device.Credential[:15],
	} {
		_, err := runDeviceAuth(t, service, ConnectionKoboSync, credential)
		ec := requireStatus(t, err, http.StatusUnauthorized)
		assert.Equal(t, "Invalid device credential", ec.Message, name)
	}
}

// A migrated Device keeps its API key as its credential, so its saved URLs
// keep working.
func TestDeviceAuth_AcceptsAMigratedKey(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at)
		VALUES ('legacy', 1, 'Old key', 'browser', 'ak_LegacyKey-With_Symbols=', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `INSERT INTO device_connections (device_id, connection) VALUES ('legacy', 'ereader_browser')`)
	require.NoError(t, err)

	seen, err := runDeviceAuth(t, service, ConnectionEReaderBrowser, "ak_LegacyKey-With_Symbols=")
	require.NoError(t, err)
	assert.Equal(t, "legacy", seen.ID)

	_, err = runDeviceAuth(t, service, ConnectionEReaderBrowser, "ak_legacykey-with_symbols=")
	requireStatus(t, err, http.StatusUnauthorized)
}

// Each connection words the 403 for a Device without it.
func TestDeviceAuth_ConnectionDeniedMessage(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	device, err := service.Create(context.Background(), 1, "Kindle", DeviceTypeKindle)
	require.NoError(t, err)
	_, err = service.Update(context.Background(), 1, device.ID, UpdateDeviceOptions{SetConnections: true})
	require.NoError(t, err)

	for connection, message := range connectionDeniedMessages {
		_, err := runDeviceAuth(t, service, connection, device.Credential)
		ec := requireStatus(t, err, http.StatusForbidden)
		assert.Equal(t, message, ec.Message)
	}
}

func TestDeviceAuth_MissingCredential_Returns401(t *testing.T) {
	t.Parallel()
	_, err := runDeviceAuth(t, NewService(testdb.New(t)), ConnectionKoboSync, "")
	requireStatus(t, err, http.StatusUnauthorized)
}

func TestRequireDevice_NoDevice_Returns401(t *testing.T) {
	t.Parallel()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	device, err := RequireDevice(c)
	assert.Nil(t, device)
	requireStatus(t, err, http.StatusUnauthorized)
}
