package koreaderplugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

type pluginFixture struct {
	t       *testing.T
	ctx     context.Context
	db      *bun.DB
	e       *echo.Echo
	h       *handler
	devices *devices.Service
	user    *models.User
	device  *devices.Device
}

func newPluginFixture(t *testing.T) *pluginFixture {
	t.Helper()
	db := testdb.New(t)
	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	bundle, err := NewBundle(pluginFS("return 1\n"), "v0.7.0")
	require.NoError(t, err)
	h := newHandler(db, bundle)
	registerPluginRoutes(e, h, devices.NewMiddleware(h.devices))

	f := &pluginFixture{t: t, ctx: context.Background(), db: db, e: e, h: h, devices: h.devices}
	f.user = f.addUser("reader", true)
	f.device = f.addDevice(f.user, devices.DeviceTypeKOReader)
	return f
}

func (f *pluginFixture) addUser(username string, booksRead bool) *models.User {
	f.t.Helper()
	role := &models.Role{Name: username + "-role", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := f.db.NewInsert().Model(role).Exec(f.ctx)
	require.NoError(f.t, err)
	if booksRead {
		_, err = f.db.NewInsert().Model(&models.Permission{RoleID: role.ID, Resource: models.ResourceBooks, Operation: models.OperationRead}).Exec(f.ctx)
		require.NoError(f.t, err)
	}
	user := &models.User{Username: username, PasswordHash: "x", RoleID: role.ID, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = f.db.NewInsert().Model(user).Exec(f.ctx)
	require.NoError(f.t, err)
	return user
}

func (f *pluginFixture) addDevice(user *models.User, deviceType string) *devices.Device {
	f.t.Helper()
	device, err := f.devices.Create(f.ctx, user.ID, user.Username+" "+deviceType, deviceType)
	require.NoError(f.t, err)
	return device
}

func (f *pluginFixture) shortCode(user *models.User, device *devices.Device) string {
	f.t.Helper()
	short, err := f.devices.GenerateShortURL(f.ctx, user.ID, device.ID)
	require.NoError(f.t, err)
	return short.ShortCode
}

// request is a plugin request: its path, body, Basic credentials, and the
// protocol it claims to speak.
type request struct {
	path       string
	body       string
	username   string
	credential string
	protocol   string
	remoteAddr string
}

func (f *pluginFixture) do(r request) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, r.path, strings.NewReader(r.body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(HeaderPluginVersion, "0.7.0")
	protocol := r.protocol
	if protocol == "" {
		protocol = strconv.Itoa(Protocol)
	}
	if protocol != "none" {
		req.Header.Set(HeaderPluginProtocol, protocol)
	}
	if r.username != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(r.username+":"+r.credential)))
	}
	if r.remoteAddr != "" {
		req.RemoteAddr = r.remoteAddr
	}
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, req)
	return rec
}

func (f *pluginFixture) pair(code string) *httptest.ResponseRecorder {
	f.t.Helper()
	return f.do(request{path: "/koreader/plugin/pair", body: `{"code":"` + code + `"}`})
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp errcodes.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), rec.Body.String())
	return resp.Error.Code
}

func TestPair_ExchangesAShortCodeForTheDevicesSignIn(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec := f.pair(f.shortCode(f.user, f.device))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp SignInResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "reader", resp.Username)
	assert.Equal(t, f.device.Credential, resp.Credential)
	assert.Equal(t, f.device.ID, resp.DeviceID)
	assert.Equal(t, f.device.Name, resp.DeviceName)
	assert.Equal(t, ServerInfo{ProtocolMin: ProtocolMin, ProtocolMax: ProtocolMax, PluginVersion: "0.7.0", PluginHash: f.h.bundle.Hash, PluginProtocol: Protocol}, resp.Server)
}

func TestPair_ACodeWorksForItsWholeLifetime(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	code := f.shortCode(f.user, f.device)

	// Nearly expired, and used once already.
	_, err := f.db.NewUpdate().Model((*devices.DeviceShortURL)(nil)).
		Set("expires_at = ?", time.Now().Add(time.Minute)).Where("short_code = ?", code).Exec(f.ctx)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, f.pair(code).Code)

	assert.Equal(t, http.StatusOK, f.pair(code).Code)
	assert.Equal(t, http.StatusOK, f.pair(strings.ToUpper(code)).Code, "a code typed in capitals")
}

func TestPair_AnExpiredCodeFails(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	code := f.shortCode(f.user, f.device)
	_, err := f.db.NewUpdate().Model((*devices.DeviceShortURL)(nil)).
		Set("expires_at = ?", time.Now().Add(-time.Second)).Where("short_code = ?", code).Exec(f.ctx)
	require.NoError(t, err)

	rec := f.pair(code)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", errorCode(t, rec))
	assert.NotContains(t, rec.Body.String(), f.device.Credential)
}

func TestPair_AnUnknownCodeFails(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	assert.Equal(t, http.StatusNotFound, f.pair("zzzzzz").Code)
}

func TestPair_NeverReturnsAnotherUsersDevice(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	other := f.addUser("other", true)
	otherDevice := f.addDevice(other, devices.DeviceTypeKOReader)
	mine := f.shortCode(f.user, f.device)
	theirs := f.shortCode(other, otherDevice)

	var resp SignInResponse
	rec := f.pair(mine)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "reader", resp.Username)
	assert.Equal(t, f.device.Credential, resp.Credential)

	rec = f.pair(theirs)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "other", resp.Username)
	assert.Equal(t, otherDevice.Credential, resp.Credential)
}

func TestPair_RefusesADeviceThatIsNotKOReader(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	kobo := f.addDevice(f.user, devices.DeviceTypeKobo)

	rec := f.pair(f.shortCode(f.user, kobo))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), kobo.Credential)
}

func TestPair_RefusesAnOwnerWhoCannotReadBooks(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	noBooks := f.addUser("nobooks", false)
	device := f.addDevice(noBooks, devices.DeviceTypeKOReader)

	rec := f.pair(f.shortCode(noBooks, device))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.NotContains(t, rec.Body.String(), device.Credential)
}

func TestPair_IsRateLimitedByAddress(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	code := f.shortCode(f.user, f.device)

	for range pairBurst {
		rec := f.do(request{path: "/koreader/plugin/pair", body: `{"code":"wrong1"}`, remoteAddr: "203.0.113.5:1234"})
		require.Equal(t, http.StatusNotFound, rec.Code)
	}
	// Even the right code waits once the guesses run out.
	rec := f.do(request{path: "/koreader/plugin/pair", body: `{"code":"` + code + `"}`, remoteAddr: "203.0.113.5:1234"})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "too_many_requests", errorCode(t, rec))

	// Another address is unaffected.
	rec = f.do(request{path: "/koreader/plugin/pair", body: `{"code":"` + code + `"}`, remoteAddr: "203.0.113.6:1234"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPluginRoutes_RefuseAProtocolTheServerNoLongerAccepts(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	code := f.shortCode(f.user, f.device)

	for _, protocol := range []string{strconv.Itoa(ProtocolMin - 1), strconv.Itoa(ProtocolMax + 1), "none", "abc"} {
		rec := f.do(request{path: "/koreader/plugin/pair", body: `{"code":"` + code + `"}`, protocol: protocol})
		assert.Equal(t, http.StatusUpgradeRequired, rec.Code, protocol)
		assert.Equal(t, "plugin_update_required", errorCode(t, rec), protocol)

		rec = f.do(request{path: "/koreader/plugin/status", username: "reader", credential: f.device.Credential, protocol: protocol})
		assert.Equal(t, http.StatusUpgradeRequired, rec.Code, protocol)
	}
}

func TestStatus_SignsInWithUsernameAndDevicePassword(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec := f.do(request{path: "/koreader/plugin/status", username: "READER", credential: f.device.Credential})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp StatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, f.device.ID, resp.DeviceID)
	assert.Equal(t, f.device.Name, resp.DeviceName)
	assert.Equal(t, "0.7.0", resp.Server.PluginVersion)
	assert.Equal(t, f.h.bundle.Hash, resp.Server.PluginHash)
}

func TestStatus_RecordsThePluginConnection(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	before := time.Now().Add(-time.Second)

	rec := f.do(request{path: "/koreader/plugin/status", username: "reader", credential: f.device.Credential})
	require.Equal(t, http.StatusOK, rec.Code)

	device, err := f.devices.Retrieve(f.ctx, f.user.ID, f.device.ID)
	require.NoError(t, err)
	require.NotNil(t, device.PluginLastConnectedAt)
	assert.True(t, device.PluginLastConnectedAt.After(before))
	require.NotNil(t, device.PluginVersion)
	assert.Equal(t, "0.7.0", *device.PluginVersion)
}

func TestStatus_RefusesAWrongSignIn(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	other := f.addUser("other", true)
	otherDevice := f.addDevice(other, devices.DeviceTypeKOReader)
	kobo := f.addDevice(f.user, devices.DeviceTypeKobo)

	for name, r := range map[string]request{
		"no sign-in":               {},
		"wrong password":           {username: "reader", credential: "wrong"},
		"another user's Device":    {username: "reader", credential: otherDevice.Credential},
		"the account password":     {username: "reader", credential: "x"},
		"a Device of another type": {username: "reader", credential: kobo.Credential},
	} {
		r.path = "/koreader/plugin/status"
		rec := f.do(r)
		want := http.StatusUnauthorized
		if name == "a Device of another type" {
			want = http.StatusForbidden
		}
		assert.Equal(t, want, rec.Code, name)
	}
}

func TestDownload_ServesTheUnpairedZipToAnyProtocol(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	// An old plugin must be able to fetch the update that replaces it.
	rec := f.do(request{path: "/koreader/plugin/download", username: "reader", credential: f.device.Credential, protocol: "none"})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "application/zip", rec.Header().Get(echo.HeaderContentType))
	files := unzip(t, rec.Body.Bytes())
	assert.Contains(t, files["shisho.koplugin/build.lua"], f.h.bundle.Hash)
	assert.NotContains(t, files, "shisho.koplugin/pairing.lua")
}

func TestDownload_NeedsASignIn(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec := f.do(request{path: "/koreader/plugin/download"})

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ownerDownload downloads the plugin from Device id's page as user.
func (f *pluginFixture) ownerDownload(user *models.User, id string, headers ...string) (*httptest.ResponseRecorder, error) {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/user/devices/"+id+"/koreader-plugin", nil)
	req.Host = "books.example.com:8080"
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	c := f.e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	auth.SetUser(c, user)
	return rec, f.h.ownerDownload(c)
}

func TestOwnerDownload_CarriesTheServerAndAFreshShortCode(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec, err := f.ownerDownload(f.user, f.device.ID)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/zip", rec.Header().Get(echo.HeaderContentType))
	assert.Contains(t, rec.Header().Get(echo.HeaderContentDisposition), "shisho.koplugin.zip")
	pairing := unzip(t, rec.Body.Bytes())["shisho.koplugin/pairing.lua"]
	assert.Contains(t, pairing, `server_url = "http://books.example.com:8080"`)

	var short devices.DeviceShortURL
	require.NoError(t, f.db.NewSelect().Model(&short).Where("device_id = ?", f.device.ID).Scan(f.ctx))
	assert.Contains(t, pairing, `short_code = "`+short.ShortCode+`"`)
	assert.Contains(t, pairing, "expires_at = "+strconv.FormatInt(short.ExpiresAt.Unix(), 10))

	// The code in the zip signs in.
	assert.Equal(t, http.StatusOK, f.pair(short.ShortCode).Code)
}

func TestOwnerDownload_RefusesAnotherUsersDevice(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	other := f.addUser("other", true)

	_, err := f.ownerDownload(other, f.device.ID)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusNotFound, codeErr.HTTPCode)
}

func TestOwnerDownload_RefusesADeviceThatIsNotKOReader(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	kobo := f.addDevice(f.user, devices.DeviceTypeKobo)

	_, err := f.ownerDownload(f.user, kobo.ID)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	assert.Equal(t, "invalid_state", codeErr.Code)
}

func TestPair_RateLimitIgnoresAddressesAClientClaims(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	for i := range pairBurst + 1 {
		req := httptest.NewRequest(http.MethodPost, "/koreader/plugin/pair", strings.NewReader(`{"code":"wrong1"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(HeaderPluginProtocol, strconv.Itoa(Protocol))
		req.Header.Set("X-Real-IP", "198.51.100."+strconv.Itoa(i))
		req.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i))
		req.RemoteAddr = "203.0.113.9:1234"
		rec := httptest.NewRecorder()
		f.e.ServeHTTP(rec, req)
		if i == pairBurst {
			assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		}
	}
}

func TestClientAddress(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		remote    string
		forwarded []string
		want      string
	}{
		"a public peer":                        {"203.0.113.9:1", nil, "203.0.113.9"},
		"a public peer claiming another":       {"203.0.113.9:1", []string{"10.0.0.1"}, "203.0.113.9"},
		"behind a proxy":                       {"10.0.0.2:1", []string{"198.51.100.7"}, "198.51.100.7"},
		"behind a proxy, with a claimed entry": {"10.0.0.2:1", []string{"1.2.3.4, 198.51.100.7"}, "198.51.100.7"},
		"a private peer with no proxy header":  {"192.168.1.5:1", nil, "192.168.1.5"},
		"an IPv6 peer counts by its /64":       {"[2001:db8:1:2:3:4:5:6]:1", nil, "2001:db8:1:2::/64"},
		"a proxy's IPv6 entry":                 {"10.0.0.2:1", []string{"2001:db8:1:2::9"}, "2001:db8:1:2::/64"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = tc.remote
		for _, v := range tc.forwarded {
			req.Header.Add("X-Forwarded-For", v)
		}
		assert.Equal(t, tc.want, clientAddress(req), name)
	}
}

func TestPair_ALookupFailureIsAServerError(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	code := f.shortCode(f.user, f.device)
	_, err := f.db.ExecContext(f.ctx, `DROP TABLE device_short_urls`)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, f.pair(code).Code)
}

func TestOwnerDownload_ALookupFailureIsAServerError(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	_, err := f.db.ExecContext(f.ctx, `UPDATE devices SET created_at = 'not a time' WHERE id = ?`, f.device.ID)
	require.NoError(t, err)

	_, err = f.ownerDownload(f.user, f.device.ID)

	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "want a plain error")
}

func TestOwnerDownload_CarriesTheURLThroughAProxy(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec, err := f.ownerDownload(f.user, f.device.ID,
		"X-Forwarded-Proto", "https", "X-Forwarded-Host", "library.example.org", "X-Forwarded-Prefix", "/shisho")
	require.NoError(t, err)

	pairing := unzip(t, rec.Body.Bytes())["shisho.koplugin/pairing.lua"]
	assert.Contains(t, pairing, `server_url = "https://library.example.org/shisho"`)
}

func TestStatus_ASignInLookupFailureIsAServerError(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)
	_, err := f.db.ExecContext(f.ctx, `UPDATE devices SET created_at = 'not a time' WHERE id = ?`, f.device.ID)
	require.NoError(t, err)

	rec := f.do(request{path: "/koreader/plugin/status", username: "reader", credential: f.device.Credential})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestStatus_TellsARefusedPluginWhatToUpdateTo(t *testing.T) {
	t.Parallel()
	f := newPluginFixture(t)

	rec := f.do(request{path: "/koreader/plugin/status", username: "reader", credential: f.device.Credential, protocol: strconv.Itoa(ProtocolMin - 1)})

	require.Equal(t, http.StatusUpgradeRequired, rec.Code)
	assert.Equal(t, "0.7.0", rec.Header().Get(HeaderServerPluginVersion))
	assert.Equal(t, f.h.bundle.Hash, rec.Header().Get(HeaderServerPluginHash))
	assert.Equal(t, strconv.Itoa(Protocol), rec.Header().Get(HeaderServerPluginProtocol))
	assert.Equal(t, strconv.Itoa(ProtocolMin), rec.Header().Get(HeaderServerProtocolMin))
	assert.Equal(t, strconv.Itoa(ProtocolMax), rec.Header().Get(HeaderServerProtocolMax))

	// Only to a signed-in Device.
	rec = f.do(request{path: "/koreader/plugin/status", username: "reader", credential: "wrong", protocol: strconv.Itoa(ProtocolMin - 1)})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Header().Get(HeaderServerPluginVersion))
	rec = f.do(request{path: "/koreader/plugin/pair", body: `{"code":"abc123"}`, protocol: strconv.Itoa(ProtocolMin - 1)})
	assert.Equal(t, http.StatusUpgradeRequired, rec.Code)
	assert.Empty(t, rec.Header().Get(HeaderServerPluginVersion))
}

func TestAddressLimiter_SharesOneBucketPastItsCap(t *testing.T) {
	t.Parallel()
	l := newAddressLimiter()
	for i := range maxBuckets {
		require.True(t, l.allow("address-"+strconv.Itoa(i)))
	}

	for range pairBurst {
		require.True(t, l.allow("newcomer"))
	}
	assert.False(t, l.allow("another newcomer"), "newcomers share one bucket")
	assert.True(t, l.allow("address-1"), "addresses already tracked keep theirs")
	assert.LessOrEqual(t, len(l.buckets), maxBuckets+1)
}
