# Plugin E2E Test Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add backend test-only infrastructure that lets Playwright E2E tests exercise the real plugin install / update / uninstall / enable-disable / config flows against a fixture plugin, without hitting the network.

**Architecture:** Define the fixture plugin's `manifest.json` and `main.js` as **Go string constants** inside `pkg/testutils/` (no committed fixture files). Expose test-only endpoints that (a) serve the fixture as a zip over localhost (built at request time from the constants), (b) directly seed plugin rows + on-disk files for flows that start from an installed state, and (c) wipe plugin state. In test mode only, add `http://127.0.0.1:` to `plugins.AllowedDownloadHosts` so the real install path accepts localhost URLs. Tests seed a repository pointing at the local `fixture.zip` and exercise the real `POST /plugins/installed` handler.

**Tech Stack:** Go, Echo, Bun, Playwright, React.

---

## File Structure

**Backend (test infra):**
- Create `pkg/testutils/plugin_fixture.go` — Go string constants (`fixtureManifestJSON`, `fixtureMainJS`) plus `fixtureFiles()` helper returning `map[string][]byte` of fixture filenames → contents
- Create `pkg/testutils/plugin_handlers.go` — handlers for seed/delete/fixture.zip/fixture-info
- Create `pkg/testutils/plugin_handlers_test.go` — Go tests for the handlers
- Modify `pkg/testutils/routes.go` — accept `*plugins.Manager` + `*plugins.Installer`, wire new routes
- Modify `pkg/server/server.go` — update caller, relax `AllowedDownloadHosts` in test mode

**E2E:**
- Modify `e2e/plugins.spec.ts` — remove the "deferred" comment header, add beforeAll helpers
- Create `e2e/plugin-install.spec.ts` — install flow
- Create `e2e/plugin-lifecycle.spec.ts` — update/uninstall/enable-disable/config flows
- Create `e2e/plugin-helpers.ts` — shared `seedPlugin`, `clearPlugins`, `seedRepository`, `loginAsPluginAdmin`

**Docs:**
- Modify `e2e/CLAUDE.md` — document the new `/test/plugins/*` endpoints

---

## Task 1: Define the fixture plugin as Go string constants

**Files:**
- Create: `pkg/testutils/plugin_fixture.go`

Rather than committing `manifest.json` and `main.js` as testdata files, keep them inline as Go string constants. The handlers in later tasks read these via a small `fixtureFiles()` helper.

Use the smallest possible valid plugin: one `metadataEnricher` capability (because `metadataEnricher` requires declared `fields`, which exercises the manifest validator) with a single declared field.

- [ ] **Step 1: Write the fixture file**

Write `pkg/testutils/plugin_fixture.go`:

```go
package testutils

// Fixture plugin used by E2E tests. Kept as Go string constants (rather than
// a committed testdata directory) so the plugin source stays alongside the
// handlers that build a zip from it and seed it to disk.

const (
	fixtureScope   = "test"
	fixtureID      = "fixture"
	fixtureVersion = "1.0.0"
	fixtureName    = "Fixture Plugin"
)

const fixtureManifestJSON = `{
  "manifestVersion": 1,
  "id": "fixture",
  "name": "Fixture Plugin",
  "version": "1.0.0",
  "description": "Test-only plugin used by E2E tests. Do not ship to users.",
  "homepage": "https://example.test/fixture",
  "capabilities": {
    "metadataEnricher": {
      "description": "E2E fixture enricher",
      "fileTypes": ["epub"],
      "fields": ["title"]
    }
  },
  "configSchema": {
    "apiKey": {
      "type": "string",
      "label": "API Key",
      "description": "Not actually used by the fixture",
      "required": false,
      "secret": true
    }
  }
}
`

const fixtureMainJS = `var plugin = (function () {
  return {
    metadataEnricher: {
      search: function () {
        return { results: [] };
      }
    }
  };
})();
`

// fixtureFiles returns the fixture plugin's on-disk file layout: map keyed
// by filename (no directories, matching the flat zip layout the installer
// expects) to the raw bytes.
func fixtureFiles() map[string][]byte {
	return map[string][]byte{
		"manifest.json": []byte(fixtureManifestJSON),
		"main.js":       []byte(fixtureMainJS),
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./pkg/testutils/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add pkg/testutils/plugin_fixture.go
git commit -m "[Test] Add fixture plugin source constants for E2E test infra"
```

---

## Task 2: Add `/test/plugins/fixture.zip` and `/test/plugins/fixture-info` handlers

**Files:**
- Create: `pkg/testutils/plugin_handlers.go`
- Create: `pkg/testutils/plugin_handlers_test.go`

These handlers serve the fixture plugin over HTTP so the real install flow (which downloads a zip and verifies SHA256) can be exercised against localhost. The zip is rebuilt on every request (simpler than caching), but the bytes are deterministic because `archive/zip` writes a fixed timestamp when we pass `time.Time{}`.

- [ ] **Step 1: Write the failing test**

Write `pkg/testutils/plugin_handlers_test.go`:

```go
package testutils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureZipIsDeterministicAndMatchesInfo(t *testing.T) {
	t.Parallel()

	e := echo.New()
	h := &handler{} // db not needed for fixture endpoints
	e.GET("/test/plugins/fixture.zip", h.fixtureZip)
	e.GET("/test/plugins/fixture-info", h.fixtureInfo)

	// Fetch the zip
	req := httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/zip", rec.Header().Get("Content-Type"))

	zipBytes, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	require.NotEmpty(t, zipBytes)

	h1 := sha256.Sum256(zipBytes)

	// Fetch it again: bytes must be identical (deterministic build).
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture.zip", nil))
	h2 := sha256.Sum256(rec2.Body.Bytes())
	assert.Equal(t, h1, h2, "fixture.zip must be deterministic across requests")

	// Fetch info and check the sha256 matches what we got from fixture.zip.
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/test/plugins/fixture-info", nil))
	require.Equal(t, http.StatusOK, rec3.Code)

	var info struct {
		Scope       string `json:"scope"`
		ID          string `json:"id"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		SHA256      string `json:"sha256"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &info))

	assert.Equal(t, "test", info.Scope)
	assert.Equal(t, "fixture", info.ID)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, hex.EncodeToString(h1[:]), info.SHA256)
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./pkg/testutils/... -run TestFixtureZipIsDeterministicAndMatchesInfo -v
```

Expected: FAIL with compilation errors (`handler` has no `fixtureZip`/`fixtureInfo` method).

- [ ] **Step 3: Write the implementation**

Write `pkg/testutils/plugin_handlers.go`:

```go
package testutils

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

// buildFixtureZip produces a deterministic zip of the fixture plugin files and
// returns the bytes plus their SHA256. The zip stores entries with a fixed
// modtime so repeat calls produce identical bytes.
func buildFixtureZip() ([]byte, string, error) {
	files := fixtureFiles()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.Modified = fixedTime
		w, err := zw.CreateHeader(fh)
		if err != nil {
			return nil, "", errors.Wrapf(err, "create zip entry %s", name)
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, "", errors.Wrapf(err, "write zip entry %s", name)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", errors.Wrap(err, "close zip")
	}

	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

// fixtureZip serves the fixture plugin as a zip.
// GET /test/plugins/fixture.zip
func (h *handler) fixtureZip(c echo.Context) error {
	data, _, err := buildFixtureZip()
	if err != nil {
		return errors.WithStack(err)
	}
	c.Response().Header().Set("Content-Type", "application/zip")
	_, err = io.Copy(c.Response().Writer, bytes.NewReader(data))
	return errors.WithStack(err)
}

type fixtureInfoResponse struct {
	Scope       string `json:"scope"`
	ID          string `json:"id"`
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

// fixtureInfo returns metadata about the fixture plugin.
// GET /test/plugins/fixture-info
func (h *handler) fixtureInfo(c echo.Context) error {
	_, sum, err := buildFixtureZip()
	if err != nil {
		return errors.WithStack(err)
	}
	scheme := "http"
	if c.Request().TLS != nil {
		scheme = "https"
	}
	downloadURL := scheme + "://" + c.Request().Host + "/test/plugins/fixture.zip"
	return c.JSON(http.StatusOK, fixtureInfoResponse{
		Scope:       fixtureScope,
		ID:          fixtureID,
		Version:     fixtureVersion,
		DownloadURL: downloadURL,
		SHA256:      sum,
	})
}
```

- [ ] **Step 4: Run the test**

```bash
go test ./pkg/testutils/... -run TestFixtureZipIsDeterministicAndMatchesInfo -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/testutils/plugin_handlers.go pkg/testutils/plugin_handlers_test.go
git commit -m "[Test] Serve plugin fixture zip from testutils"
```

---

## Task 3: Register the two fixture routes

**Files:**
- Modify: `pkg/testutils/routes.go`

These two routes do not depend on the plugin manager or installer, so register them with the existing `RegisterRoutes(e, db)` signature. Task 4 extends the signature later.

- [ ] **Step 1: Wire the routes**

Edit `pkg/testutils/routes.go`:

```go
func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	h := &handler{db: db}

	test := e.Group("/test")
	test.POST("/users", h.createUser)
	test.DELETE("/users", h.deleteAllUsers)

	// eReader test data endpoints
	test.POST("/libraries", h.createLibrary)
	test.POST("/books", h.createBook)
	test.POST("/persons", h.createPerson)
	test.POST("/series", h.createSeries)
	test.POST("/api-keys", h.createAPIKey)
	test.DELETE("/ereader", h.deleteAllEReaderData)

	// Plugin fixture endpoints — serve the fixture plugin as a zip so the
	// real install flow can download it from localhost during E2E tests.
	test.GET("/plugins/fixture.zip", h.fixtureZip)
	test.GET("/plugins/fixture-info", h.fixtureInfo)
}
```

- [ ] **Step 2: Smoke test via Go tests**

```bash
go test ./pkg/testutils/... -v
```

Expected: all pass (routes compile, existing tests unaffected).

- [ ] **Step 3: Commit**

```bash
git add pkg/testutils/routes.go
git commit -m "[Test] Register plugin fixture routes"
```

---

## Task 4: Plumb `*plugins.Manager` and `*plugins.Installer` into testutils

**Files:**
- Modify: `pkg/testutils/routes.go`
- Modify: `pkg/testutils/handlers.go` (add fields to `handler` struct)
- Modify: `pkg/server/server.go`

Seed + delete handlers (Task 5) need to (a) write plugin files to the plugin directory, and (b) call `manager.LoadPlugin` / `manager.UnloadPlugin` so the in-memory runtime stays consistent with DB state. Both require access to the configured plugin dir and manager.

- [ ] **Step 1: Extend the handler struct**

In `pkg/testutils/handlers.go`, change:

```go
type handler struct {
	db *bun.DB
}
```

to:

```go
type handler struct {
	db        *bun.DB
	manager   *plugins.Manager
	installer *plugins.Installer
}
```

Add the import:

```go
"github.com/shishobooks/shisho/pkg/plugins"
```

- [ ] **Step 2: Extend `RegisterRoutes`**

In `pkg/testutils/routes.go`:

```go
import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB, manager *plugins.Manager, installer *plugins.Installer) {
	h := &handler{db: db, manager: manager, installer: installer}
	// ... existing route registrations unchanged ...
}
```

- [ ] **Step 3: Update the caller**

In `pkg/server/server.go`, around line 69-71, change:

```go
if cfg.IsTestMode() {
	testutils.RegisterRoutes(e, db)
}
```

to:

```go
if cfg.IsTestMode() {
	testutils.RegisterRoutes(e, db, pm, plugins.NewInstaller(cfg.PluginDir))
}
```

Note: `pm` (the manager) is already a parameter of `server.New`. `plugins.NewInstaller` is cheap to instantiate — we do not share the one constructed later at line 226 because initialization order would complicate the split.

- [ ] **Step 4: Update the failing test**

Edit `pkg/testutils/plugin_handlers_test.go` — the existing Task 2 test constructs `&handler{}` directly, which still works because all new fields are nilable. No change needed, but if `go vet` complains about unkeyed fields, make the literal keyed.

- [ ] **Step 5: Run the full Go build**

```bash
mise check:quiet
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/testutils/routes.go pkg/testutils/handlers.go pkg/server/server.go
git commit -m "[Test] Plumb plugin manager and installer into testutils"
```

---

## Task 5: Add `POST /test/plugins` seed handler

**Files:**
- Modify: `pkg/testutils/plugin_handlers.go`
- Modify: `pkg/testutils/plugin_handlers_test.go`
- Modify: `pkg/testutils/routes.go`

The seed handler writes the fixture plugin files to `{pluginDir}/{scope}/{id}/`, inserts a `plugins` row with caller-controlled fields (status, update_available_version, etc.), and calls `manager.LoadPlugin` when the caller requests it. This gives tests deterministic "installed plugin" starting states without going through the download path.

- [ ] **Step 1: Write the failing test**

Append to `pkg/testutils/plugin_handlers_test.go`:

```go
func TestSeedPluginWritesDBRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tdb := testdb.New(t) // existing helper — see pkg/testdb usage in other test files
	defer tdb.Close()

	tmp := t.TempDir()
	installer := plugins.NewInstaller(tmp)
	e := echo.New()
	e.Binder = &echo.DefaultBinder{}
	h := &handler{db: tdb.DB, manager: nil, installer: installer}
	e.POST("/test/plugins", h.seedPlugin)

	body := `{
		"scope": "test",
		"id": "fixture",
		"name": "Fixture Plugin",
		"version": "1.0.0",
		"status": 0,
		"update_available_version": "2.0.0"
	}`
	req := httptest.NewRequest(http.MethodPost, "/test/plugins", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	// DB row exists
	var count int
	err := tdb.DB.NewSelect().Model((*models.Plugin)(nil)).
		Where("scope = ? AND id = ?", "test", "fixture").
		ColumnExpr("count(*)").Scan(ctx, &count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Files exist on disk
	assert.FileExists(t, filepath.Join(tmp, "test", "fixture", "manifest.json"))
	assert.FileExists(t, filepath.Join(tmp, "test", "fixture", "main.js"))
}
```

Check which `testdb` / DB test helper is used in `pkg/plugins/service_test.go`; imitate it here. If `pkg/testutils` does not yet import `testdb`, use the same pattern as `pkg/plugins/service_test.go:TestMain` for DB setup (inspect that file and mirror).

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./pkg/testutils/... -run TestSeedPluginWritesDBRow -v
```

Expected: FAIL (method not defined).

- [ ] **Step 3: Implement the handler**

Append to `pkg/testutils/plugin_handlers.go`:

```go
type seedPluginRequest struct {
	Scope                  string  `json:"scope" validate:"required"`
	ID                     string  `json:"id" validate:"required"`
	Name                   string  `json:"name"`
	Version                string  `json:"version"`
	Status                 int     `json:"status"` // matches models.PluginStatus (0=active, -1=disabled, -2=malfunctioned, -3=notsupported)
	UpdateAvailableVersion *string `json:"update_available_version"`
	RepositoryScope        *string `json:"repository_scope"`
	RepositoryURL          *string `json:"repository_url"`
	SkipLoad               bool    `json:"skip_load"` // default false: call manager.LoadPlugin after seeding
}

// seedPlugin seeds an installed plugin for tests. Writes the fixture plugin
// files to disk under {pluginDir}/{scope}/{id}/ and inserts a plugins row with
// the caller-specified metadata. Optionally calls manager.LoadPlugin so the
// runtime is available.
// POST /test/plugins
func (h *handler) seedPlugin(c echo.Context) error {
	ctx := c.Request().Context()

	var req seedPluginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	if req.Scope == "" || req.ID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "scope and id are required")
	}

	name := req.Name
	if name == "" {
		name = "Fixture Plugin"
	}
	version := req.Version
	if version == "" {
		version = fixtureVersion
	}

	// Write fixture files to {pluginDir}/{scope}/{id}/
	destDir := filepath.Join(h.installer.PluginDir(), req.Scope, req.ID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return errors.Wrap(err, "create plugin dir")
	}
	for name, data := range fixtureFiles() {
		if err := os.WriteFile(filepath.Join(destDir, name), data, 0644); err != nil {
			return errors.Wrapf(err, "write %s", name)
		}
	}

	// Insert DB row
	plugin := &models.Plugin{
		Scope:                  req.Scope,
		ID:                     req.ID,
		Name:                   name,
		Version:                version,
		Status:                 models.PluginStatus(req.Status),
		AutoUpdate:             true,
		InstalledAt:            time.Now(),
		UpdateAvailableVersion: req.UpdateAvailableVersion,
		RepositoryScope:        req.RepositoryScope,
		RepositoryURL:          req.RepositoryURL,
	}
	if _, err := h.db.NewInsert().Model(plugin).Exec(ctx); err != nil {
		return errors.Wrap(err, "insert plugin row")
	}

	// Load the runtime unless told not to
	if !req.SkipLoad && h.manager != nil && plugin.Status == models.PluginStatusActive {
		if err := h.manager.LoadPlugin(ctx, req.Scope, req.ID); err != nil {
			// Store load error on the row but don't fail the request —
			// the test may intentionally be seeding a broken state.
			msg := err.Error()
			plugin.LoadError = &msg
			_, _ = h.db.NewUpdate().Model(plugin).WherePK().Exec(ctx)
		}
	}

	return c.JSON(http.StatusCreated, plugin)
}
```

Add imports at the top: `"os"`, `"path/filepath"`, `"time"`, `"github.com/shishobooks/shisho/pkg/models"`.

- [ ] **Step 4: Register the route**

In `pkg/testutils/routes.go`, under the plugin fixture block:

```go
test.POST("/plugins", h.seedPlugin)
```

- [ ] **Step 5: Run the test**

```bash
go test ./pkg/testutils/... -run TestSeedPluginWritesDBRow -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/testutils/plugin_handlers.go pkg/testutils/plugin_handlers_test.go pkg/testutils/routes.go
git commit -m "[Test] Seed plugin rows directly from testutils"
```

---

## Task 6: Add `DELETE /test/plugins` teardown handler

**Files:**
- Modify: `pkg/testutils/plugin_handlers.go`
- Modify: `pkg/testutils/plugin_handlers_test.go`
- Modify: `pkg/testutils/routes.go`

Wipes plugin state: unloads every loaded runtime, removes every subdirectory of `pluginDir`, truncates `plugins`, `plugin_configs`, `plugin_hook_configs`, `library_plugin_hook_configs`, `library_plugin_customizations`, `plugin_field_settings`, `library_plugin_field_settings`, `plugin_identifier_types`, and `plugin_repositories`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/testutils/plugin_handlers_test.go`:

```go
func TestDeleteAllPluginsWipesStateAndDisk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tdb := testdb.New(t)
	defer tdb.Close()

	tmp := t.TempDir()
	// Seed a fake plugin dir
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "test", "fixture"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "test", "fixture", "x.txt"), []byte("x"), 0644))

	// Seed a DB row
	_, err := tdb.DB.NewInsert().Model(&models.Plugin{
		Scope: "test", ID: "fixture", Name: "F", Version: "1.0.0",
		Status: models.PluginStatusActive, InstalledAt: time.Now(),
	}).Exec(ctx)
	require.NoError(t, err)

	installer := plugins.NewInstaller(tmp)
	e := echo.New()
	h := &handler{db: tdb.DB, manager: nil, installer: installer}
	e.DELETE("/test/plugins", h.deleteAllPlugins)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/test/plugins", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// Row gone
	var count int
	err = tdb.DB.NewSelect().Model((*models.Plugin)(nil)).
		ColumnExpr("count(*)").Scan(ctx, &count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Directory gone
	_, err = os.Stat(filepath.Join(tmp, "test"))
	assert.True(t, os.IsNotExist(err), "plugin dir should be removed")
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./pkg/testutils/... -run TestDeleteAllPluginsWipesStateAndDisk -v
```

Expected: FAIL (method not defined).

- [ ] **Step 3: Implement the handler**

Append to `pkg/testutils/plugin_handlers.go`:

```go
type deleteAllPluginsResponse struct {
	Plugins      int `json:"plugins"`
	Repositories int `json:"repositories"`
}

// deleteAllPlugins wipes all plugin state: unloads runtimes, removes the
// plugin directory contents, and truncates all plugin-related tables.
// DELETE /test/plugins
func (h *handler) deleteAllPlugins(c echo.Context) error {
	ctx := c.Request().Context()
	var resp deleteAllPluginsResponse

	// Unload runtimes for every currently-installed plugin.
	if h.manager != nil {
		existing := make([]*models.Plugin, 0)
		if err := h.db.NewSelect().Model(&existing).Scan(ctx); err == nil {
			for _, p := range existing {
				h.manager.UnloadPlugin(p.Scope, p.ID)
			}
		}
	}

	// Truncate in FK-safe order: child tables first.
	_, _ = h.db.NewDelete().Model((*models.LibraryPluginFieldSetting)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.PluginFieldSetting)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.LibraryPluginHookConfig)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.LibraryPluginCustomization)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.PluginHookConfig)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.PluginIdentifierType)(nil)).Where("1=1").Exec(ctx)
	_, _ = h.db.NewDelete().Model((*models.PluginConfig)(nil)).Where("1=1").Exec(ctx)

	result, _ := h.db.NewDelete().Model((*models.Plugin)(nil)).Where("1=1").Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.Plugins = int(n)
	}

	// Repositories: only delete non-official ones to match RemoveRepository
	// semantics. Tests that need a clean slate (including official) can call
	// the endpoint with ?include_official=true.
	includeOfficial := c.QueryParam("include_official") == "true"
	delQ := h.db.NewDelete().Model((*models.PluginRepository)(nil))
	if !includeOfficial {
		delQ = delQ.Where("is_official = ?", false)
	} else {
		delQ = delQ.Where("1=1")
	}
	result, _ = delQ.Exec(ctx)
	if n, _ := result.RowsAffected(); n > 0 {
		resp.Repositories = int(n)
	}

	// Wipe the plugin directory (everything under pluginDir).
	if h.installer != nil {
		dir := h.installer.PluginDir()
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
			}
		}
	}

	return c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 4: Register the route**

In `pkg/testutils/routes.go`:

```go
test.DELETE("/plugins", h.deleteAllPlugins)
```

- [ ] **Step 5: Run the test**

```bash
go test ./pkg/testutils/... -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/testutils/plugin_handlers.go pkg/testutils/plugin_handlers_test.go pkg/testutils/routes.go
git commit -m "[Test] Add DELETE /test/plugins teardown handler"
```

---

## Task 7: Relax `AllowedDownloadHosts` in test mode

**Files:**
- Modify: `pkg/server/server.go`

In test mode, install URLs will be `http://127.0.0.1:{port}/test/plugins/fixture.zip` — not a GitHub URL. `plugins.AllowedDownloadHosts` is a package `var`, so we append a localhost prefix in test mode before any routes are registered.

- [ ] **Step 1: Update server.go**

In `pkg/server/server.go`, update the test-mode block around line 69:

```go
if cfg.IsTestMode() {
	// Allow localhost download URLs so E2E tests can install the fixture
	// plugin from /test/plugins/fixture.zip. Safe because these hosts are
	// only added in test mode (ENVIRONMENT=test).
	plugins.AllowedDownloadHosts = append(plugins.AllowedDownloadHosts,
		"http://127.0.0.1:",
		"http://localhost:",
	)
	testutils.RegisterRoutes(e, db, pm, plugins.NewInstaller(cfg.PluginDir))
}
```

- [ ] **Step 2: Verify via unit test**

Run:

```bash
go test ./pkg/plugins/... -run TestIsAllowedDownloadURL -v
```

If this test does not exist, add it in `pkg/plugins/installer_test.go`:

```go
func TestIsAllowedDownloadURL_AcceptsLocalhostWhenConfigured(t *testing.T) {
	t.Parallel()
	orig := AllowedDownloadHosts
	defer func() { AllowedDownloadHosts = orig }()
	AllowedDownloadHosts = append(orig, "http://127.0.0.1:")

	assert.True(t, isAllowedDownloadURL("http://127.0.0.1:9876/test/plugins/fixture.zip"))
	assert.False(t, isAllowedDownloadURL("http://evil.example.com/x.zip"))
}
```

Run:

```bash
go test ./pkg/plugins/... -run TestIsAllowedDownloadURL -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/server/server.go pkg/plugins/installer_test.go
git commit -m "[Test] Allow localhost plugin downloads in test mode"
```

---

## Task 8: E2E helpers

**Files:**
- Create: `e2e/plugin-helpers.ts`

Shared helpers the three spec files below all call into. Keep these small and explicit — repeating three lines in three spec files is worse than an over-broad helper that hides the endpoints.

- [ ] **Step 1: Write the helpers**

Write `e2e/plugin-helpers.ts`:

```typescript
import type { APIRequestContext, Page } from "@playwright/test";

import { expect } from "./fixtures";

export const PLUGIN_TEST_USERNAME = "plugintest";
export const PLUGIN_TEST_PASSWORD = "password123";

export interface FixtureInfo {
  scope: string;
  id: string;
  version: string;
  download_url: string;
  sha256: string;
}

export async function getFixtureInfo(
  api: APIRequestContext,
): Promise<FixtureInfo> {
  const resp = await api.get("/test/plugins/fixture-info");
  expect(resp.ok()).toBeTruthy();
  return resp.json();
}

export interface SeedPluginBody {
  scope: string;
  id: string;
  name?: string;
  version?: string;
  status?: number; // 0 active, -1 disabled, -2 malfunctioned, -3 unsupported
  update_available_version?: string;
  repository_scope?: string;
  repository_url?: string;
  skip_load?: boolean;
}

export async function seedPlugin(
  api: APIRequestContext,
  body: SeedPluginBody,
): Promise<void> {
  const resp = await api.post("/test/plugins", { data: body });
  expect(resp.status()).toBe(201);
}

export async function clearPlugins(api: APIRequestContext): Promise<void> {
  const resp = await api.delete("/test/plugins?include_official=true");
  expect(resp.ok()).toBeTruthy();
}

export async function loginAsPluginAdmin(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Username").fill(PLUGIN_TEST_USERNAME);
  await page.getByLabel("Password").fill(PLUGIN_TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL("/settings/libraries");
}

export async function resetAndLogin(
  api: APIRequestContext,
  page: Page,
): Promise<void> {
  await clearPlugins(api);
  await api.delete("/test/ereader");
  await api.delete("/test/users");
  await api.post("/test/users", {
    data: {
      username: PLUGIN_TEST_USERNAME,
      password: PLUGIN_TEST_PASSWORD,
    },
  });
  await loginAsPluginAdmin(page);
}
```

- [ ] **Step 2: Commit**

```bash
git add e2e/plugin-helpers.ts
git commit -m "[E2E] Add shared plugin test helpers"
```

---

## Task 9: E2E install flow

**Files:**
- Create: `e2e/plugin-install.spec.ts`

Exercises the real `POST /plugins/installed` handler. The test adds a custom repository whose `repository.json` is served by... wait — we do not yet have a repository.json endpoint. Two paths:

1. **Add a fixture repository endpoint** (`GET /test/plugins/repository.json`) and rely on `POST /plugins/repositories/:scope/sync` to pull it in.
2. **Install via `{downloadURL, sha256}` payload** — the install handler accepts this form and skips the repository lookup entirely.

Option 2 is simpler and still exercises the full install code path (download, SHA256 verify, unzip, manifest parse, LoadPlugin, DB insert). Use it.

- [ ] **Step 1: Write the spec**

Write `e2e/plugin-install.spec.ts`:

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  clearPlugins,
  getFixtureInfo,
  loginAsPluginAdmin,
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
} from "./plugin-helpers";

test.describe("Plugin install flow", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const api = await request.newContext({ baseURL: apiBaseURL });
    await clearPlugins(api);
    await api.delete("/test/ereader");
    await api.delete("/test/users");
    await api.post("/test/users", {
      data: {
        username: PLUGIN_TEST_USERNAME,
        password: PLUGIN_TEST_PASSWORD,
      },
    });
    await api.dispose();
  });

  test.beforeEach(async ({ apiContext }) => {
    await clearPlugins(apiContext);
  });

  test("installing from a direct URL adds the plugin to the Installed tab", async ({
    apiContext,
    page,
  }) => {
    const info = await getFixtureInfo(apiContext);
    await loginAsPluginAdmin(page);

    // Call the install API directly (the Discover tab needs a seeded
    // repository to list this plugin; that is covered by the next test).
    const installResp = await apiContext.post("/plugins/installed", {
      data: {
        scope: info.scope,
        id: info.id,
        name: "Fixture Plugin",
        version: info.version,
        download_url: info.download_url,
        sha256: info.sha256,
      },
    });
    expect(installResp.status()).toBe(201);

    await page.goto("/settings/plugins");
    await expect(
      page.getByRole("link", { name: /Fixture Plugin/ }),
    ).toBeVisible();
  });

  test("clicking Install from Discover adds the plugin to Installed", async ({
    apiContext,
    page,
  }) => {
    const info = await getFixtureInfo(apiContext);

    // Seed a repository whose single version points at our fixture zip.
    // This requires a helper endpoint that serves the repository manifest
    // OR we insert a pre-synced repository directly. For simplicity, we
    // mock the Discover list by seeding an installed-and-uninstalled
    // round-trip path: install → immediately uninstall → verify Discover
    // now shows the plugin as installable.
    //
    // Repository-backed Discover is deferred to a follow-up; the primary
    // value here is that the UI Install button wires to the install
    // mutation, which is covered by the direct-URL test above.
    test.skip(
      true,
      "Discover-backed install requires a /test/plugins/repository endpoint (follow-up task).",
    );
    expect(info.version).toBeTruthy(); // keep the unused var referenced
    await loginAsPluginAdmin(page);
  });
});
```

**Note:** The second test is intentionally skipped — the primary install button test needs a seeded repository to show the fixture in Discover. If the team wants that too, a follow-up task can add `GET /test/plugins/repository.json` and a helper that inserts + syncs a test repository. The test here documents the gap explicitly rather than leaving it undiscovered.

- [ ] **Step 2: Run the E2E spec**

Ensure servers are up (Playwright auto-starts them). Then:

```bash
mise test:e2e -- e2e/plugin-install.spec.ts
```

Expected: the first test PASSES, the second is skipped.

- [ ] **Step 3: Commit**

```bash
git add e2e/plugin-install.spec.ts
git commit -m "[E2E] Add plugin install flow test"
```

---

## Task 10: E2E uninstall / update / enable-disable flows

**Files:**
- Create: `e2e/plugin-lifecycle.spec.ts`

These flows all start from an *already-installed* state — use `seedPlugin` to set up each test.

- [ ] **Step 1: Write the spec**

Write `e2e/plugin-lifecycle.spec.ts`:

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  clearPlugins,
  loginAsPluginAdmin,
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
  seedPlugin,
} from "./plugin-helpers";

test.describe("Plugin lifecycle flows", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const api = await request.newContext({ baseURL: apiBaseURL });
    await clearPlugins(api);
    await api.delete("/test/ereader");
    await api.delete("/test/users");
    await api.post("/test/users", {
      data: {
        username: PLUGIN_TEST_USERNAME,
        password: PLUGIN_TEST_PASSWORD,
      },
    });
    await api.dispose();
  });

  test.beforeEach(async ({ apiContext }) => {
    await clearPlugins(apiContext);
  });

  test("uninstall removes the plugin from the Installed list", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins/test/fixture");

    // Scroll to / open the danger zone and click uninstall.
    await page.getByRole("button", { name: /Uninstall/ }).click();
    // Confirmation dialog: confirm.
    await page
      .getByRole("dialog")
      .getByRole("button", { name: /Uninstall/ })
      .click();

    // Redirected back to the Installed list; plugin no longer present.
    await expect(page).toHaveURL(/\/settings\/plugins$/);
    await expect(
      page.getByText(
        "No plugins installed yet. Browse available plugins to get started.",
      ),
    ).toBeVisible();
  });

  test("update applies the new version and clears the update pill", async ({
    apiContext,
    page,
  }) => {
    // Seed with an update pending. The /plugins/installed/:scope/:id/update
    // handler performs the real update flow, so it needs a valid download
    // URL + sha. We patch the plugin after the page renders by calling
    // the update endpoint ourselves with fixture info.
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
      version: "0.9.0",
      update_available_version: "1.0.0",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins");

    // Update pill is visible.
    await expect(
      page.getByRole("button", { name: "Update" }),
    ).toBeVisible();

    // The real updateVersion handler looks up the repository version. That
    // path requires a seeded repository; the direct-URL update path is not
    // exposed from the UI. Skip this test until repository seeding lands.
    test.skip(
      true,
      "Update-via-UI requires a seeded repository (follow-up task).",
    );
  });

  test("disabling a plugin marks it disabled and re-enabling restores active", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins/test/fixture");

    // Click Disable on the detail page.
    await page.getByRole("button", { name: /Disable/ }).click();

    // After disabling, the button flips to "Enable".
    await expect(page.getByRole("button", { name: /Enable/ })).toBeVisible();

    // Back to the list: plugin is in the disabled section.
    await page.goto("/settings/plugins");
    const row = page.getByRole("link", { name: /Fixture Plugin/ });
    await expect(row).toBeVisible();

    // Re-enable and confirm.
    await page.goto("/settings/plugins/test/fixture");
    await page.getByRole("button", { name: /Enable/ }).click();
    await expect(page.getByRole("button", { name: /Disable/ })).toBeVisible();
  });
});
```

**Note:** The update-via-UI test is skipped for the same reason as Discover-backed install: the UI path hits the repository-lookup code. A follow-up can add a test-only repository endpoint.

- [ ] **Step 2: Run the spec**

```bash
mise test:e2e -- e2e/plugin-lifecycle.spec.ts
```

Expected: uninstall and enable/disable PASS; update test is skipped.

- [ ] **Step 3: Commit**

```bash
git add e2e/plugin-lifecycle.spec.ts
git commit -m "[E2E] Add plugin uninstall and enable/disable tests"
```

---

## Task 11: E2E config save flow

**Files:**
- Create: `e2e/plugin-config.spec.ts`

The Notion task's "Reload / config" bullet conflates two things: (a) UI config save works and round-trips, and (b) backend hot-reloads the plugin so next invocation picks up new config. Only (a) is observable from the UI. Scope this test to (a).

- [ ] **Step 1: Write the spec**

Write `e2e/plugin-config.spec.ts`:

```typescript
import { expect, getApiBaseURL, request, test } from "./fixtures";
import {
  clearPlugins,
  loginAsPluginAdmin,
  PLUGIN_TEST_PASSWORD,
  PLUGIN_TEST_USERNAME,
  seedPlugin,
} from "./plugin-helpers";

test.describe("Plugin config save", () => {
  test.beforeAll(async ({ browser }) => {
    const apiBaseURL = getApiBaseURL(browser.browserType().name());
    const api = await request.newContext({ baseURL: apiBaseURL });
    await clearPlugins(api);
    await api.delete("/test/ereader");
    await api.delete("/test/users");
    await api.post("/test/users", {
      data: {
        username: PLUGIN_TEST_USERNAME,
        password: PLUGIN_TEST_PASSWORD,
      },
    });
    await api.dispose();
  });

  test.beforeEach(async ({ apiContext }) => {
    await clearPlugins(apiContext);
  });

  test("editing a config field saves and round-trips", async ({
    apiContext,
    page,
  }) => {
    await seedPlugin(apiContext, {
      scope: "test",
      id: "fixture",
      name: "Fixture Plugin",
    });

    await loginAsPluginAdmin(page);
    await page.goto("/settings/plugins/test/fixture");

    // The fixture has a single secret config field "apiKey".
    const input = page.getByLabel("API Key");
    await input.fill("test-secret-value");
    await page.getByRole("button", { name: /Save/ }).click();

    // Assert the save succeeded: the value round-trips via the API.
    const resp = await apiContext.get(
      "/plugins/installed/test/fixture/config",
    );
    expect(resp.ok()).toBeTruthy();
    const body = await resp.json();
    // Secret values are masked on read — we only assert the key exists.
    expect(body.values).toHaveProperty("apiKey");
  });
});
```

- [ ] **Step 2: Run the spec**

```bash
mise test:e2e -- e2e/plugin-config.spec.ts
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add e2e/plugin-config.spec.ts
git commit -m "[E2E] Add plugin config save test"
```

---

## Task 12: Remove the "deferred" comment from the existing spec

**Files:**
- Modify: `e2e/plugins.spec.ts`

The top-of-file comment claims the deferred flows are not covered. Update it to reference the new spec files.

- [ ] **Step 1: Edit the comment**

In `e2e/plugins.spec.ts`, replace the multi-line comment header (lines 1-19) with:

```typescript
/**
 * E2E tests for the redesigned Plugins admin UI (/settings/plugins).
 *
 * This file covers UI-structural behavior that does not require plugin
 * seeding: routing, tab selection, legacy redirects, empty-state copy.
 *
 * Flows that require plugin seeding live in:
 *   - e2e/plugin-install.spec.ts   (install)
 *   - e2e/plugin-lifecycle.spec.ts (uninstall, enable/disable, update)
 *   - e2e/plugin-config.spec.ts    (config save)
 *
 * Running:
 *   pnpm test:e2e                        # Run all E2E tests
 *   pnpm test:e2e e2e/plugins.spec.ts    # Run only this file
 */
```

- [ ] **Step 2: Commit**

```bash
git add e2e/plugins.spec.ts
git commit -m "[E2E] Point plugins.spec.ts header to the new lifecycle specs"
```

---

## Task 13: Document the new endpoints in e2e/CLAUDE.md

**Files:**
- Modify: `e2e/CLAUDE.md`

Extend the "Test-Only API Endpoints" table to cover the new plugin endpoints.

- [ ] **Step 1: Extend the table**

In `e2e/CLAUDE.md`, find the section:

```
### Available Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/test/users` | POST | Create a test user with admin role |
| `/test/users` | DELETE | Delete all users |
```

Replace with:

```
### Available Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/test/users` | POST | Create a test user with admin role |
| `/test/users` | DELETE | Delete all users |
| `/test/ereader` | DELETE | Wipe all eReader test data |
| `/test/plugins` | POST | Seed a plugin (disk + DB) |
| `/test/plugins` | DELETE | Wipe all plugin state (add `?include_official=true` to also wipe official repos) |
| `/test/plugins/fixture.zip` | GET | Fixture plugin zipped for install flows |
| `/test/plugins/fixture-info` | GET | `{scope, id, version, download_url, sha256}` for the fixture |
```

- [ ] **Step 2: Commit**

```bash
git add e2e/CLAUDE.md
git commit -m "[Docs] Document /test/plugins endpoints in e2e/CLAUDE.md"
```

---

## Task 14: Full check

- [ ] **Step 1: Run everything**

```bash
mise check:quiet
```

Expected: PASS (Go tests, lint, JS tests, E2E tests all green).

- [ ] **Step 2: If anything fails, triage**

- Go test failures → fix in the relevant handler/test file
- Lint failures → `mise lint` to see the specific errors
- E2E flake → re-run once; if persistent, use Playwright's `--debug` or `--ui` mode locally

---

## Out of scope / Follow-up tasks

1. **Discover-backed install E2E test** — requires a test-only `GET /test/plugins/repository.json` handler plus a seed endpoint that inserts a `plugin_repositories` row pointing at the local manifest URL. Two of the specs above have `test.skip(...)` calls that document this gap.
2. **Update-via-UI test** — same blocker as #1. The UI's Update button calls `POST /plugins/installed/:scope/:id/update`, which does a repository lookup for the target version.
3. **"Reload without server restart" observability** — inherently not testable from the UI. Belongs in backend integration tests in `pkg/plugins/`.
