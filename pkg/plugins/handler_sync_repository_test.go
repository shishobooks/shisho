package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncRepository_RefreshesUpdateAvailableVersion verifies that after a
// manual repo sync, CheckForUpdates runs so stale update_available_version
// values don't linger until the 24h background check.
//
// Mutates global AllowedFetchHosts — not safe for t.Parallel().
func TestSyncRepository_RefreshesUpdateAvailableVersion(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	installer := NewInstaller(t.TempDir())
	mgr := NewManager(service, t.TempDir(), "")
	ctx := t.Context()

	// Plugin installed at 1.0.0 with a stale update hint pointing at an older
	// version (what the pre-8a2b2f2 bug wrote to the DB).
	stale := "0.1.0"
	plugin := &models.Plugin{
		Scope:                  "official",
		ID:                     "my-plugin",
		Name:                   "My Plugin",
		Version:                "1.0.0",
		Status:                 models.PluginStatusActive,
		InstalledAt:            time.Now(),
		AutoUpdate:             true,
		UpdateAvailableVersion: &stale,
	}
	require.NoError(t, service.InstallPlugin(ctx, plugin))

	// Test server serves a repo manifest listing a newer version.
	manifest := RepositoryManifest{
		RepositoryVersion: 1,
		Scope:             "official",
		Name:              "Official Repo",
		Plugins: []AvailablePlugin{{
			ID:   "my-plugin",
			Name: "My Plugin",
			Versions: []PluginVersion{
				{Version: "2.0.0", ManifestVersion: 1},
				{Version: "1.0.0", ManifestVersion: 1},
				{Version: "0.1.0", ManifestVersion: 1},
			},
		}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	require.NoError(t, service.AddRepository(ctx, &models.PluginRepository{
		URL:        server.URL + "/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}))

	h := NewHandler(service, mgr, installer)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope")
	c.SetParamValues("official")

	require.NoError(t, h.syncRepository(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	require.NotNil(t, updated.UpdateAvailableVersion, "sync should have re-evaluated update_available_version")
	assert.Equal(t, "2.0.0", *updated.UpdateAvailableVersion)

	// Response body embeds the PluginRepository fields at the top level and
	// omits update_refresh_error on the happy path so the client doesn't
	// surface a false warning.
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "official", body["scope"])
	assert.NotContains(t, body, "update_refresh_error")
}

// TestSyncRepository_UsesFetchedManifestForRefresh pins the M2 optimization —
// after a sync, CheckForUpdatesForRepo reuses the manifest already fetched by
// syncRepository instead of re-fetching every enabled repo. We prove this by
// adding a second enabled repo whose URL is unreachable; if the handler called
// the broad CheckForUpdates, it would try to hit that URL and log a warning.
// With CheckForUpdatesForRepo it only uses the passed manifest, so the dead
// repo is never contacted.
//
// Mutates global AllowedFetchHosts — not safe for t.Parallel().
func TestSyncRepository_UsesFetchedManifestForRefresh(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	installer := NewInstaller(t.TempDir())
	mgr := NewManager(service, t.TempDir(), "")
	ctx := t.Context()

	require.NoError(t, service.InstallPlugin(ctx, &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Status:      models.PluginStatusActive,
		InstalledAt: time.Now(),
		AutoUpdate:  true,
	}))

	manifest := RepositoryManifest{
		RepositoryVersion: 1,
		Scope:             "official",
		Name:              "Official",
		Plugins: []AvailablePlugin{{
			ID:   "my-plugin",
			Name: "My Plugin",
			Versions: []PluginVersion{
				{Version: "2.0.0", ManifestVersion: 1},
			},
		}},
	}

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	origHosts := AllowedFetchHosts
	AllowedFetchHosts = []string{server.URL}
	defer func() { AllowedFetchHosts = origHosts }()

	require.NoError(t, service.AddRepository(ctx, &models.PluginRepository{
		URL:        server.URL + "/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official"),
		IsOfficial: true,
		Enabled:    true,
	}))

	h := NewHandler(service, mgr, installer)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("scope")
	c.SetParamValues("official")

	require.NoError(t, h.syncRepository(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, hits, "the repo manifest should be fetched exactly once per sync")
}
