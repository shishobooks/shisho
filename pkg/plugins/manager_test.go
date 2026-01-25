package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestManager creates a Manager with a temp plugin directory containing
// the specified testdata plugin(s) copied into scope/id subdirectories.
func setupTestManager(t *testing.T, plugins ...struct{ scope, id, testdata string }) (*Manager, *Service) {
	t.Helper()

	db := setupTestDB(t)
	service := NewService(db)

	pluginDir := t.TempDir()

	for _, p := range plugins {
		destDir := filepath.Join(pluginDir, p.scope, p.id)
		err := os.MkdirAll(destDir, 0755)
		require.NoError(t, err)

		srcDir := filepath.Join("testdata", p.testdata)
		copyTestdataFile(t, srcDir, destDir, "manifest.json")
		copyTestdataFile(t, srcDir, destDir, "main.js")
	}

	manager := NewManager(service, pluginDir)
	return manager, service
}

func copyTestdataFile(t *testing.T, srcDir, destDir, filename string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(srcDir, filename))
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, filename), data, 0644)
	require.NoError(t, err)
}

func TestManager_LoadAll(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install an enabled plugin record
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// LoadAll should load it
	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	rt := mgr.GetRuntime("test", "simple-enricher")
	require.NotNil(t, rt)
	assert.Equal(t, "Simple Enricher", rt.manifest.Name)
}

func TestManager_LoadAll_Disabled(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install a disabled plugin record
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     false,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	rt := mgr.GetRuntime("test", "simple-enricher")
	assert.Nil(t, rt)
}

func TestManager_LoadAll_LoadError(t *testing.T) {
	// Create a manager with no plugin files (pointing to non-existent dir)
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()
	mgr := NewManager(service, pluginDir)
	ctx := context.Background()

	// Install a plugin record pointing to non-existent directory
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "nonexistent-plugin",
		Name:        "Nonexistent Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	// LoadAll should not fail, but store error
	err = mgr.LoadAll(ctx)
	require.NoError(t, err)

	// Runtime should not be loaded
	rt := mgr.GetRuntime("test", "nonexistent-plugin")
	assert.Nil(t, rt)

	// LoadError should be stored in the database
	retrieved, err := service.RetrievePlugin(ctx, "test", "nonexistent-plugin")
	require.NoError(t, err)
	assert.NotNil(t, retrieved.LoadError)
	assert.Contains(t, *retrieved.LoadError, "failed to load plugin")
}

func TestManager_LoadPlugin(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install plugin record (needed for AppendToOrder foreign key)
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "simple-enricher")
	require.NoError(t, err)

	rt := mgr.GetRuntime("test", "simple-enricher")
	require.NotNil(t, rt)
	assert.Equal(t, "Simple Enricher", rt.manifest.Name)
}

func TestManager_UnloadPlugin(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install and load
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "simple-enricher")
	require.NoError(t, err)
	require.NotNil(t, mgr.GetRuntime("test", "simple-enricher"))

	// Unload
	mgr.UnloadPlugin("test", "simple-enricher")
	assert.Nil(t, mgr.GetRuntime("test", "simple-enricher"))
}

func TestManager_ReloadPlugin(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install and load
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "simple-enricher")
	require.NoError(t, err)

	// Reload
	err = mgr.ReloadPlugin(ctx, "test", "simple-enricher")
	require.NoError(t, err)

	// Should still be accessible
	rt := mgr.GetRuntime("test", "simple-enricher")
	require.NotNil(t, rt)
	assert.Equal(t, "Simple Enricher", rt.manifest.Name)
}

func TestManager_GetOrderedRuntimes(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "simple-enricher", "simple-enricher"})
	ctx := context.Background()

	// Install and load plugin
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "simple-enricher",
		Name:        "Simple Enricher",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "simple-enricher")
	require.NoError(t, err)

	// The plugin has metadataEnricher hook, so order table should have an entry
	runtimes, err := mgr.GetOrderedRuntimes(ctx, models.PluginHookMetadataEnricher, 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "Simple Enricher", runtimes[0].manifest.Name)

	// Hook type with no plugins should return empty
	runtimes, err = mgr.GetOrderedRuntimes(ctx, models.PluginHookFileParser, 0)
	require.NoError(t, err)
	assert.Empty(t, runtimes)
}

func TestManager_RegisteredFileExtensions(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "multi-hook", "multi-hook"})
	ctx := context.Background()

	// Install and load multi-hook plugin (has fileParser types: ["pdf"])
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "multi-hook",
		Name:        "Multi Hook Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "multi-hook")
	require.NoError(t, err)

	exts := mgr.RegisteredFileExtensions()
	assert.Contains(t, exts, "pdf")
}

func TestManager_RegisteredFileExtensions_SkipsReserved(t *testing.T) {
	// Create a plugin that declares reserved extensions in fileParser.types
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Create a plugin dir with manifest that includes reserved types
	destDir := filepath.Join(pluginDir, "test", "reserved-parser")
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "reserved-parser",
  "name": "Reserved Parser",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "description": "Test parser with reserved types",
      "types": ["epub", "cbz", "m4b", "docx"]
    }
  }
}`
	mainJS := `var plugin = (function() {
  return {
    fileParser: {
      types: ["epub", "cbz", "m4b", "docx"],
      parse: function(context) { return { title: "Test" }; }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	mgr := NewManager(service, pluginDir)
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "reserved-parser",
		Name:        "Reserved Parser",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "reserved-parser")
	require.NoError(t, err)

	exts := mgr.RegisteredFileExtensions()
	// Should include docx but not reserved types
	assert.Contains(t, exts, "docx")
	assert.NotContains(t, exts, "epub")
	assert.NotContains(t, exts, "cbz")
	assert.NotContains(t, exts, "m4b")
}

func TestManager_RegisteredConverterExtensions(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "multi-hook", "multi-hook"})
	ctx := context.Background()

	// Install and load multi-hook plugin (has inputConverter sourceTypes: ["pdf"])
	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "multi-hook",
		Name:        "Multi Hook Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "multi-hook")
	require.NoError(t, err)

	exts := mgr.RegisteredConverterExtensions()
	assert.Contains(t, exts, "pdf")
}

func TestManager_GetParserForType(t *testing.T) {
	mgr, svc := setupTestManager(t, struct{ scope, id, testdata string }{"test", "multi-hook", "multi-hook"})
	ctx := context.Background()

	plugin := &models.Plugin{
		Scope:       "test",
		ID:          "multi-hook",
		Name:        "Multi Hook Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err := svc.InstallPlugin(ctx, plugin)
	require.NoError(t, err)

	err = mgr.LoadPlugin(ctx, "test", "multi-hook")
	require.NoError(t, err)

	// Should find the parser for "pdf"
	rt := mgr.GetParserForType("pdf")
	require.NotNil(t, rt)
	assert.Equal(t, "Multi Hook Plugin", rt.manifest.Name)

	// Should return nil for unknown type
	rt = mgr.GetParserForType("unknown")
	assert.Nil(t, rt)

	// Should return nil for reserved types
	rt = mgr.GetParserForType("epub")
	assert.Nil(t, rt)
	rt = mgr.GetParserForType("cbz")
	assert.Nil(t, rt)
	rt = mgr.GetParserForType("m4b")
	assert.Nil(t, rt)
}

func TestManager_GetOrderedRuntimes_WithLibrary(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	// Create a real library (FK constraint requires it)
	library := insertTestLibrary(t, db, "Test Library")

	// Install two plugins
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// Set global order
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginOrder{
		{Scope: "test", PluginID: "enricher1"},
		{Scope: "test", PluginID: "enricher2"},
	})
	require.NoError(t, err)

	// Create mock runtimes
	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	rt2 := &Runtime{scope: "test", pluginID: "enricher2"}
	mgr.plugins["test/enricher1"] = rt1
	mgr.plugins["test/enricher2"] = rt2

	// libraryID=0 falls back to global order
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
	assert.Equal(t, "enricher2", runtimes[1].pluginID)

	// Set library-specific order (enricher1 disabled, enricher2 enabled)
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher2", Enabled: true},
		{Scope: "test", PluginID: "enricher1", Enabled: false},
	})
	require.NoError(t, err)

	// Uses library order (only enabled plugins)
	runtimes, err = mgr.GetOrderedRuntimes(ctx, "metadataEnricher", library.ID)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher2", runtimes[0].pluginID)

	// Non-customized library falls back to global order
	runtimes, err = mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 99999)
	require.NoError(t, err)
	require.Len(t, runtimes, 2)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
	assert.Equal(t, "enricher2", runtimes[1].pluginID)
}

func TestManager_GetOrderedRuntimes_GlobalDisabledExcluded(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	mgr := &Manager{
		service: svc,
		plugins: make(map[string]*Runtime),
	}

	// Create a real library (FK constraint requires it)
	library := insertTestLibrary(t, db, "Test Library")

	// Install two plugins
	p1 := &models.Plugin{Scope: "test", ID: "enricher1", Name: "Enricher 1", Version: "1.0.0", Enabled: true}
	p2 := &models.Plugin{Scope: "test", ID: "enricher2", Name: "Enricher 2", Version: "1.0.0", Enabled: true}
	_, err := db.NewInsert().Model(p1).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(p2).Exec(ctx)
	require.NoError(t, err)

	// Set global order with both plugins
	err = svc.SetOrder(ctx, "metadataEnricher", []models.PluginOrder{
		{Scope: "test", PluginID: "enricher1"},
		{Scope: "test", PluginID: "enricher2"},
	})
	require.NoError(t, err)

	// Only load enricher1 into the plugins map (enricher2 is globally disabled/unloaded)
	rt1 := &Runtime{scope: "test", pluginID: "enricher1"}
	mgr.plugins["test/enricher1"] = rt1

	// Set library order with both enabled
	err = svc.SetLibraryOrder(ctx, library.ID, "metadataEnricher", []models.LibraryPlugin{
		{Scope: "test", PluginID: "enricher1", Enabled: true},
		{Scope: "test", PluginID: "enricher2", Enabled: true},
	})
	require.NoError(t, err)

	// Even though library order says enricher2 is enabled,
	// it's not in the plugins map (globally disabled), so it's excluded
	runtimes, err := mgr.GetOrderedRuntimes(ctx, "metadataEnricher", library.ID)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)

	// Same for global fallback path
	runtimes, err = mgr.GetOrderedRuntimes(ctx, "metadataEnricher", 0)
	require.NoError(t, err)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "enricher1", runtimes[0].pluginID)
}
