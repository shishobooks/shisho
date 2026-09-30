package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// lifecycleSpec describes one version of a test plugin in scope "test".
type lifecycleSpec struct {
	id, version     string
	enricher        bool
	parser          bool
	identifierTypes bool
	brokenScript    bool
	extraFiles      map[string]string
}

// files returns the plugin package contents for the spec.
func (s lifecycleSpec) files() map[string]string {
	caps := map[string]any{}
	var hooks []string
	if s.enricher {
		caps["metadataEnricher"] = map[string]any{"fileTypes": []string{"epub"}, "fields": []string{"title"}}
		hooks = append(hooks, `metadataEnricher:{search:function(){return{results:[]}}}`)
	}
	if s.parser {
		caps["fileParser"] = map[string]any{"types": []string{"lcx"}}
		hooks = append(hooks, `fileParser:{parse:function(){return{title:"T"}}}`)
	}
	if s.identifierTypes {
		caps["identifierTypes"] = []map[string]string{{"id": "shelfmark", "name": "Shelfmark"}}
	}
	manifest, _ := json.Marshal(map[string]any{
		"manifestVersion": 1, "id": s.id, "name": "Plugin " + s.id, "version": s.version, "capabilities": caps,
	})
	mainJS := "var plugin=(function(){return{" + strings.Join(hooks, ",") + "};})();"
	if s.brokenScript {
		mainJS = "var plugin = (;"
	}
	files := map[string]string{"manifest.json": string(manifest), "main.js": mainJS}
	for name, content := range s.extraFiles {
		files[name] = content
	}
	return files
}

// writeLive replaces the live directory of the spec's plugin with its files.
func writeLive(t *testing.T, pluginDir string, s lifecycleSpec) {
	t.Helper()
	dir := filepath.Join(pluginDir, "test", s.id)
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, content := range s.files() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
}

// liveManifestVersion reads the version from the live manifest on disk.
func liveManifestVersion(t *testing.T, pluginDir, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(pluginDir, "test", id, "manifest.json"))
	require.NoError(t, err)
	m, err := ParseManifest(data)
	require.NoError(t, err)
	return m.Version
}

// lifecycleEnv is a database, plugin directory, manager, and handler for
// one test.
type lifecycleEnv struct {
	db        *bun.DB
	svc       *Service
	mgr       *Manager
	h         *handler
	pluginDir string
}

func newLifecycleEnv(t *testing.T) *lifecycleEnv {
	t.Helper()
	db := testdb.New(t)
	svc := NewService(db)
	pluginDir := t.TempDir()
	mgr := NewManager(svc, pluginDir, t.TempDir())
	return &lifecycleEnv{db: db, svc: svc, mgr: mgr, h: NewHandler(svc, mgr, NewInstaller(pluginDir)), pluginDir: pluginDir}
}

// install writes the spec's files, inserts its row with status, and loads
// it when Active. updateAvailable, when set, is stored on the row.
func (e *lifecycleEnv) install(t *testing.T, s lifecycleSpec, status models.PluginStatus, updateAvailable string) {
	t.Helper()
	ctx := context.Background()
	writeLive(t, e.pluginDir, s)
	row := &models.Plugin{
		Scope: "test", ID: s.id, Name: "Plugin " + s.id, Version: s.version, Status: status,
		AutoUpdate: true, InstalledAt: time.Now(),
	}
	if updateAvailable != "" {
		row.UpdateAvailableVersion = &updateAvailable
	}
	if status == models.PluginStatusMalfunctioned {
		msg := "failed to load"
		row.LoadError = &msg
	}
	require.NoError(t, e.svc.InstallPlugin(ctx, row))
	if status == models.PluginStatusActive {
		require.NoError(t, e.mgr.LoadPlugin(ctx, "test", s.id))
	}
}

func (e *lifecycleEnv) row(t *testing.T, id string) *models.Plugin {
	t.Helper()
	p, err := e.svc.RetrievePlugin(context.Background(), "test", id)
	require.NoError(t, err)
	return p
}

// count returns the number of rows in table for plugin test/id.
func (e *lifecycleEnv) count(t *testing.T, table, id string, extra ...string) int {
	t.Helper()
	q := strings.Join(append([]string{"SELECT COUNT(*) FROM " + table + " WHERE scope = 'test' AND plugin_id = ?"}, extra...), " AND ")
	var n int
	require.NoError(t, e.db.QueryRow(q, id).Scan(&n))
	return n
}

// packageServer serves a repository index for test/id at /repo.json and
// the package at /pkg.zip. With block set, each download signals hit and
// waits until release is closed. It points the download and fetch host
// allowlists at itself for the rest of the test, so its callers cannot run
// in parallel.
type packageServer struct {
	*httptest.Server
	hit         chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func servePackage(t *testing.T, id, version string, zipData []byte, block bool) *packageServer {
	t.Helper()
	ps := &packageServer{hit: make(chan struct{}, 1), release: make(chan struct{})}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repo.json":
			_ = json.NewEncoder(w).Encode(RepositoryManifest{
				RepositoryVersion: 1, Scope: "test", Name: "Test",
				Plugins: []AvailablePlugin{{ID: id, Name: id, Versions: []PluginVersion{
					{Version: version, ManifestVersion: 1, DownloadURL: ps.URL + "/pkg.zip", SHA256: sha256Hex(zipData)},
				}}},
			})
		case "/pkg.zip":
			if block {
				ps.hit <- struct{}{}
				<-ps.release
			}
			_, _ = w.Write(zipData)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		ps.releaseNow()
		ps.Close()
	})
	origDownload, origFetch := AllowedDownloadHosts, AllowedFetchHosts
	AllowedDownloadHosts, AllowedFetchHosts = []string{ps.URL}, []string{ps.URL}
	t.Cleanup(func() { AllowedDownloadHosts, AllowedFetchHosts = origDownload, origFetch })
	return ps
}

// releaseNow lets blocked downloads finish. It is safe to call twice.
func (ps *packageServer) releaseNow() {
	ps.releaseOnce.Do(func() { close(ps.release) })
}

func (e *lifecycleEnv) addRepo(t *testing.T, url string) {
	t.Helper()
	require.NoError(t, e.svc.AddRepository(context.Background(), &models.PluginRepository{URL: url, Scope: "test", Enabled: true}))
}

// waitOrFail waits for a result, failing the test instead of hanging if
// the call is blocked behind another transition.
func waitOrFail(t *testing.T, done <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(30 * time.Second):
		t.Fatalf("%s did not finish", what)
		return nil
	}
}

// A reload that adds a hook type puts the plugin in the global
// order and in every customized library order, so the new hook runs.
func TestReload_AppendsNewHookTypes(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	ctx := context.Background()
	library := insertTestLibrary(t, e.db, "Library")
	require.NoError(t, e.svc.SetLibraryOrder(ctx, library.ID, models.PluginHookMetadataEnricher, []models.LibraryPluginHookConfig{}))
	e.install(t, lifecycleSpec{id: "grow", version: "1.0.0", parser: true}, models.PluginStatusActive, "")

	writeLive(t, e.pluginDir, lifecycleSpec{id: "grow", version: "1.1.0", parser: true, enricher: true})
	c, _ := pluginRequestContext("", "test", "grow")
	require.NoError(t, e.h.reload(c))

	order, err := e.svc.GetOrder(ctx, models.PluginHookMetadataEnricher)
	require.NoError(t, err)
	require.Len(t, order, 1)
	assert.Equal(t, "grow", order[0].PluginID)

	for _, libraryID := range []int{0, library.ID} {
		rts, err := e.mgr.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, libraryID)
		require.NoError(t, err)
		require.Len(t, rts, 1, "library %d", libraryID)
		assert.Equal(t, "grow", rts[0].pluginID)
	}
	assert.Equal(t, "1.1.0", e.row(t, "grow").Version)
}

// An update whose package is not a zip, has no manifest, or has
// a script that does not load returns 422 and keeps the installed files,
// runtime, version, and status.
// Mutates the global host allowlists.
func TestUpdateVersion_BadPackageKeepsInstalledVersion(t *testing.T) {
	v1 := lifecycleSpec{id: "keep", version: "1.0.0", enricher: true}
	tests := []struct {
		name string
		pkg  []byte
		code string
	}{
		{"not a zip", []byte("not a zip"), "validation_error"},
		{"no manifest", pluginZip(t, map[string]string{"main.js": enricherMainJS}), "validation_error"},
		{"broken script", pluginZip(t, lifecycleSpec{id: "keep", version: "2.0.0", enricher: true, brokenScript: true}.files()), "plugin_load_failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := servePackage(t, "keep", "2.0.0", tt.pkg, false)
			e := newLifecycleEnv(t)
			e.addRepo(t, ps.URL+"/repo.json")
			e.install(t, v1, models.PluginStatusActive, "2.0.0")
			before := e.mgr.GetRuntime("test", "keep")
			require.NotNil(t, before)

			c, _ := pluginRequestContext("", "test", "keep")
			assertErrcodeFields(t, e.h.updateVersion(c), http.StatusUnprocessableEntity, tt.code)

			data, err := os.ReadFile(filepath.Join(e.pluginDir, "test", "keep", "manifest.json"))
			require.NoError(t, err, "the installed files must remain")
			assert.JSONEq(t, v1.files()["manifest.json"], string(data))
			assert.Same(t, before, e.mgr.GetRuntime("test", "keep"))
			row := e.row(t, "keep")
			assert.Equal(t, "1.0.0", row.Version)
			assert.Equal(t, models.PluginStatusActive, row.Status)
			require.NotNil(t, row.UpdateAvailableVersion)
			assert.Equal(t, "2.0.0", *row.UpdateAvailableVersion)
		})
	}
}

// An install whose payload id differs from the manifest id is
// refused and leaves nothing under either id.
// Mutates the global host allowlists.
func TestInstall_ManifestIDMismatchIsRejected(t *testing.T) {
	pkg := pluginZip(t, lifecycleSpec{id: "other", version: "1.0.0", enricher: true}.files())
	ps := servePackage(t, "wanted", "1.0.0", pkg, false)
	e := newLifecycleEnv(t)

	c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"wanted","download_url":%q,"sha256":%q}`, ps.URL+"/pkg.zip", sha256Hex(pkg)), "", "")
	assertErrcodeFields(t, e.h.install(c), http.StatusUnprocessableEntity, "validation_error")

	assert.NoDirExists(t, filepath.Join(e.pluginDir, "test", "wanted"))
	assert.NoDirExists(t, filepath.Join(e.pluginDir, "test", "other"))
	staged, _ := os.ReadDir(filepath.Join(e.pluginDir, "test", stagingDirName))
	assert.Empty(t, staged, "the rejected package must not stay staged")
	plugins, err := e.svc.ListPlugins(context.Background())
	require.NoError(t, err)
	assert.Empty(t, plugins)
}

// Installing an id that is already installed is refused and
// leaves its files alone.
// Mutates the global host allowlists.
func TestInstall_AlreadyInstalledIsRefused(t *testing.T) {
	v1 := lifecycleSpec{id: "dup", version: "1.0.0", enricher: true}
	pkg := pluginZip(t, lifecycleSpec{id: "dup", version: "2.0.0", enricher: true}.files())
	ps := servePackage(t, "dup", "2.0.0", pkg, false)
	e := newLifecycleEnv(t)
	e.install(t, v1, models.PluginStatusActive, "")

	c, _ := pluginRequestContext(fmt.Sprintf(`{"scope":"test","id":"dup","download_url":%q,"sha256":%q}`, ps.URL+"/pkg.zip", sha256Hex(pkg)), "", "")
	assertErrcodeFields(t, e.h.install(c), http.StatusUnprocessableEntity, "invalid_state")

	assert.Equal(t, "1.0.0", liveManifestVersion(t, e.pluginDir, "dup"))
	assert.Equal(t, "1.0.0", e.row(t, "dup").Version)
}

// Updating a Disabled plugin leaves it unloaded, and updating a
// Malfunctioned plugin to a version that loads makes it Active with its
// hook order rows.
// Mutates the global host allowlists.
func TestUpdateVersion_RespectsStatus(t *testing.T) {
	v2 := lifecycleSpec{id: "st", version: "2.0.0", enricher: true}
	pkg := pluginZip(t, v2.files())

	t.Run("disabled", func(t *testing.T) {
		ps := servePackage(t, "st", "2.0.0", pkg, false)
		e := newLifecycleEnv(t)
		e.addRepo(t, ps.URL+"/repo.json")
		e.install(t, lifecycleSpec{id: "st", version: "1.0.0", enricher: true}, models.PluginStatusDisabled, "2.0.0")

		c, _ := pluginRequestContext("", "test", "st")
		require.NoError(t, e.h.updateVersion(c))

		assert.Nil(t, e.mgr.GetRuntime("test", "st"), "a disabled plugin must stay unloaded")
		row := e.row(t, "st")
		assert.Equal(t, models.PluginStatusDisabled, row.Status)
		assert.Equal(t, "2.0.0", row.Version)
		assert.Nil(t, row.UpdateAvailableVersion)
		assert.Equal(t, "2.0.0", liveManifestVersion(t, e.pluginDir, "st"))
	})

	t.Run("malfunctioned", func(t *testing.T) {
		ps := servePackage(t, "st", "2.0.0", pkg, false)
		e := newLifecycleEnv(t)
		e.addRepo(t, ps.URL+"/repo.json")
		e.install(t, lifecycleSpec{id: "st", version: "1.0.0", enricher: true, brokenScript: true}, models.PluginStatusMalfunctioned, "2.0.0")

		c, _ := pluginRequestContext("", "test", "st")
		require.NoError(t, e.h.updateVersion(c))

		assert.NotNil(t, e.mgr.GetRuntime("test", "st"))
		row := e.row(t, "st")
		assert.Equal(t, models.PluginStatusActive, row.Status)
		assert.Nil(t, row.LoadError)
		assert.Equal(t, 1, e.count(t, "plugin_hook_configs", "st", "hook_type = 'metadataEnricher'"))
	})
}

// A successful update replaces the whole directory, so a file the new
// version dropped is gone, and leaves no staging or trash behind.
// Mutates the global host allowlists.
func TestUpdateVersion_ReplacesTheDirectory(t *testing.T) {
	pkg := pluginZip(t, lifecycleSpec{id: "swap", version: "2.0.0", enricher: true}.files())
	ps := servePackage(t, "swap", "2.0.0", pkg, false)
	e := newLifecycleEnv(t)
	e.addRepo(t, ps.URL+"/repo.json")
	e.install(t, lifecycleSpec{id: "swap", version: "1.0.0", enricher: true, extraFiles: map[string]string{"old.txt": "old", "icon.png": "icon"}}, models.PluginStatusActive, "2.0.0")
	before := e.mgr.GetRuntime("test", "swap")

	c, _ := pluginRequestContext("", "test", "swap")
	require.NoError(t, e.h.updateVersion(c))

	assert.NoFileExists(t, filepath.Join(e.pluginDir, "test", "swap", "old.txt"))
	icon, err := os.ReadFile(filepath.Join(e.pluginDir, "test", "swap", "icon.png"))
	require.NoError(t, err, "a package without an icon keeps the installed one")
	assert.Equal(t, "icon", string(icon))
	assert.Equal(t, "2.0.0", liveManifestVersion(t, e.pluginDir, "swap"))
	after := e.mgr.GetRuntime("test", "swap")
	require.NotNil(t, after)
	assert.NotSame(t, before, after)
	for _, dir := range []string{stagingDirName, trashDirName} {
		entries, _ := os.ReadDir(filepath.Join(e.pluginDir, "test", dir))
		assert.Empty(t, entries, dir)
	}
}

// Uninstall removes every child row and the directory, and it
// deletes the row before the files, so a failed delete keeps the plugin
// whole.
func TestUninstall_RemovesRowsBeforeFiles(t *testing.T) {
	t.Parallel()
	spec := lifecycleSpec{id: "gone", version: "1.0.0", enricher: true, identifierTypes: true}
	setup := func(t *testing.T) (*lifecycleEnv, int) {
		e := newLifecycleEnv(t)
		ctx := context.Background()
		e.install(t, spec, models.PluginStatusActive, "")
		library := insertTestLibrary(t, e.db, "Library")
		require.NoError(t, e.svc.SetConfig(ctx, "test", "gone", "key", "value"))
		require.NoError(t, e.svc.SetFieldSetting(ctx, "test", "gone", "title", false))
		require.NoError(t, e.svc.SetLibraryOrder(ctx, library.ID, models.PluginHookMetadataEnricher, []models.LibraryPluginHookConfig{{Scope: "test", PluginID: "gone"}}))
		require.NoError(t, e.svc.SetLibraryFieldSetting(ctx, library.ID, "test", "gone", "title", false))
		return e, library.ID
	}
	tables := []string{"plugin_configs", "plugin_identifier_types", "plugin_hook_configs", "library_plugin_hook_configs", "plugin_field_settings", "library_plugin_field_settings"}

	t.Run("removes everything", func(t *testing.T) {
		t.Parallel()
		e, _ := setup(t)
		for _, table := range tables {
			require.Positive(t, e.count(t, table, "gone"), table)
		}

		c, rec := pluginRequestContext("", "test", "gone")
		require.NoError(t, e.h.uninstall(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)

		for _, table := range tables {
			assert.Zero(t, e.count(t, table, "gone"), table)
		}
		assert.NoDirExists(t, filepath.Join(e.pluginDir, "test", "gone"))
		assert.Nil(t, e.mgr.GetRuntime("test", "gone"))
	})

	t.Run("a failed row delete keeps the files", func(t *testing.T) {
		t.Parallel()
		e, _ := setup(t)
		_, err := e.db.Exec(`CREATE TRIGGER fail_plugin_deletes BEFORE DELETE ON plugins BEGIN SELECT RAISE(ABORT, 'injected delete fault'); END`)
		require.NoError(t, err)

		c, _ := pluginRequestContext("", "test", "gone")
		assertServerFault(t, e.h.uninstall(c))

		assert.DirExists(t, filepath.Join(e.pluginDir, "test", "gone"))
		assert.NotNil(t, e.mgr.GetRuntime("test", "gone"))
	})
}

// A version without identifierTypes removes the old types, and
// a version that drops a hook type removes its global and library rows.
func TestReload_RemovesDroppedTypes(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	ctx := context.Background()
	library := insertTestLibrary(t, e.db, "Library")
	e.install(t, lifecycleSpec{id: "shrink", version: "1.0.0", parser: true, enricher: true, identifierTypes: true}, models.PluginStatusActive, "")
	require.NoError(t, e.svc.SetLibraryOrder(ctx, library.ID, models.PluginHookMetadataEnricher, []models.LibraryPluginHookConfig{{Scope: "test", PluginID: "shrink"}}))
	require.Equal(t, 1, e.count(t, "plugin_identifier_types", "shrink"))

	writeLive(t, e.pluginDir, lifecycleSpec{id: "shrink", version: "2.0.0", parser: true})
	c, _ := pluginRequestContext("", "test", "shrink")
	require.NoError(t, e.h.reload(c))

	assert.Zero(t, e.count(t, "plugin_identifier_types", "shrink"))
	assert.Zero(t, e.count(t, "plugin_hook_configs", "shrink", "hook_type = 'metadataEnricher'"))
	assert.Zero(t, e.count(t, "library_plugin_hook_configs", "shrink", "hook_type = 'metadataEnricher'"))
	assert.Equal(t, 1, e.count(t, "plugin_hook_configs", "shrink", "hook_type = 'fileParser'"))
}

// A disable that lands while an update is downloading sticks,
// and the row's version matches the manifest on disk.
// Mutates the global host allowlists.
func TestUpdateVersion_DisableDuringDownloadSticks(t *testing.T) {
	pkg := pluginZip(t, lifecycleSpec{id: "race", version: "2.0.0", enricher: true}.files())
	ps := servePackage(t, "race", "2.0.0", pkg, true)
	e := newLifecycleEnv(t)
	e.addRepo(t, ps.URL+"/repo.json")
	e.install(t, lifecycleSpec{id: "race", version: "1.0.0", enricher: true}, models.PluginStatusActive, "2.0.0")

	updated := make(chan error, 1)
	go func() {
		c, _ := pluginRequestContext("", "test", "race")
		updated <- e.h.updateVersion(c)
	}()
	<-ps.hit

	disabled := make(chan error, 1)
	go func() {
		c, _ := pluginRequestContext(`{"enabled":false}`, "test", "race")
		disabled <- e.h.update(c)
	}()
	require.NoError(t, waitOrFail(t, disabled, "disable"))
	ps.releaseNow()
	require.NoError(t, waitOrFail(t, updated, "update"))

	row := e.row(t, "race")
	assert.Equal(t, models.PluginStatusDisabled, row.Status)
	assert.Nil(t, e.mgr.GetRuntime("test", "race"), "the runtime must match the Disabled status")
	assert.Equal(t, liveManifestVersion(t, e.pluginDir, "race"), row.Version)
	assert.Equal(t, "2.0.0", row.Version)
}

// A disable that lands while the update check is fetching
// repositories stays Disabled.
func TestCheckForUpdates_DisableDuringFetchSticks(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	e.addRepo(t, "https://example.com/repo.json")
	e.install(t, lifecycleSpec{id: "chk", version: "1.0.0", enricher: true}, models.PluginStatusActive, "")
	hit, release := make(chan struct{}, 1), make(chan struct{})
	e.mgr.fetchRepo = func(string) (*RepositoryManifest, error) {
		hit <- struct{}{}
		<-release
		return &RepositoryManifest{RepositoryVersion: 1, Scope: "test", Name: "Test", Plugins: []AvailablePlugin{{
			ID: "chk", Name: "chk", Versions: []PluginVersion{{Version: "2.0.0", ManifestVersion: 1, DownloadURL: "https://github.com/x.zip", SHA256: "0"}},
		}}}, nil
	}

	checked := make(chan error, 1)
	go func() { checked <- e.mgr.CheckForUpdates(context.Background()) }()
	<-hit

	disabled := make(chan error, 1)
	go func() {
		c, _ := pluginRequestContext(`{"enabled":false}`, "test", "chk")
		disabled <- e.h.update(c)
	}()
	require.NoError(t, waitOrFail(t, disabled, "disable"))
	close(release)
	require.NoError(t, waitOrFail(t, checked, "update check"))

	row := e.row(t, "chk")
	assert.Equal(t, models.PluginStatusDisabled, row.Status)
	require.NotNil(t, row.UpdateAvailableVersion)
	assert.Equal(t, "2.0.0", *row.UpdateAvailableVersion)
}

// A scope or id that is not a single safe path segment is a
// 422 on every route that touches the plugin directory, before anything on
// disk is touched. The rejection happens before any disk or database
// access, so one environment serves every case: the subtests run in
// sequence and re-check the sentinel and the live plugin after each call.
func TestUnsafePluginIDsAreRejected(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	sentinel := filepath.Join(e.pluginDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o644))
	writeLive(t, e.pluginDir, lifecycleSpec{id: "test", version: "1.0.0", enricher: true})
	refs := []struct{ scope, id string }{
		{"test", ".."},
		{"..", "test"},
		{"test", "a/b"},
		{"test", `a\b`},
		{"test", "a\x00b"},
		{"test", "a\tb"},
		{"test", "a\x7fb"},
		{"test", strings.Repeat("a", maxPathSegmentLen+1)},
		{"test", ""},
	}
	routes := map[string]func(h *handler, scope, id string) error{
		"uninstall": func(h *handler, scope, id string) error {
			c, _ := pluginRequestContext("", scope, id)
			return h.uninstall(c)
		},
		"update version": func(h *handler, scope, id string) error {
			c, _ := pluginRequestContext("", scope, id)
			return h.updateVersion(c)
		},
		"reload": func(h *handler, scope, id string) error {
			c, _ := pluginRequestContext("", scope, id)
			return h.reload(c)
		},
		"patch": func(h *handler, scope, id string) error {
			c, _ := pluginRequestContext(`{"enabled":true}`, scope, id)
			return h.update(c)
		},
		"install": func(h *handler, scope, id string) error {
			body, _ := json.Marshal(map[string]string{"scope": scope, "id": id})
			c, _ := pluginRequestContext(string(body), "", "")
			return h.install(c)
		},
	}
	for name, call := range routes {
		for _, ref := range refs {
			t.Run(fmt.Sprintf("%s %q/%q", name, ref.scope, ref.id), func(t *testing.T) {
				assertErrcodeFields(t, call(e.h, ref.scope, ref.id), http.StatusUnprocessableEntity, "validation_error")
				assert.FileExists(t, sentinel)
				assert.FileExists(t, filepath.Join(e.pluginDir, "test", "test", "manifest.json"))
			})
		}
	}
}

// PATCH validates the confidence threshold before it applies anything, so
// a rejected payload does not enable the plugin.
func TestUpdate_InvalidThresholdChangesNothing(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	e.install(t, lifecycleSpec{id: "thr", version: "1.0.0", enricher: true}, models.PluginStatusDisabled, "")

	c, _ := pluginRequestContext(`{"enabled":true,"confidence_threshold":5,"config":{"k":"v"}}`, "test", "thr")
	assertErrcodeFields(t, e.h.update(c), http.StatusUnprocessableEntity, "validation_error")

	assert.Equal(t, models.PluginStatusDisabled, e.row(t, "thr").Status)
	assert.Nil(t, e.mgr.GetRuntime("test", "thr"))
	assert.Zero(t, e.count(t, "plugin_configs", "thr"))
}

// LoadAll removes staging and trash directories a crash left behind, and
// first restores a plugin whose update was interrupted between moving the
// installed version to trash and moving the new one into place.
func TestManager_LoadAllSweepsLeftovers(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	ctx := context.Background()
	// "kept" was fully replaced: its live directory is the new version, so
	// its trashed copy is garbage. "lost" crashed mid-swap: only the trashed
	// copy exists.
	e.install(t, lifecycleSpec{id: "kept", version: "2.0.0", enricher: true}, models.PluginStatusActive, "")
	e.install(t, lifecycleSpec{id: "lost", version: "1.0.0", enricher: true}, models.PluginStatusActive, "")
	scopeDir := filepath.Join(e.pluginDir, "test")
	require.NoError(t, os.MkdirAll(filepath.Join(scopeDir, trashDirName, "replaced-1"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(scopeDir, trashDirName, "replaced-2"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(scopeDir, trashDirName, "replaced-1", "kept"), 0o755))
	require.NoError(t, os.Rename(filepath.Join(scopeDir, "lost"), filepath.Join(scopeDir, trashDirName, "replaced-2", "lost")))
	require.NoError(t, os.MkdirAll(filepath.Join(scopeDir, stagingDirName, "package-abc"), 0o755))
	e.mgr.UnloadPlugin("test", "lost")

	require.NoError(t, e.mgr.LoadAll(ctx))

	assert.NoDirExists(t, filepath.Join(scopeDir, stagingDirName))
	assert.NoDirExists(t, filepath.Join(scopeDir, trashDirName))
	assert.Equal(t, "1.0.0", liveManifestVersion(t, e.pluginDir, "lost"))
	assert.Equal(t, "2.0.0", liveManifestVersion(t, e.pluginDir, "kept"))
	assert.Equal(t, models.PluginStatusActive, e.row(t, "lost").Status)
	assert.NotNil(t, e.mgr.GetRuntime("test", "lost"))
}

// The local scan skips a directory whose manifest names a different id, so
// no row points at a directory that does not exist.
func TestScan_SkipsManifestIDMismatch(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	dir := filepath.Join(e.pluginDir, "local", "folder")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, content := range (lifecycleSpec{id: "different", version: "1.0.0", enricher: true}).files() {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}

	c, rec := pluginRequestContext("", "", "")
	require.NoError(t, e.h.scan(c))
	assert.JSONEq(t, `[]`, rec.Body.String())
	plugins, err := e.svc.ListPlugins(context.Background())
	require.NoError(t, err)
	assert.Empty(t, plugins)
}

// A manifest id must be a single safe directory name, since the plugin is
// installed under it.
func TestParseManifest_RejectsUnsafeIDs(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"..", ".hidden", "a/b", `a\b`, "a\x00b", "a\nb", "a\x7fb", strings.Repeat("a", maxPathSegmentLen+1)} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(map[string]any{"manifestVersion": 1, "id": id, "name": "P", "version": "1.0.0"})
			require.NoError(t, err)
			_, err = ParseManifest(data)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "id")
		})
	}
}

// A reload whose new files do not load is a 422 that stores the error and
// keeps the previous runtime and version, instead of a 200 the UI reports
// as success. If storing the error fails, it is a 500.
func TestReload_LoadFailure(t *testing.T) {
	t.Parallel()
	for _, fault := range []bool{false, true} {
		t.Run(fmt.Sprintf("write fault %v", fault), func(t *testing.T) {
			t.Parallel()
			e := newLifecycleEnv(t)
			e.install(t, lifecycleSpec{id: "rl", version: "1.0.0", enricher: true}, models.PluginStatusActive, "")
			before := e.mgr.GetRuntime("test", "rl")
			writeLive(t, e.pluginDir, lifecycleSpec{id: "rl", version: "2.0.0", enricher: true, brokenScript: true})
			if fault {
				failPluginUpdates(t, e.db)
			}

			c, _ := pluginRequestContext("", "test", "rl")
			err := e.h.reload(c)

			row := e.row(t, "rl")
			if fault {
				assertServerFault(t, err)
				assert.Nil(t, row.LoadError)
			} else {
				assertErrcodeFields(t, err, http.StatusUnprocessableEntity, "plugin_load_failure")
				require.NotNil(t, row.LoadError)
			}
			assert.Equal(t, models.PluginStatusActive, row.Status)
			assert.Equal(t, "1.0.0", row.Version)
			assert.Same(t, before, e.mgr.GetRuntime("test", "rl"))
		})
	}
}

// Enabling a plugin that fails to load is a 422, but if its error cannot be
// stored it is a 500 that leaves the plugin Disabled and unloaded.
func TestUpdate_EnableLoadFailureWriteFaultIsServerError(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	e.install(t, lifecycleSpec{id: "en", version: "1.0.0", enricher: true, brokenScript: true}, models.PluginStatusDisabled, "")
	failPluginUpdates(t, e.db)

	c, _ := pluginRequestContext(`{"enabled":true}`, "test", "en")
	assertServerFault(t, e.h.update(c))

	row := e.row(t, "en")
	assert.Equal(t, models.PluginStatusDisabled, row.Status)
	assert.Nil(t, row.LoadError)
	assert.Nil(t, e.mgr.GetRuntime("test", "en"))
}

// The already-installed check is a 422 only for an existing row; failing to
// read the plugins table is a 500.
func TestInstall_InstalledLookupFaultIsServerError(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	_, err := e.db.Exec("DROP TABLE plugins")
	require.NoError(t, err)

	c, _ := pluginRequestContext(`{"scope":"test","id":"x"}`, "", "")
	assertServerFault(t, e.h.install(c))
}

// An update whose new version loads but cannot be stored is a 500 that
// puts the previous directory back and keeps the previous runtime and row.
// Mutates the global host allowlists.
func TestUpdateVersion_StoreFaultRollsBack(t *testing.T) {
	v1 := lifecycleSpec{id: "rb", version: "1.0.0", enricher: true, identifierTypes: true}
	pkg := pluginZip(t, lifecycleSpec{id: "rb", version: "2.0.0", enricher: true, identifierTypes: true}.files())
	ps := servePackage(t, "rb", "2.0.0", pkg, false)
	e := newLifecycleEnv(t)
	e.addRepo(t, ps.URL+"/repo.json")
	e.install(t, v1, models.PluginStatusActive, "2.0.0")
	before := e.mgr.GetRuntime("test", "rb")
	_, err := e.db.Exec("DROP TABLE plugin_identifier_types")
	require.NoError(t, err)

	c, _ := pluginRequestContext("", "test", "rb")
	assertServerFault(t, e.h.updateVersion(c))

	assert.Equal(t, "1.0.0", liveManifestVersion(t, e.pluginDir, "rb"))
	assert.Same(t, before, e.mgr.GetRuntime("test", "rb"))
	row := e.row(t, "rb")
	assert.Equal(t, "1.0.0", row.Version)
	require.NotNil(t, row.UpdateAvailableVersion)
	for _, dir := range []string{stagingDirName, trashDirName} {
		entries, _ := os.ReadDir(filepath.Join(e.pluginDir, "test", dir))
		assert.Empty(t, entries, dir)
	}
}

// installStaged checks again under the plugin's lock: a row that appeared
// after the handler's check is refused, and so is a directory with no row,
// which may be an unscanned local plugin. Neither touches the directory.
func TestInstallStaged_RefusesUnderLock(t *testing.T) {
	t.Parallel()
	stage := func(t *testing.T, e *lifecycleEnv) *stagedPackage {
		dir := filepath.Join(e.pluginDir, "test", stagingDirName, "pkg")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		files := lifecycleSpec{id: "late", version: "2.0.0", enricher: true}.files()
		for name, content := range files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
		}
		m, err := ParseManifest([]byte(files["manifest.json"]))
		require.NoError(t, err)
		return &stagedPackage{dir: dir, manifest: m}
	}

	t.Run("row exists", func(t *testing.T) {
		t.Parallel()
		e := newLifecycleEnv(t)
		e.install(t, lifecycleSpec{id: "late", version: "1.0.0", enricher: true}, models.PluginStatusActive, "")

		err := e.mgr.installStaged(context.Background(), &models.Plugin{Scope: "test", ID: "late", InstalledAt: time.Now()}, stage(t, e))
		require.ErrorIs(t, err, ErrAlreadyInstalled)
		assert.Equal(t, "1.0.0", liveManifestVersion(t, e.pluginDir, "late"))
		assert.NoDirExists(t, filepath.Join(e.pluginDir, "test", stagingDirName, "pkg"))
	})

	t.Run("directory without a row", func(t *testing.T) {
		t.Parallel()
		e := newLifecycleEnv(t)
		writeLive(t, e.pluginDir, lifecycleSpec{id: "late", version: "1.0.0", enricher: true})

		err := e.mgr.installStaged(context.Background(), &models.Plugin{Scope: "test", ID: "late", InstalledAt: time.Now()}, stage(t, e))
		require.ErrorIs(t, err, ErrDirectoryExists)
		assert.Equal(t, "1.0.0", liveManifestVersion(t, e.pluginDir, "late"))
		plugins, err := e.svc.ListPlugins(context.Background())
		require.NoError(t, err)
		assert.Empty(t, plugins)
	})
}

// Turning auto_update off while the update check is fetching repositories
// keeps the check from writing an available version.
func TestCheckForUpdates_AutoUpdateOffDuringFetch(t *testing.T) {
	t.Parallel()
	e := newLifecycleEnv(t)
	e.addRepo(t, "https://example.com/repo.json")
	e.install(t, lifecycleSpec{id: "au", version: "1.0.0", enricher: true}, models.PluginStatusActive, "")
	hit, release := make(chan struct{}, 1), make(chan struct{})
	e.mgr.fetchRepo = func(string) (*RepositoryManifest, error) {
		hit <- struct{}{}
		<-release
		return &RepositoryManifest{RepositoryVersion: 1, Scope: "test", Name: "Test", Plugins: []AvailablePlugin{{
			ID: "au", Name: "au", Versions: []PluginVersion{{Version: "2.0.0", ManifestVersion: 1, DownloadURL: "https://github.com/x.zip", SHA256: "0"}},
		}}}, nil
	}

	checked := make(chan error, 1)
	go func() { checked <- e.mgr.CheckForUpdates(context.Background()) }()
	<-hit
	c, _ := pluginRequestContext(`{"auto_update":false}`, "test", "au")
	require.NoError(t, e.h.update(c))
	close(release)
	require.NoError(t, waitOrFail(t, checked, "update check"))

	row := e.row(t, "au")
	assert.False(t, row.AutoUpdate)
	assert.Nil(t, row.UpdateAvailableVersion)
}

// stageLocal writes a staged package for test/id under the scope's staging
// directory, as stagePackage would.
func stageLocal(t *testing.T, e *lifecycleEnv, spec lifecycleSpec) *stagedPackage {
	t.Helper()
	root := filepath.Join(e.pluginDir, "test", stagingDirName)
	require.NoError(t, os.MkdirAll(root, 0o755))
	dir, err := os.MkdirTemp(root, "package-")
	require.NoError(t, err)
	files := spec.files()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	m, err := ParseManifest([]byte(files["manifest.json"]))
	require.NoError(t, err)
	return &stagedPackage{dir: dir, manifest: m}
}

// Every transition waits for the plugin's lock: while another holder has
// it, the call blocks, and it completes once the lock is released.
func TestTransitionsWaitForThePluginLock(t *testing.T) {
	t.Parallel()
	v1 := lifecycleSpec{id: "lk", version: "1.0.0", enricher: true}
	// Each transition prepares on the test goroutine and returns the call
	// to run concurrently.
	transitions := map[string]struct {
		installed bool
		status    models.PluginStatus
		prepare   func(t *testing.T, e *lifecycleEnv) func() error
	}{
		"install": {prepare: func(t *testing.T, e *lifecycleEnv) func() error {
			pkg := stageLocal(t, e, v1)
			return func() error {
				return e.mgr.installStaged(context.Background(), &models.Plugin{Scope: "test", ID: "lk", InstalledAt: time.Now()}, pkg)
			}
		}},
		"update": {installed: true, status: models.PluginStatusActive, prepare: func(t *testing.T, e *lifecycleEnv) func() error {
			pkg := stageLocal(t, e, lifecycleSpec{id: "lk", version: "2.0.0", enricher: true})
			return func() error {
				_, err := e.mgr.updateStaged(context.Background(), "test", "lk", pkg)
				return err
			}
		}},
		"reload": {installed: true, status: models.PluginStatusActive, prepare: func(_ *testing.T, e *lifecycleEnv) func() error {
			c, _ := pluginRequestContext("", "test", "lk")
			return func() error { return e.h.reload(c) }
		}},
		"enable": {installed: true, status: models.PluginStatusDisabled, prepare: func(_ *testing.T, e *lifecycleEnv) func() error {
			c, _ := pluginRequestContext(`{"enabled":true}`, "test", "lk")
			return func() error { return e.h.update(c) }
		}},
		"uninstall": {installed: true, status: models.PluginStatusActive, prepare: func(_ *testing.T, e *lifecycleEnv) func() error {
			c, _ := pluginRequestContext("", "test", "lk")
			return func() error { return e.h.uninstall(c) }
		}},
	}
	for name, tr := range transitions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLifecycleEnv(t)
			if tr.installed {
				e.install(t, v1, tr.status, "")
			}
			call := tr.prepare(t, e)
			waiting := make(chan string, 1)
			e.mgr.onLockWait = func(key string) { waiting <- key }

			unlock := e.mgr.lockPlugin("test", "lk")
			done := make(chan error, 1)
			go func() { done <- call() }()
			select {
			case key := <-waiting:
				assert.Equal(t, "test/lk", key)
			case err := <-done:
				unlock()
				t.Fatalf("%s finished without waiting for the lock: %v", name, err)
			}
			unlock()
			require.NoError(t, waitOrFail(t, done, name))
		})
	}
}
