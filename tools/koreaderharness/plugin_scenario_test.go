//go:build koreaderharness

package main

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	pluginfiles "github.com/shishobooks/shisho/koreader-plugin"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin's Lua unit tests, with KOReader's own luajit and modules.
func TestPluginLuaUnitTests(t *testing.T) {
	h := &Harness{Dir: t.TempDir(), ReleaseDir: filepath.Join("..", "..", "tmp", "koreader", "release")}
	result, err := h.RunLuaTests(context.Background(), filepath.Join("..", "..", "koreader-plugin"))
	require.NoError(t, err)
	for _, failure := range result.Failed {
		t.Errorf("%s\n%s", failure.Name, failure.Error)
	}
	assert.True(t, result.OK, "%s", result.Raw)
	assert.Positive(t, result.Passed)
}

// containerServer is the URL the container reaches the scenario's server
// at, as a Device page download stamps it.
func (s *scenario) containerServer() string {
	return containerURL(s.url)
}

// devicePageZip downloads the plugin from the Device's page as its owner,
// as the browser does, reaching the server at the URL the container can.
func (s *scenario) devicePageZip() []byte {
	s.t.Helper()
	// Signing in to the web needs a longer password than the scenario's.
	hash, err := auth.HashPassword("web-password")
	require.NoError(s.t, err)
	_, err = s.db.NewUpdate().Model(s.user).Set("password_hash = ?", hash).WherePK().Exec(s.ctx)
	require.NoError(s.t, err)
	client := &http.Client{}
	login, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.url+"/api/auth/login", strings.NewReader(`{"username":"reader","password":"web-password"}`))
	require.NoError(s.t, err)
	login.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(login)
	require.NoError(s.t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	require.Equal(s.t, http.StatusOK, resp.StatusCode)
	cookies := resp.Cookies()

	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.url+"/api/user/devices/"+s.device.ID+"/koreader-plugin", nil)
	require.NoError(s.t, err)
	u, err := url.Parse(s.containerServer())
	require.NoError(s.t, err)
	req.Host = u.Host
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = client.Do(req)
	require.NoError(s.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(s.t, err)
	require.Equal(s.t, http.StatusOK, resp.StatusCode, string(data))
	return data
}

func (s *scenario) installPlugin(zip []byte) {
	s.t.Helper()
	require.NoError(s.t, s.h.InstallPlugin(zip))
}

// pluginFile is the path of a file in the installed plugin.
func (s *scenario) pluginFile(name string) string {
	return filepath.Join(s.h.pluginsDir(), "shisho.koplugin", name)
}

func (s *scenario) catalogs(result *Result) []OPDSServer {
	s.t.Helper()
	var matching []OPDSServer
	for _, server := range result.OPDS {
		if strings.HasPrefix(server.URL, s.containerServer()) {
			matching = append(matching, server)
		}
	}
	return matching
}

func TestPluginSignsInFromItsZip(t *testing.T) {
	s := newScenario(t)
	s.installPlugin(s.devicePageZip())

	// Without Wi-Fi, nothing happens yet.
	offline := s.run(Args{Command: "plugin-launch"})
	require.NotNil(t, offline.Plugin)
	assert.Empty(t, offline.Plugin.Servers)
	assert.Nil(t, offline.Plugin.SignInURL, "a code that hasn't expired waits for Wi-Fi")

	// With Wi-Fi it signs itself in, and adds the catalog.
	online := s.run(Args{Command: "plugin-launch", Online: true})
	require.Len(t, online.Plugin.Servers, 1, "shown: %v", online.Shown)
	server := online.Plugin.Servers[0]
	assert.Equal(t, s.containerServer(), server.URL)
	assert.Equal(t, "reader", server.Username)
	assert.Equal(t, s.device.Credential, server.Credential)
	assert.Equal(t, s.device.Name, server.DeviceName)
	assert.Equal(t, s.containerServer(), online.Plugin.InstalledFrom)
	assert.Contains(t, strings.Join(online.Shown, " "), "Signed in to")

	catalogs := s.catalogs(online)
	require.Len(t, catalogs, 1)
	assert.Equal(t, s.containerServer()+"/opds/v1/catalog", catalogs[0].URL)
	assert.Equal(t, "reader", catalogs[0].Username)
	assert.Equal(t, s.device.Credential, catalogs[0].Password)
	assert.Greater(t, len(online.OPDS), 1, "KOReader's own catalogs stay")

	// The catalog entry browses and downloads.
	target, err := findAcquisition(s.ctx, s.url, catalogs[0].Username, catalogs[0].Password, "Long Book")
	require.NoError(t, err)
	downloaded := s.run(Args{Command: "download", URL: target, Output: "catalog.epub", Username: catalogs[0].Username, Password: catalogs[0].Password})
	assert.NotEmpty(t, downloaded.Hash)

	// The Device page shows the plugin connection.
	device, err := s.devices.Retrieve(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	require.NotNil(t, device.PluginLastConnectedAt)
	require.NotNil(t, device.PluginVersion)
	assert.Equal(t, online.Plugin.Version, *device.PluginVersion)

	// A later launch doesn't sign in again.
	again := s.run(Args{Command: "plugin-launch", Online: true})
	assert.Len(t, again.Plugin.Servers, 1)
	assert.NotContains(t, strings.Join(again.Shown, " "), "Signed in to")

	// After regenerating the credential, one sign-in updates the catalog
	// entry rather than adding another.
	regenerated, err := s.devices.RegenerateCredential(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	stale := s.run(Args{Command: "plugin-launch", Online: true})
	require.Len(t, stale.Plugin.Servers, 1)
	assert.True(t, stale.Plugin.Servers[0].NeedsLogin, "the old credential needs a new sign-in")

	wrong := s.run(Args{Command: "plugin-signin", Server: s.url, Username: "reader", Password: s.device.Credential, Online: true})
	assert.False(t, wrong.Plugin.SignedIn)
	assert.Equal(t, "needs_login", wrong.Plugin.Outcome)

	signedIn := s.run(Args{Command: "plugin-signin", Server: s.url, Username: "reader", Password: regenerated.Credential, Online: true})
	require.True(t, signedIn.Plugin.SignedIn, signedIn.Plugin.Message)
	require.Len(t, signedIn.Plugin.Servers, 1)
	assert.False(t, signedIn.Plugin.Servers[0].NeedsLogin)
	assert.Equal(t, regenerated.Credential, signedIn.Plugin.Servers[0].Credential)
	catalogs = s.catalogs(signedIn)
	require.Len(t, catalogs, 1, "the entry is updated, not duplicated")
	assert.Equal(t, regenerated.Credential, catalogs[0].Password)
}

// expirePairing makes the installed zip's pairing look as it would 30
// minutes later, on the device and on the server.
func (s *scenario) expirePairing() {
	s.t.Helper()
	path := s.pluginFile("pairing.lua")
	data, err := os.ReadFile(path)
	require.NoError(s.t, err)
	expired := regexp.MustCompile(`expires_at = \d+`).ReplaceAll(data, []byte("expires_at = 1"))
	require.NoError(s.t, os.WriteFile(path, expired, 0o644))
	_, err = s.db.NewUpdate().Model((*devices.DeviceShortURL)(nil)).
		Set("expires_at = ?", time.Now().Add(-time.Minute)).Where("device_id = ?", s.device.ID).Exec(s.ctx)
	require.NoError(s.t, err)
}

func TestPluginWithAnExpiredCodeAsksForANewOne(t *testing.T) {
	s := newScenario(t)
	s.installPlugin(s.devicePageZip())
	s.expirePairing()

	launched := s.run(Args{Command: "plugin-launch", Online: true})
	assert.Empty(t, launched.Plugin.Servers)
	require.NotNil(t, launched.Plugin.SignInURL, "sign-in opens")
	assert.Equal(t, s.containerServer(), *launched.Plugin.SignInURL, "with the server filled in")
	assert.Empty(t, s.catalogs(launched))

	// Pair on the Device page, and enter the code.
	short, err := s.devices.GenerateShortURL(s.ctx, s.user.ID, s.device.ID)
	require.NoError(t, err)
	signedIn := s.run(Args{Command: "plugin-signin", Server: *launched.Plugin.SignInURL, Code: strings.ToUpper(short.ShortCode), Online: true})
	require.True(t, signedIn.Plugin.SignedIn, signedIn.Plugin.Message)
	require.Len(t, signedIn.Plugin.Servers, 1)
	require.Len(t, s.catalogs(signedIn), 1)
	assert.Equal(t, s.containerServer(), signedIn.Plugin.InstalledFrom, "the zip came from this server")

	// The expired pairing doesn't ask again.
	again := s.run(Args{Command: "plugin-launch", Online: true})
	assert.Nil(t, again.Plugin.SignInURL)
}

func TestPluginBeforeAnySignInChecksForNoUpdate(t *testing.T) {
	s := newScenario(t)
	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(t, err)
	s.installPlugin(bundle.Zip())

	launched := s.run(Args{Command: "plugin-launch", Online: true})
	assert.Empty(t, launched.Plugin.Servers)
	assert.Nil(t, launched.Plugin.SignInURL, "a release zip has no server to ask about")
	assert.Nil(t, launched.Plugin.Offer)
	checked := s.run(Args{Command: "plugin-update", Online: true})
	assert.Nil(t, checked.Plugin.Offer)
	require.NotNil(t, checked.Plugin.SignInURL, "checking with no server opens sign-in")
	assert.Empty(t, *checked.Plugin.SignInURL, "a release zip knows no server")
}

// olderBuild is the embedded plugin with one more file, zipped as another
// build of the server's own version and paired with the scenario's server.
func (s *scenario) olderBuild() []byte {
	s.t.Helper()
	files := fstest.MapFS{}
	err := fs.WalkDir(pluginfiles.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(pluginfiles.FS, path)
		files[path] = &fstest.MapFile{Data: data}
		return err
	})
	require.NoError(s.t, err)
	files[pluginfiles.Dir+"/stale.lua"] = &fstest.MapFile{Data: []byte("return {}\n")}
	current, err := koreaderplugin.DefaultBundle()
	require.NoError(s.t, err)
	older, err := koreaderplugin.NewBundle(files, "dev")
	require.NoError(s.t, err)
	require.Equal(s.t, current.Version, older.Version)
	require.NotEqual(s.t, current.Hash, older.Hash)

	short, err := s.devices.GenerateShortURL(s.ctx, s.user.ID, s.device.ID)
	require.NoError(s.t, err)
	data, err := older.PairedZip(s.containerServer(), short.ShortCode, short.ExpiresAt)
	require.NoError(s.t, err)
	return data
}

func TestPluginUpdatesFromItsServer(t *testing.T) {
	s := newScenario(t)
	current, err := koreaderplugin.DefaultBundle()
	require.NoError(t, err)
	s.installPlugin(s.olderBuild())

	launched := s.run(Args{Command: "plugin-launch", Online: true})
	require.Len(t, launched.Plugin.Servers, 1, "shown: %v", launched.Shown)
	require.NotEqual(t, current.Hash, launched.Plugin.Hash)
	// Wi-Fi coming on offered the server's build, installed from it.
	require.NotNil(t, launched.Plugin.Offer)
	assert.Equal(t, current.Hash, launched.Plugin.Offer.Hash)

	updated := s.run(Args{Command: "plugin-update", Online: true, Confirm: true})
	assert.Contains(t, updated.Plugin.PromptedUpdate, "new build")
	assert.True(t, updated.Plugin.Restart, "KOReader restarts into it")
	assert.Equal(t, current.Hash, updated.Plugin.Hash)
	_, err = os.Stat(s.pluginFile("stale.lua"))
	assert.True(t, os.IsNotExist(err), "the old plugin's files are gone")
	_, err = os.Stat(s.pluginFile("pairing.lua"))
	assert.True(t, os.IsNotExist(err), "an update carries no pairing")
	require.Len(t, updated.Plugin.Servers, 1, "the sign-in survives the update")

	upToDate := s.run(Args{Command: "plugin-update", Online: true})
	assert.Nil(t, upToDate.Plugin.Offer)
	assert.Contains(t, strings.Join(upToDate.Shown, " "), "up to date")

	// Reinstall plugin from this server installs on purpose.
	reinstalled := s.run(Args{Command: "plugin-update", Online: true, From: s.url})
	assert.True(t, reinstalled.Plugin.Installed, reinstalled.Plugin.Message)
	assert.True(t, reinstalled.Plugin.Restart)
	assert.Equal(t, current.Hash, reinstalled.Plugin.Hash)
	assert.Equal(t, s.containerServer(), reinstalled.Plugin.InstalledFrom)
}

// Check for plugin update turns Wi-Fi on and runs before the paired zip's own
// sign-in, which waits for Wi-Fi's event. It signs in from the zip first,
// rather than saying to sign in and then signing in.
func TestPluginUpdateCheckSignsInFromTheZipFirst(t *testing.T) {
	s := newScenario(t)
	s.installPlugin(s.devicePageZip())

	checked := s.run(Args{Command: "plugin-update", Online: true})

	require.Len(t, checked.Plugin.Servers, 1, "shown: %v", checked.Shown)
	shown := strings.Join(checked.Shown, " | ")
	assert.Contains(t, shown, "Signed in to")
	assert.NotContains(t, shown, "Sign in to a Shisho server first")
	assert.Equal(t, 1, strings.Count(shown, "up to date"), "one check, not one per path: %s", shown)
}

// KOReader lists a plugin it doesn't know after More tools, often on the
// Tools tab's second page. The plugin sits at the top of Tools instead.
func TestPluginMenuLeadsTools(t *testing.T) {
	s := newScenario(t)
	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(t, err)
	s.installPlugin(bundle.Zip())

	menu := s.run(Args{Command: "plugin-menu"})

	require.NotEmpty(t, menu.Plugin.Tools)
	assert.Equal(t, "Shisho", menu.Plugin.Tools[0], "tools: %v", menu.Plugin.Tools)
}

// Reinstalling from a server only matters with more than one, so the menu
// offers it only then.
func TestPluginMenuOffersReinstallOnlyWithSeveralServers(t *testing.T) {
	s := newScenario(t)
	bundle, err := koreaderplugin.DefaultBundle()
	require.NoError(t, err)
	s.installPlugin(bundle.Zip())
	state := filepath.Join(s.h.Dir, "home", ".config", "koreader", "settings", "shisho.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(state), 0o755))

	require.NoError(t, os.WriteFile(state, []byte(`return { servers = { { url = "http://a.example" } } }`), 0o644))
	one := s.run(Args{Command: "plugin-menu"})
	assert.Equal(t, []string{
		"a.example", "a.example > Sign in again", "a.example > Remove server",
		"Sync now", "Settings", "Settings > Sync on sleep",
		"Settings > Newer position from another reader", "Settings > Older position from another reader",
		"Add a server", "Check for plugin update", "Plugin version " + one.Plugin.Version,
	}, one.Plugin.Menu)

	require.NoError(t, os.WriteFile(state, []byte(`return { servers = { { url = "http://a.example" }, { url = "http://b.example" } } }`), 0o644))
	two := s.run(Args{Command: "plugin-menu"})
	assert.Contains(t, two.Plugin.Menu, "a.example > Reinstall plugin from this server")
	assert.Contains(t, two.Plugin.Menu, "b.example > Reinstall plugin from this server")
	assert.NotContains(t, two.Plugin.Menu, "Install plugin from")
}

// Check for plugin update turns Wi-Fi on, and Wi-Fi's own check follows a
// moment later. The update is offered once, not again by that check.
func TestPluginUpdateCheckOffersOnce(t *testing.T) {
	s := newScenario(t)
	s.installPlugin(s.olderBuild())

	checked := s.run(Args{Command: "plugin-update", Online: true})

	require.NotNil(t, checked.Plugin.Offer, "shown: %v", checked.Shown)
	assert.Equal(t, 1, strings.Count(strings.Join(checked.Shown, " | "), "new build"), "shown: %v", checked.Shown)
}
