package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// identifierPluginManifest declares an identifier type, so loading the
// plugin writes plugin_identifier_types, and a metadata enricher hook, so
// loading it appends to plugin_hook_configs.
const identifierPluginManifest = `{
  "manifestVersion": 1,
  "id": "%s",
  "name": "Identifier Plugin",
  "version": "1.0.0",
  "capabilities": {
    "identifierTypes": [{"id": "shelfmark", "name": "Shelfmark"}],
    "metadataEnricher": {"fileTypes": ["epub"], "fields": ["title"]}
  }
}`

const enricherMainJS = `var plugin=(function(){return{metadataEnricher:{search:function(){return{results:[]}}}};})();`

// brokenPluginManifest declares an enricher field that does not exist, so
// the plugin installs but fails to load.
const brokenPluginManifest = `{
  "manifestVersion": 1,
  "id": "%s",
  "name": "Broken Plugin",
  "version": "1.0.0",
  "capabilities": {"metadataEnricher": {"fileTypes": ["epub"], "fields": ["nonsenseField"]}}
}`

// writePluginFiles writes manifest.json (with id substituted) and main.js
// for scope/id under pluginDir.
func writePluginFiles(t *testing.T, pluginDir, scope, id, manifest string) {
	t.Helper()
	dir := filepath.Join(pluginDir, scope, id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(fmt.Sprintf(manifest, id)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(enricherMainJS), 0o644))
}

// pluginZip returns a plugin package holding the given files.
func pluginZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// skipIfRoot skips a test that relies on permission bits, which root ignores.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
}

// installPluginRow inserts a plugin row for test/id with the given status.
func installPluginRow(t *testing.T, svc *Service, id string, status models.PluginStatus) {
	t.Helper()
	require.NoError(t, svc.InstallPlugin(context.Background(), &models.Plugin{
		Scope: "test", ID: id, Name: id, Version: "1.0.0", Status: status, InstalledAt: time.Now(),
	}))
}

// pluginRequestContext builds a JSON request context with scope and id path
// params.
func pluginRequestContext(body, scope, id string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(http.MethodPost, "/", nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if scope != "" {
		c.SetParamNames("scope", "id")
		c.SetParamValues(scope, id)
	}
	return c, rec
}

// Install sorts InstallPlugin failures: a bad URL, checksum, or package is a
// 422, a download host that fails is a 502, and a local IO failure is a 500.
// Mutates global AllowedDownloadHosts, so it cannot run in parallel.
func TestInstall_InstallerErrorMapping(t *testing.T) {
	goodZip := pluginZip(t, map[string]string{
		"manifest.json": fmt.Sprintf(identifierPluginManifest, "good"),
		"main.js":       enricherMainJS,
	})
	noManifestZip := pluginZip(t, map[string]string{"main.js": enricherMainJS})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good.zip":
			_, _ = w.Write(goodZip)
		case "/no-manifest.zip":
			_, _ = w.Write(noManifestZip)
		case "/not-a-zip.zip":
			_, _ = w.Write([]byte("not a zip"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	tests := []struct {
		name   string
		url    string
		sha    string
		status int
		code   string
	}{
		{"download URL not allowed", "https://example.com/good.zip", sha256Hex(goodZip), http.StatusUnprocessableEntity, "validation_error"},
		{"checksum mismatch", server.URL + "/good.zip", "0000", http.StatusUnprocessableEntity, "validation_error"},
		{"package without a manifest", server.URL + "/no-manifest.zip", sha256Hex(noManifestZip), http.StatusUnprocessableEntity, "validation_error"},
		{"package that is not a zip", server.URL + "/not-a-zip.zip", sha256Hex([]byte("not a zip")), http.StatusUnprocessableEntity, "validation_error"},
		{"download host error", server.URL + "/missing.zip", "0000", http.StatusBadGateway, "upstream_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testdb.New(t)
			svc := NewService(db)
			pluginDir := t.TempDir()
			h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

			c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"good","download_url":%q,"sha256":%q}`, tt.url, tt.sha), "", "")
			assertErrcodeFields(t, h.install(c), tt.status, tt.code)
		})
	}

	t.Run("plugin directory not writable", func(t *testing.T) {
		skipIfRoot(t)
		db := testdb.New(t)
		svc := NewService(db)
		pluginDir := t.TempDir()
		require.NoError(t, os.Chmod(pluginDir, 0o500))
		t.Cleanup(func() { _ = os.Chmod(pluginDir, 0o755) })
		h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

		c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"good","download_url":%q,"sha256":%q}`, server.URL+"/good.zip", sha256Hex(goodZip)), "", "")
		assertServerFault(t, h.install(c))
	})
}

// assertErrcodeFields requires err to be an errcodes error with status and
// code, whatever its message.
func assertErrcodeFields(t *testing.T, err error, status int, code string) {
	t.Helper()
	require.Error(t, err)
	var ecErr *errcodes.Error
	require.ErrorAs(t, err, &ecErr, "want an errcodes error, got %T: %v", err, err)
	assert.Equal(t, status, ecErr.HTTPCode, ecErr.Message)
	assert.Equal(t, code, ecErr.Code, ecErr.Message)
}

// Update Version shares install's mapping: a checksum mismatch is a 422 and
// a download host error is a 502, not the 500 every failure used to be.
// Mutates global AllowedDownloadHosts and AllowedFetchHosts.
func TestUpdateVersion_InstallerErrorMapping(t *testing.T) {
	zipData := pluginZip(t, map[string]string{
		"manifest.json": fmt.Sprintf(identifierPluginManifest, "upd"),
		"main.js":       enricherMainJS,
	})
	var serverURL string
	versionURL := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			manifest := RepositoryManifest{
				RepositoryVersion: 1, Scope: "test", Name: "Test Repo",
				Plugins: []AvailablePlugin{{ID: "upd", Name: "Upd", Versions: []PluginVersion{
					{Version: "2.0.0", ManifestVersion: 1, DownloadURL: serverURL + versionURL["path"], SHA256: versionURL["sha"]},
				}}},
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case "/upd.zip":
			_, _ = w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	origDownload, origFetch := AllowedDownloadHosts, AllowedFetchHosts
	AllowedDownloadHosts, AllowedFetchHosts = []string{server.URL}, []string{server.URL}
	defer func() { AllowedDownloadHosts, AllowedFetchHosts = origDownload, origFetch }()

	tests := []struct {
		name, path, sha string
		status          int
		code            string
	}{
		{"checksum mismatch", "/upd.zip", "0000", http.StatusUnprocessableEntity, "validation_error"},
		{"download host error", "/missing.zip", "0000", http.StatusBadGateway, "upstream_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionURL["path"], versionURL["sha"] = tt.path, tt.sha
			db := testdb.New(t)
			svc := NewService(db)
			ctx := context.Background()
			available := "2.0.0"
			require.NoError(t, svc.InstallPlugin(ctx, &models.Plugin{
				Scope: "test", ID: "upd", Name: "Upd", Version: "1.0.0", Status: models.PluginStatusActive,
				InstalledAt: time.Now(), UpdateAvailableVersion: &available,
			}))
			require.NoError(t, svc.AddRepository(ctx, &models.PluginRepository{URL: server.URL + "/manifest.json", Scope: "test", Enabled: true}))
			pluginDir := t.TempDir()
			h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

			c, _ := pluginRequestContext("", "test", "upd")
			assertErrcodeFields(t, h.updateVersion(c), tt.status, tt.code)
		})
	}
}

// Installing from repositories when no repository answered reports the
// failed fetch as a 502 instead of claiming the plugin is not listed. A
// repository that answers without the plugin is still a 404.
// Mutates global AllowedFetchHosts.
func TestInstall_RepositoryFetchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty.json" {
			_ = json.NewEncoder(w).Encode(RepositoryManifest{RepositoryVersion: 1, Scope: "test", Name: "Empty"})
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	origFetch := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origFetch }()

	tests := []struct {
		name, path string
		status     int
		code       string
	}{
		{"repository unreachable", "/down.json", http.StatusBadGateway, "upstream_error"},
		{"repository without the plugin", "/empty.json", http.StatusNotFound, "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testdb.New(t)
			svc := NewService(db)
			require.NoError(t, svc.AddRepository(context.Background(), &models.PluginRepository{URL: server.URL + tt.path, Scope: "test", Enabled: true}))
			pluginDir := t.TempDir()
			h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

			c, _ := pluginRequestContext(`{"scope":"test","id":"missing"}`, "", "")
			assertErrcodeFields(t, h.install(c), tt.status, tt.code)
		})
	}
}

// failPluginUpdates makes every UPDATE of the plugins table fail, leaving
// inserts and reads working.
func failPluginUpdates(t *testing.T, db *bun.DB) {
	t.Helper()
	_, err := db.Exec(`CREATE TRIGGER fail_plugin_updates BEFORE UPDATE ON plugins BEGIN SELECT RAISE(ABORT, 'injected update fault'); END`)
	require.NoError(t, err)
}

// An install whose plugin fails to load stores the failure on the row it
// inserts. If that write fails the request is a 500, not a 201 whose body
// reports a status the database never saved, and the files are removed.
// Mutates global AllowedDownloadHosts.
func TestInstall_LoadFailureWriteFaultIsServerError(t *testing.T) {
	zipData := pluginZip(t, map[string]string{
		"manifest.json": fmt.Sprintf(brokenPluginManifest, "broken"),
		"main.js":       enricherMainJS,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipData)
	}))
	defer server.Close()
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	db := testdb.New(t)
	svc := NewService(db)
	_, err := db.Exec(`CREATE TRIGGER fail_plugin_inserts BEFORE INSERT ON plugins BEGIN SELECT RAISE(ABORT, 'injected insert fault'); END`)
	require.NoError(t, err)
	pluginDir := t.TempDir()
	h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

	c, rec := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"broken","download_url":%q,"sha256":%q}`, server.URL+"/broken.zip", sha256Hex(zipData)), "", "")
	assertServerFault(t, h.install(c))
	assert.NotEqual(t, http.StatusCreated, rec.Code)

	// The install is removed rather than left Active with no runtime.
	_, err = svc.RetrievePlugin(context.Background(), "test", "broken")
	require.ErrorIs(t, err, sql.ErrNoRows, "the plugin row must be removed")
	assert.NoDirExists(t, filepath.Join(pluginDir, "test", "broken"), "the plugin files must be removed")
}

// Enabling a plugin whose identifier types cannot be stored is a server
// fault. The plugin keeps its stored state and is not left loaded, instead
// of being marked Malfunctioned for a database problem.
func TestUpdate_EnableDatabaseFaultIsServerError(t *testing.T) {
	t.Parallel()

	for _, table := range []string{"plugin_identifier_types", "plugin_hook_configs"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			svc := NewService(db)
			pluginDir := t.TempDir()
			writePluginFiles(t, pluginDir, "test", "ids", identifierPluginManifest)
			installPluginRow(t, svc, "ids", models.PluginStatusDisabled)
			mgr := NewManager(svc, pluginDir, "")
			h := NewHandler(svc, mgr, NewInstaller(pluginDir))
			_, err := db.Exec("DROP TABLE " + table)
			require.NoError(t, err)

			c, _ := pluginRequestContext(`{"enabled":true}`, "test", "ids")
			assertServerFault(t, h.update(c))

			stored, err := svc.RetrievePlugin(context.Background(), "test", "ids")
			require.NoError(t, err)
			assert.Equal(t, models.PluginStatusDisabled, stored.Status)
			assert.Nil(t, stored.LoadError)
			assert.Nil(t, mgr.GetRuntime("test", "ids"), "a plugin whose load hit a database fault must not stay loaded")
		})
	}
}

// Loading a plugin that is already in the hook order is not an error: only
// the duplicate insert is ignored.
func TestManager_LoadPluginTwiceIgnoresDuplicateOrder(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	writePluginFiles(t, pluginDir, "test", "ids", identifierPluginManifest)
	installPluginRow(t, svc, "ids", models.PluginStatusActive)
	mgr := NewManager(svc, pluginDir, "")

	require.NoError(t, mgr.LoadPlugin(context.Background(), "test", "ids"))
	require.NoError(t, mgr.LoadPlugin(context.Background(), "test", "ids"))
}

// Reloading a plugin whose identifier types cannot be stored is a server
// fault that leaves the loaded runtime and the stored load error alone.
func TestReload_DatabaseFaultIsServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	writePluginFiles(t, pluginDir, "test", "ids", identifierPluginManifest)
	installPluginRow(t, svc, "ids", models.PluginStatusActive)
	mgr := NewManager(svc, pluginDir, "")
	require.NoError(t, mgr.LoadPlugin(context.Background(), "test", "ids"))
	before := mgr.GetRuntime("test", "ids")
	require.NotNil(t, before)
	h := NewHandler(svc, mgr, NewInstaller(pluginDir))
	_, err := db.Exec("DROP TABLE plugin_identifier_types")
	require.NoError(t, err)

	c, _ := pluginRequestContext("", "test", "ids")
	assertServerFault(t, h.reload(c))

	stored, err := svc.RetrievePlugin(context.Background(), "test", "ids")
	require.NoError(t, err)
	assert.Nil(t, stored.LoadError)
	assert.Same(t, before, mgr.GetRuntime("test", "ids"))
}

// A plugin whose state cannot honor the request is a 422 with the
// invalid_state code.
func TestPluginStateChecksAreInvalidState(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db)
	installPluginRow(t, svc, "idle", models.PluginStatusDisabled)
	pluginDir := t.TempDir()
	h := NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))

	c, _ := pluginRequestContext("", "test", "idle")
	assertErrcode(t, h.reload(c), http.StatusUnprocessableEntity, "invalid_state", "Plugin must be active to reload.")

	c, _ = pluginRequestContext("", "test", "idle")
	assertErrcode(t, h.updateVersion(c), http.StatusUnprocessableEntity, "invalid_state", "No update available for this plugin.")
}

// An icon that exists but cannot be read is a server fault, not a 404.
func TestGetImage_StatFaultIsServerError(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)
	pluginDir := t.TempDir()
	dir := filepath.Join(pluginDir, "test", "icon")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "icon.png"), []byte("png"), 0o644))
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	h := &handler{installer: NewInstaller(pluginDir)}

	c, _ := pluginRequestContext("", "test", "icon")
	assertServerFault(t, h.getImage(c))
}

// A manifest must name each identifier type once, with an id. A repeated id
// would otherwise fail the identifier type insert, a database error that
// hides the plugin's own mistake.
func TestParseManifest_RejectsBadIdentifierTypeIDs(t *testing.T) {
	t.Parallel()

	for name, types := range map[string]string{
		"repeated id": `[{"id": "shelfmark", "name": "A"}, {"id": "shelfmark", "name": "B"}]`,
		"empty id":    `[{"id": "", "name": "A"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseManifest([]byte(`{"manifestVersion": 1, "id": "p", "name": "P", "version": "1.0.0",
				"capabilities": {"identifierTypes": ` + types + `}}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "identifierTypes")
		})
	}
}

// corruptEntryZip returns a package whose manifest.json is stored with a
// CRC32 that does not match its content, so it opens but fails to read.
func corruptEntryZip(t *testing.T) []byte {
	t.Helper()
	content := []byte(fmt.Sprintf(identifierPluginManifest, "good"))
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.CreateRaw(&zip.FileHeader{
		Name: "manifest.json", Method: zip.Store, CRC32: 1,
		CompressedSize64: uint64(len(content)), UncompressedSize64: uint64(len(content)),
	})
	require.NoError(t, err)
	_, err = f.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// modeZeroZip returns a valid package whose files carry no permission bits.
func modeZeroZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"manifest.json": fmt.Sprintf(identifierPluginManifest, "good"),
		"main.js":       enricherMainJS,
	} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0)
		f, err := w.CreateHeader(header)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// A package entry whose data is corrupt is an invalid package (422), and a
// package whose entries carry no permission bits still installs.
// Mutates global AllowedDownloadHosts.
func TestInstall_PackageEntryProblems(t *testing.T) {
	corrupt := corruptEntryZip(t)
	modeZero := modeZeroZip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/corrupt.zip" {
			_, _ = w.Write(corrupt)
			return
		}
		_, _ = w.Write(modeZero)
	}))
	defer server.Close()
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	newHandler := func(t *testing.T) *handler {
		db := testdb.New(t)
		svc := NewService(db)
		pluginDir := t.TempDir()
		return NewHandler(svc, NewManager(svc, pluginDir, ""), NewInstaller(pluginDir))
	}

	t.Run("corrupt entry", func(t *testing.T) {
		c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"good","download_url":%q,"sha256":%q}`, server.URL+"/corrupt.zip", sha256Hex(corrupt)), "", "")
		assertErrcodeFields(t, newHandler(t).install(c), http.StatusUnprocessableEntity, "validation_error")
	})
	t.Run("entries without permission bits", func(t *testing.T) {
		skipIfRoot(t)
		c, rec := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"good","download_url":%q,"sha256":%q}`, server.URL+"/mode-zero.zip", sha256Hex(modeZero)), "", "")
		require.NoError(t, newHandler(t).install(c))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}

// An install whose load hits a server fault removes what it installed, so
// the plugin is not left Active in the database with no runtime. Here the
// identifier types table is missing.
// Mutates global AllowedDownloadHosts.
func TestInstall_LoadServerFaultRemovesInstall(t *testing.T) {
	zipData := pluginZip(t, map[string]string{
		"manifest.json": fmt.Sprintf(identifierPluginManifest, "ids"),
		"main.js":       enricherMainJS,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipData)
	}))
	defer server.Close()
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	db := testdb.New(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	mgr := NewManager(svc, pluginDir, "")
	h := NewHandler(svc, mgr, NewInstaller(pluginDir))
	_, err := db.Exec("DROP TABLE plugin_identifier_types")
	require.NoError(t, err)

	c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"ids","download_url":%q,"sha256":%q}`, server.URL+"/ids.zip", sha256Hex(zipData)), "", "")
	assertServerFault(t, h.install(c))

	_, err = svc.RetrievePlugin(context.Background(), "test", "ids")
	require.ErrorIs(t, err, sql.ErrNoRows, "the plugin row must be removed")
	assert.NoDirExists(t, filepath.Join(pluginDir, "test", "ids"), "the plugin files must be removed")
	assert.Nil(t, mgr.GetRuntime("test", "ids"))
}

// An icon file that exists but cannot be opened is a server fault, not the
// 404 echo's c.File returns for every open failure.
func TestGetImage_OpenFaultIsServerError(t *testing.T) {
	t.Parallel()
	skipIfRoot(t)
	pluginDir := t.TempDir()
	dir := filepath.Join(pluginDir, "test", "icon")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	iconPath := filepath.Join(dir, "icon.png")
	require.NoError(t, os.WriteFile(iconPath, []byte("png"), 0o644))
	require.NoError(t, os.Chmod(iconPath, 0o000))
	t.Cleanup(func() { _ = os.Chmod(iconPath, 0o644) })
	h := &handler{installer: NewInstaller(pluginDir)}

	c, _ := pluginRequestContext("", "test", "icon")
	err := h.getImage(c)
	assertServerFault(t, err)
	var httpErr *echo.HTTPError
	assert.NotErrorAs(t, err, &httpErr, "want a server fault, not echo's 404")
}
