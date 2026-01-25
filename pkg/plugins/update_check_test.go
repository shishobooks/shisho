package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_CheckForUpdates_UpdateAvailable(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// Install a plugin at version 1.0.0
	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// Add an enabled repository
	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	// Mock fetchRepo to return a manifest with a newer version
	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "1.0.0", ManifestVersion: 1},
						{Version: "2.0.0", ManifestVersion: 1},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Verify the plugin has UpdateAvailableVersion set
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	require.NotNil(t, updated.UpdateAvailableVersion)
	assert.Equal(t, "2.0.0", *updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_AlreadyUpToDate(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// Install a plugin already at the latest version
	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "2.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "1.0.0", ManifestVersion: 1},
						{Version: "2.0.0", ManifestVersion: 1},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// UpdateAvailableVersion should remain nil
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_ClearsStaleUpdate(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// Install a plugin that was previously flagged as having an update
	staleVersion := "2.0.0"
	plugin := &models.Plugin{
		Scope:                  "official",
		ID:                     "my-plugin",
		Name:                   "My Plugin",
		Version:                "2.0.0",
		Enabled:                true,
		InstalledAt:            time.Now(),
		UpdateAvailableVersion: &staleVersion,
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	// The repo now only has 2.0.0 (same as installed)
	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "2.0.0", ManifestVersion: 1},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// UpdateAvailableVersion should be cleared
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_DisabledRepoSkipped(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// Remove the seeded official repository so we control what's in the DB
	_, err := db.NewDelete().Model((*models.PluginRepository)(nil)).Where("1=1").Exec(ctx)
	require.NoError(t, err)

	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// Add a disabled repository
	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    false,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	fetchCalled := false
	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		fetchCalled = true
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "2.0.0", ManifestVersion: 1},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// fetchRepo should not have been called for the disabled repo
	assert.False(t, fetchCalled)

	// No update should be recorded
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_FetchErrorSkipped(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	// Simulate a fetch error
	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return nil, errors.New("network timeout")
	}

	// Should not return an error — just skip the repo
	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// No update should be recorded
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_IncompatibleVersionsFiltered(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "official",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	// Only the newer version has an unsupported manifest version
	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "1.0.0", ManifestVersion: 1},
						{Version: "3.0.0", ManifestVersion: 999}, // unsupported
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// No update should be recorded since 3.0.0 is incompatible and 1.0.0 is the same
	updated, err := service.RetrievePlugin(ctx, "official", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}

func TestManager_CheckForUpdates_NoPlugins(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// No plugins installed — should return immediately without error
	err := mgr.CheckForUpdates(ctx)
	require.NoError(t, err)
}

func TestManager_CheckForUpdates_ScopeMismatch(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	mgr := NewManager(service, t.TempDir())
	ctx := context.Background()

	// Plugin from scope "community"
	plugin := &models.Plugin{
		Scope:       "community",
		ID:          "my-plugin",
		Name:        "My Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// Repository from scope "official"
	repo := &models.PluginRepository{
		URL:        "https://raw.githubusercontent.com/test/repo/main/manifest.json",
		Scope:      "official",
		Name:       strPtr("Official Repo"),
		IsOfficial: true,
		Enabled:    true,
	}
	err = service.AddRepository(ctx, repo)
	require.NoError(t, err)

	mgr.fetchRepo = func(_ string) (*RepositoryManifest, error) {
		return &RepositoryManifest{
			RepositoryVersion: 1,
			Scope:             "official",
			Name:              "Official Repo",
			Plugins: []AvailablePlugin{
				{
					ID:   "my-plugin",
					Name: "My Plugin",
					Versions: []PluginVersion{
						{Version: "2.0.0", ManifestVersion: 1},
					},
				},
			},
		}, nil
	}

	err = mgr.CheckForUpdates(ctx)
	require.NoError(t, err)

	// Scope doesn't match, so no update should be set
	updated, err := service.RetrievePlugin(ctx, "community", "my-plugin")
	require.NoError(t, err)
	assert.Nil(t, updated.UpdateAvailableVersion)
}
