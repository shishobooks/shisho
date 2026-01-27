package plugins

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Enable foreign keys for cascade behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func insertTestPlugin(t *testing.T, db *bun.DB, scope, id string) *models.Plugin {
	t.Helper()
	plugin := &models.Plugin{
		Scope:       scope,
		ID:          id,
		Name:        id + " Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	_, err := db.NewInsert().Model(plugin).Exec(context.Background())
	require.NoError(t, err)
	return plugin
}

func TestService_InstallAndRetrievePlugin(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	desc := "A test plugin"
	author := "Test Author"
	plugin := &models.Plugin{
		Scope:       "community",
		ID:          "test-plugin",
		Name:        "Test Plugin",
		Version:     "1.0.0",
		Description: &desc,
		Author:      &author,
		Enabled:     true,
		InstalledAt: time.Now().UTC().Truncate(time.Second),
	}

	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	retrieved, err := svc.RetrievePlugin(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Equal(t, "Test Plugin", retrieved.Name)
	assert.Equal(t, "1.0.0", retrieved.Version)
	assert.Equal(t, &desc, retrieved.Description)
	assert.Equal(t, &author, retrieved.Author)
	assert.True(t, retrieved.Enabled)

	// Test not found
	_, err = svc.RetrievePlugin(ctx, "community", "nonexistent")
	require.Error(t, err)
}

func TestService_ListPlugins(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "alpha")
	insertTestPlugin(t, db, "community", "beta")
	insertTestPlugin(t, db, "shisho", "core")

	plugins, err := svc.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, plugins, 3)

	// Should be ordered by scope ASC, id ASC
	assert.Equal(t, "alpha", plugins[0].ID)
	assert.Equal(t, "community", plugins[0].Scope)
	assert.Equal(t, "beta", plugins[1].ID)
	assert.Equal(t, "community", plugins[1].Scope)
	assert.Equal(t, "core", plugins[2].ID)
	assert.Equal(t, "shisho", plugins[2].Scope)
}

func TestService_UpdatePlugin(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	plugin := insertTestPlugin(t, db, "community", "test-plugin")

	plugin.Version = "2.0.0"
	plugin.Enabled = false
	now := time.Now().UTC().Truncate(time.Second)
	plugin.UpdatedAt = &now

	err := svc.UpdatePlugin(ctx, plugin)
	require.NoError(t, err)

	retrieved, err := svc.RetrievePlugin(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", retrieved.Version)
	assert.False(t, retrieved.Enabled)
	assert.NotNil(t, retrieved.UpdatedAt)
}

func TestService_UninstallPlugin(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Add config for the plugin
	err := svc.SetConfig(ctx, "community", "test-plugin", "api_key", "secret123")
	require.NoError(t, err)

	// Add order entry for the plugin
	err = svc.AppendToOrder(ctx, models.PluginHookFileParser, "community", "test-plugin")
	require.NoError(t, err)

	// Verify config exists
	val, err := svc.GetConfigRaw(ctx, "community", "test-plugin", "api_key")
	require.NoError(t, err)
	require.NotNil(t, val)

	// Uninstall
	err = svc.UninstallPlugin(ctx, "community", "test-plugin")
	require.NoError(t, err)

	// Plugin should be gone
	_, err = svc.RetrievePlugin(ctx, "community", "test-plugin")
	require.Error(t, err)

	// Config should be cascade-deleted
	val, err = svc.GetConfigRaw(ctx, "community", "test-plugin", "api_key")
	require.NoError(t, err)
	assert.Nil(t, val)

	// Order should be cascade-deleted
	orders, err := svc.GetOrder(ctx, models.PluginHookFileParser)
	require.NoError(t, err)
	assert.Empty(t, orders)
}

func TestService_GetConfig_MasksSecrets(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Set a secret config and a normal config
	err := svc.SetConfig(ctx, "community", "test-plugin", "api_key", "secret123")
	require.NoError(t, err)
	err = svc.SetConfig(ctx, "community", "test-plugin", "endpoint", "https://api.example.com")
	require.NoError(t, err)

	schema := ConfigSchema{
		"api_key": ConfigField{
			Type:   "string",
			Label:  "API Key",
			Secret: true,
		},
		"endpoint": ConfigField{
			Type:  "string",
			Label: "Endpoint",
		},
	}

	// Get with raw=false should mask secrets
	config, err := svc.GetConfig(ctx, "community", "test-plugin", schema, false)
	require.NoError(t, err)
	assert.Equal(t, "***", config["api_key"])
	assert.Equal(t, "https://api.example.com", config["endpoint"])

	// Get with raw=true should not mask
	configRaw, err := svc.GetConfig(ctx, "community", "test-plugin", schema, true)
	require.NoError(t, err)
	assert.Equal(t, "secret123", configRaw["api_key"])
	assert.Equal(t, "https://api.example.com", configRaw["endpoint"])
}

func TestService_SetConfig_Upsert(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Initial set
	err := svc.SetConfig(ctx, "community", "test-plugin", "key1", "value1")
	require.NoError(t, err)

	val, err := svc.GetConfigRaw(ctx, "community", "test-plugin", "key1")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.Equal(t, "value1", *val)

	// Upsert with new value
	err = svc.SetConfig(ctx, "community", "test-plugin", "key1", "value2")
	require.NoError(t, err)

	val, err = svc.GetConfigRaw(ctx, "community", "test-plugin", "key1")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.Equal(t, "value2", *val)
}

func TestService_GetConfigRaw(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Non-existent key returns nil
	val, err := svc.GetConfigRaw(ctx, "community", "test-plugin", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, val)

	// Set and retrieve
	err = svc.SetConfig(ctx, "community", "test-plugin", "mykey", "myvalue")
	require.NoError(t, err)

	val, err = svc.GetConfigRaw(ctx, "community", "test-plugin", "mykey")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.Equal(t, "myvalue", *val)
}

func TestService_GetOrder_SetOrder(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "plugin-a")
	insertTestPlugin(t, db, "community", "plugin-b")
	insertTestPlugin(t, db, "shisho", "plugin-c")

	// Initially empty
	orders, err := svc.GetOrder(ctx, models.PluginHookFileParser)
	require.NoError(t, err)
	assert.Empty(t, orders)

	// Set order
	entries := []models.PluginOrder{
		{Scope: "community", PluginID: "plugin-b"},
		{Scope: "community", PluginID: "plugin-a"},
		{Scope: "shisho", PluginID: "plugin-c"},
	}
	err = svc.SetOrder(ctx, models.PluginHookFileParser, entries)
	require.NoError(t, err)

	// Retrieve and verify positions
	orders, err = svc.GetOrder(ctx, models.PluginHookFileParser)
	require.NoError(t, err)
	require.Len(t, orders, 3)
	assert.Equal(t, "plugin-b", orders[0].PluginID)
	assert.Equal(t, 0, orders[0].Position)
	assert.Equal(t, "plugin-a", orders[1].PluginID)
	assert.Equal(t, 1, orders[1].Position)
	assert.Equal(t, "plugin-c", orders[2].PluginID)
	assert.Equal(t, 2, orders[2].Position)

	// Reorder by setting a new order
	newEntries := []models.PluginOrder{
		{Scope: "shisho", PluginID: "plugin-c"},
		{Scope: "community", PluginID: "plugin-a"},
	}
	err = svc.SetOrder(ctx, models.PluginHookFileParser, newEntries)
	require.NoError(t, err)

	orders, err = svc.GetOrder(ctx, models.PluginHookFileParser)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, "plugin-c", orders[0].PluginID)
	assert.Equal(t, "plugin-a", orders[1].PluginID)
}

func TestService_AppendToOrder(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "plugin-a")
	insertTestPlugin(t, db, "community", "plugin-b")

	// Append first
	err := svc.AppendToOrder(ctx, models.PluginHookOutputGenerator, "community", "plugin-a")
	require.NoError(t, err)

	orders, err := svc.GetOrder(ctx, models.PluginHookOutputGenerator)
	require.NoError(t, err)
	require.Len(t, orders, 1)
	assert.Equal(t, "plugin-a", orders[0].PluginID)
	assert.Equal(t, 0, orders[0].Position)

	// Append second
	err = svc.AppendToOrder(ctx, models.PluginHookOutputGenerator, "community", "plugin-b")
	require.NoError(t, err)

	orders, err = svc.GetOrder(ctx, models.PluginHookOutputGenerator)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, "plugin-a", orders[0].PluginID)
	assert.Equal(t, 0, orders[0].Position)
	assert.Equal(t, "plugin-b", orders[1].PluginID)
	assert.Equal(t, 1, orders[1].Position)
}

func TestService_ListRepositories(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	repos, err := svc.ListRepositories(ctx)
	require.NoError(t, err)

	// The official repo should be seeded by migration
	require.Len(t, repos, 1)
	assert.Equal(t, "shisho", repos[0].Scope)
	assert.True(t, repos[0].IsOfficial)
	assert.True(t, repos[0].Enabled)
}

func TestService_AddAndRemoveRepository(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Add a community repo
	repo := &models.PluginRepository{
		URL:     "https://example.com/plugins/repository.json",
		Scope:   "example",
		Enabled: true,
	}
	err := svc.AddRepository(ctx, repo)
	require.NoError(t, err)

	repos, err := svc.ListRepositories(ctx)
	require.NoError(t, err)
	require.Len(t, repos, 2)

	// Official should come first due to ordering
	assert.Equal(t, "shisho", repos[0].Scope)
	assert.Equal(t, "example", repos[1].Scope)

	// Remove the community repo
	err = svc.RemoveRepository(ctx, "example")
	require.NoError(t, err)

	repos, err = svc.ListRepositories(ctx)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "shisho", repos[0].Scope)

	// Attempting to remove the official repo should not delete it
	err = svc.RemoveRepository(ctx, "shisho")
	require.NoError(t, err) // no error, but nothing deleted

	repos, err = svc.ListRepositories(ctx)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "shisho", repos[0].Scope)
}

func TestService_UpsertIdentifierTypes(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "metadata-plugin")

	// Initial upsert
	types := []IdentifierTypeCap{
		{
			ID:          "isbn",
			Name:        "ISBN",
			URLTemplate: "https://openlibrary.org/isbn/{{id}}",
			Pattern:     `^\d{10,13}$`,
		},
		{
			ID:   "custom-id",
			Name: "Custom ID",
		},
	}

	err := svc.UpsertIdentifierTypes(ctx, "community", "metadata-plugin", types)
	require.NoError(t, err)

	// Verify by querying directly
	var idTypes []*models.PluginIdentifierType
	err = db.NewSelect().Model(&idTypes).
		Where("scope = ?", "community").
		Where("plugin_id = ?", "metadata-plugin").
		OrderExpr("id ASC").
		Scan(ctx)
	require.NoError(t, err)
	require.Len(t, idTypes, 2)

	assert.Equal(t, "custom-id", idTypes[0].ID)
	assert.Equal(t, "Custom ID", idTypes[0].Name)
	assert.Nil(t, idTypes[0].URLTemplate)
	assert.Nil(t, idTypes[0].Pattern)

	assert.Equal(t, "isbn", idTypes[1].ID)
	assert.Equal(t, "ISBN", idTypes[1].Name)
	assert.NotNil(t, idTypes[1].URLTemplate)
	assert.Equal(t, "https://openlibrary.org/isbn/{{id}}", *idTypes[1].URLTemplate)
	assert.NotNil(t, idTypes[1].Pattern)
	assert.Equal(t, `^\d{10,13}$`, *idTypes[1].Pattern)

	// Upsert with different types (should replace)
	newTypes := []IdentifierTypeCap{
		{
			ID:   "doi",
			Name: "DOI",
		},
	}
	err = svc.UpsertIdentifierTypes(ctx, "community", "metadata-plugin", newTypes)
	require.NoError(t, err)

	idTypes = nil
	err = db.NewSelect().Model(&idTypes).
		Where("scope = ?", "community").
		Where("plugin_id = ?", "metadata-plugin").
		Scan(ctx)
	require.NoError(t, err)
	require.Len(t, idTypes, 1)
	assert.Equal(t, "doi", idTypes[0].ID)

	// Upsert with empty types (should clear all)
	err = svc.UpsertIdentifierTypes(ctx, "community", "metadata-plugin", nil)
	require.NoError(t, err)

	idTypes = nil
	err = db.NewSelect().Model(&idTypes).
		Where("scope = ?", "community").
		Where("plugin_id = ?", "metadata-plugin").
		Scan(ctx)
	require.NoError(t, err)
	assert.Empty(t, idTypes)
}

func TestService_GetFieldSettings_EmptyByDefault(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// No settings stored = empty map (everything enabled by default)
	settings, err := svc.GetFieldSettings(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestService_SetFieldSetting_DisableField(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Disable a field
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	settings, err := svc.GetFieldSettings(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Len(t, settings, 1)
	assert.False(t, settings["title"])
}

func TestService_SetFieldSetting_EnableFieldRemovesRow(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Disable then re-enable
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	err = svc.SetFieldSetting(ctx, "community", "test-plugin", "title", true)
	require.NoError(t, err)

	// Should be empty (enabled = no row)
	settings, err := svc.GetFieldSettings(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestService_SetFieldSetting_MultipleFields(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")

	// Disable multiple fields
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)
	err = svc.SetFieldSetting(ctx, "community", "test-plugin", "authors", false)
	require.NoError(t, err)
	err = svc.SetFieldSetting(ctx, "community", "test-plugin", "description", false)
	require.NoError(t, err)

	settings, err := svc.GetFieldSettings(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Len(t, settings, 3)
	assert.False(t, settings["title"])
	assert.False(t, settings["authors"])
	assert.False(t, settings["description"])

	// Re-enable one
	err = svc.SetFieldSetting(ctx, "community", "test-plugin", "authors", true)
	require.NoError(t, err)

	settings, err = svc.GetFieldSettings(ctx, "community", "test-plugin")
	require.NoError(t, err)
	assert.Len(t, settings, 2)
	assert.False(t, settings["title"])
	assert.False(t, settings["description"])
	_, hasAuthors := settings["authors"]
	assert.False(t, hasAuthors) // no key means enabled
}

func TestService_GetLibraryFieldSettings_EmptyByDefault(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	settings, err := svc.GetLibraryFieldSettings(ctx, library.ID, "community", "test-plugin")
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestService_SetLibraryFieldSetting_Override(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	// Library can disable a field
	err := svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	settings, err := svc.GetLibraryFieldSettings(ctx, library.ID, "community", "test-plugin")
	require.NoError(t, err)
	assert.Len(t, settings, 1)
	assert.False(t, settings["title"])

	// Library can also explicitly enable (to override global disable)
	err = svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "authors", true)
	require.NoError(t, err)

	settings, err = svc.GetLibraryFieldSettings(ctx, library.ID, "community", "test-plugin")
	require.NoError(t, err)
	assert.Len(t, settings, 2)
	assert.False(t, settings["title"])
	assert.True(t, settings["authors"])
}

func TestService_ResetLibraryFieldSettings(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	// Set some overrides
	err := svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "title", false)
	require.NoError(t, err)
	err = svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "authors", true)
	require.NoError(t, err)

	// Reset all
	err = svc.ResetLibraryFieldSettings(ctx, library.ID, "community", "test-plugin")
	require.NoError(t, err)

	settings, err := svc.GetLibraryFieldSettings(ctx, library.ID, "community", "test-plugin")
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestService_GetEffectiveFieldSettings_GlobalOnly(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	// Disable title globally
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	declaredFields := []string{"title", "authors", "description"}
	effective, err := svc.GetEffectiveFieldSettings(ctx, library.ID, "community", "test-plugin", declaredFields)
	require.NoError(t, err)

	// title=false (from global), others=true (default)
	assert.False(t, effective["title"])
	assert.True(t, effective["authors"])
	assert.True(t, effective["description"])
}

func TestService_GetEffectiveFieldSettings_LibraryOverridesGlobal(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	// Disable title globally
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	// Library re-enables title
	err = svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "title", true)
	require.NoError(t, err)

	// Library disables authors (globally enabled by default)
	err = svc.SetLibraryFieldSetting(ctx, library.ID, "community", "test-plugin", "authors", false)
	require.NoError(t, err)

	declaredFields := []string{"title", "authors", "description"}
	effective, err := svc.GetEffectiveFieldSettings(ctx, library.ID, "community", "test-plugin", declaredFields)
	require.NoError(t, err)

	assert.True(t, effective["title"])       // library enabled, overrides global
	assert.False(t, effective["authors"])    // library disabled, overrides default
	assert.True(t, effective["description"]) // default enabled
}

func TestService_GetEffectiveFieldSettings_OnlyDeclaredFields(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	// Disable title globally
	err := svc.SetFieldSetting(ctx, "community", "test-plugin", "title", false)
	require.NoError(t, err)

	// Only ask for subset of fields
	declaredFields := []string{"authors", "description"}
	effective, err := svc.GetEffectiveFieldSettings(ctx, library.ID, "community", "test-plugin", declaredFields)
	require.NoError(t, err)

	// Only requested fields returned
	assert.Len(t, effective, 2)
	assert.True(t, effective["authors"])
	assert.True(t, effective["description"])
	_, hasTitle := effective["title"]
	assert.False(t, hasTitle)
}

func TestService_GetEffectiveFieldSettings_EmptyDeclaredFields(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	insertTestPlugin(t, db, "community", "test-plugin")
	library := insertTestLibrary(t, db, "Test Library")

	effective, err := svc.GetEffectiveFieldSettings(ctx, library.ID, "community", "test-plugin", nil)
	require.NoError(t, err)
	assert.Empty(t, effective)

	effective, err = svc.GetEffectiveFieldSettings(ctx, library.ID, "community", "test-plugin", []string{})
	require.NoError(t, err)
	assert.Empty(t, effective)
}
