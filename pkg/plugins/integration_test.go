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

// TestPluginLifecycle exercises the full plugin lifecycle:
// install, load, run all hooks, and uninstall.
func TestPluginLifecycle(t *testing.T) {
	// 1. Set up infrastructure: in-memory DB, service, plugin directory
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// 2. Create a test plugin on disk with all 4 hook types
	scope := "test"
	pluginID := "lifecycle-plugin"
	destDir := filepath.Join(pluginDir, scope, pluginID)
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifestJSON := `{
  "manifestVersion": 1,
  "id": "lifecycle-plugin",
  "name": "Lifecycle Test Plugin",
  "version": "1.0.0",
  "description": "Integration test plugin exercising all hooks",
  "author": "Test",
  "capabilities": {
    "inputConverter": {
      "description": "Converts test files",
      "sourceTypes": ["testinput"],
      "targetType": "epub"
    },
    "fileParser": {
      "description": "Parses testformat files",
      "types": ["testformat"],
      "mimeTypes": ["application/x-test"]
    },
    "metadataEnricher": {
      "description": "Enriches metadata with genres",
      "fileTypes": ["testformat"]
    },
    "outputGenerator": {
      "description": "Generates test output",
      "id": "test-output",
      "name": "Test Output",
      "sourceTypes": ["epub"]
    }
  },
  "configSchema": {
    "api_key": {
      "type": "string",
      "label": "API Key",
      "required": false
    }
  }
}`

	mainJS := `var plugin = (function() {
  return {
    inputConverter: {
      convert: function(ctx) {
        var content = shisho.fs.readTextFile(ctx.sourcePath);
        var targetPath = ctx.targetDir + "/converted.epub";
        shisho.fs.writeTextFile(targetPath, "converted:" + content);
        return { success: true, targetPath: targetPath };
      }
    },
    fileParser: {
      parse: function(ctx) {
        return {
          title: "Parsed Title",
          authors: [{name: "Test Author", role: "author"}],
          description: "A test book",
          genres: ["Fantasy"],
          series: "Test Series",
          seriesNumber: 1.5
        };
      }
    },
    metadataEnricher: {
      enrich: function(ctx) {
        return {
          modified: true,
          metadata: {
            genres: ["Science Fiction"],
            tags: ["enriched"],
            description: "Enriched: " + ctx.book.title
          }
        };
      }
    },
    outputGenerator: {
      generate: function(ctx) {
        var content = shisho.fs.readTextFile(ctx.sourcePath);
        shisho.fs.writeTextFile(ctx.destPath, "generated:" + content);
      },
      fingerprint: function(ctx) {
        return "fp-" + ctx.book.title + "-" + ctx.file.fileType;
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifestJSON), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	// 3. Install the plugin record in the database
	plugin := &models.Plugin{
		Scope:       scope,
		ID:          pluginID,
		Name:        "Lifecycle Test Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// 4. Create a Manager and load the plugin
	manager := NewManager(service, pluginDir)
	ctx := context.Background()

	err = manager.LoadPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	// 5. Verify runtime is loaded
	rt := manager.GetRuntime(scope, pluginID)
	require.NotNil(t, rt, "runtime should be loaded after LoadPlugin")

	// 6. Verify hook types are detected
	hookTypes := rt.HookTypes()
	assert.Contains(t, hookTypes, "fileParser")
	assert.Contains(t, hookTypes, "metadataEnricher")
	assert.Contains(t, hookTypes, "outputGenerator")
	assert.Contains(t, hookTypes, "inputConverter")
	assert.Len(t, hookTypes, 4)

	// 7. Verify manifest was parsed correctly
	manifest := rt.Manifest()
	assert.Equal(t, "Lifecycle Test Plugin", manifest.Name)
	assert.Equal(t, "1.0.0", manifest.Version)
	assert.Equal(t, "test-output", manifest.Capabilities.OutputGenerator.ID)
	assert.Equal(t, []string{"testformat"}, manifest.Capabilities.FileParser.Types)

	// 8. Verify GetParserForType works
	parserRT := manager.GetParserForType("testformat")
	require.NotNil(t, parserRT, "GetParserForType should find plugin for testformat")
	assert.Equal(t, manifest.Name, parserRT.Manifest().Name)

	// 9. Verify GetOutputGenerator works
	gen := manager.GetOutputGenerator("test-output")
	require.NotNil(t, gen, "GetOutputGenerator should find plugin for test-output")
	assert.Equal(t, "test-output", gen.SupportedType())

	// 10. Verify RegisteredFileExtensions
	exts := manager.RegisteredFileExtensions()
	assert.Contains(t, exts, "testformat")

	// 11. Verify RegisteredConverterExtensions
	convExts := manager.RegisteredConverterExtensions()
	assert.Contains(t, convExts, "testinput")

	// --- Run Hooks ---

	// 12. Run inputConverter
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "input.testinput")
	err = os.WriteFile(sourcePath, []byte("raw content"), 0644)
	require.NoError(t, err)
	targetDir := t.TempDir()

	convertResult, err := manager.RunInputConverter(ctx, rt, sourcePath, targetDir)
	require.NoError(t, err)
	require.NotNil(t, convertResult)
	assert.True(t, convertResult.Success)
	assert.Equal(t, targetDir+"/converted.epub", convertResult.TargetPath)

	convertedContent, err := os.ReadFile(convertResult.TargetPath)
	require.NoError(t, err)
	assert.Equal(t, "converted:raw content", string(convertedContent))

	// 13. Run fileParser
	testFile := filepath.Join(sourceDir, "test.testformat")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	metadata, err := manager.RunFileParser(ctx, rt, testFile, "testformat")
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "Parsed Title", metadata.Title)
	require.Len(t, metadata.Authors, 1)
	assert.Equal(t, "Test Author", metadata.Authors[0].Name)
	assert.Equal(t, "author", metadata.Authors[0].Role)
	assert.Equal(t, "A test book", metadata.Description)
	assert.Equal(t, []string{"Fantasy"}, metadata.Genres)
	assert.Equal(t, "Test Series", metadata.Series)
	require.NotNil(t, metadata.SeriesNumber)
	assert.InDelta(t, 1.5, *metadata.SeriesNumber, 0.001)

	// 14. Run metadataEnricher
	enrichCtx := map[string]interface{}{
		"book": map[string]interface{}{
			"title":   "My Book",
			"authors": []string{"Author A"},
		},
		"file": map[string]interface{}{
			"fileType": "testformat",
			"filePath": testFile,
		},
	}

	enrichResult, err := manager.RunMetadataEnricher(ctx, rt, enrichCtx)
	require.NoError(t, err)
	require.NotNil(t, enrichResult)
	assert.True(t, enrichResult.Modified)
	require.NotNil(t, enrichResult.Metadata)
	assert.Equal(t, []string{"Science Fiction"}, enrichResult.Metadata.Genres)
	assert.Equal(t, []string{"enriched"}, enrichResult.Metadata.Tags)
	assert.Equal(t, "Enriched: My Book", enrichResult.Metadata.Description)

	// 15. Run outputGenerator
	genSourceDir := t.TempDir()
	genSourcePath := filepath.Join(genSourceDir, "source.epub")
	err = os.WriteFile(genSourcePath, []byte("epub content"), 0644)
	require.NoError(t, err)

	genDestDir := t.TempDir()
	destPath := filepath.Join(genDestDir, "output.test")

	bookCtx := map[string]interface{}{
		"title":   "Test Book",
		"authors": []string{"Author A"},
	}
	fileCtx := map[string]interface{}{
		"fileType": "epub",
		"filePath": genSourcePath,
	}

	err = manager.RunOutputGenerator(ctx, rt, genSourcePath, destPath, bookCtx, fileCtx)
	require.NoError(t, err)

	outputContent, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, "generated:epub content", string(outputContent))

	// 16. Run fingerprint
	fp, err := manager.RunFingerprint(rt, bookCtx, fileCtx)
	require.NoError(t, err)
	assert.Equal(t, "fp-Test Book-epub", fp)

	// Verify fingerprint is stable (same inputs produce same output)
	fp2, err := manager.RunFingerprint(rt, bookCtx, fileCtx)
	require.NoError(t, err)
	assert.Equal(t, fp, fp2)

	// --- Unload and verify cleanup ---

	// 17. Unload the plugin
	manager.UnloadPlugin(scope, pluginID)

	// 18. Verify runtime is gone
	rt = manager.GetRuntime(scope, pluginID)
	assert.Nil(t, rt, "runtime should be nil after UnloadPlugin")

	// 19. Verify GetParserForType no longer finds it
	parserRT = manager.GetParserForType("testformat")
	assert.Nil(t, parserRT, "GetParserForType should return nil after unload")

	// 20. Verify GetOutputGenerator no longer finds it
	gen = manager.GetOutputGenerator("test-output")
	assert.Nil(t, gen, "GetOutputGenerator should return nil after unload")

	// 21. Verify RegisteredFileExtensions is now empty
	exts = manager.RegisteredFileExtensions()
	assert.NotContains(t, exts, "testformat")

	// 22. Verify RegisteredConverterExtensions is now empty
	convExts = manager.RegisteredConverterExtensions()
	assert.NotContains(t, convExts, "testinput")

	// --- Reload and verify ---

	// 23. Reload the plugin (simulating a re-enable)
	err = manager.LoadPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	rt = manager.GetRuntime(scope, pluginID)
	require.NotNil(t, rt, "runtime should be available again after reload")

	// 24. Hooks should work again after reload
	metadata, err = manager.RunFileParser(ctx, rt, testFile, "testformat")
	require.NoError(t, err)
	assert.Equal(t, "Parsed Title", metadata.Title)

	// --- Full uninstall ---

	// 25. Unload and uninstall
	manager.UnloadPlugin(scope, pluginID)
	err = service.UninstallPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	// 26. Verify plugin record is gone
	_, err = service.RetrievePlugin(ctx, scope, pluginID)
	require.Error(t, err, "plugin should not be retrievable after uninstall")
}

// TestPluginLifecycle_ConfigIntegration verifies that plugin config is accessible
// from within JavaScript hooks via shisho.config.get/getAll.
func TestPluginLifecycle_ConfigIntegration(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	scope := "test"
	pluginID := "config-plugin"
	destDir := filepath.Join(pluginDir, scope, pluginID)
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifestJSON := `{
  "manifestVersion": 1,
  "id": "config-plugin",
  "name": "Config Test Plugin",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "description": "Uses config in enrichment",
      "fileTypes": ["epub"]
    }
  },
  "configSchema": {
    "prefix": {
      "type": "string",
      "label": "Prefix",
      "required": false
    }
  }
}`

	// Plugin that reads config and uses it in enrichment
	mainJS := `var plugin = (function() {
  return {
    metadataEnricher: {
      enrich: function(ctx) {
        var prefix = shisho.config.get("prefix");
        if (!prefix) {
          prefix = "default";
        }
        return {
          modified: true,
          metadata: {
            description: prefix + ": enriched"
          }
        };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifestJSON), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
	require.NoError(t, err)

	// Install the plugin
	plugin := &models.Plugin{
		Scope:       scope,
		ID:          pluginID,
		Name:        "Config Test Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Set config before loading
	err = service.SetConfig(context.Background(), scope, pluginID, "prefix", "custom")
	require.NoError(t, err)

	// Load and run
	manager := NewManager(service, pluginDir)
	ctx := context.Background()

	err = manager.LoadPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	rt := manager.GetRuntime(scope, pluginID)
	require.NotNil(t, rt)

	enrichCtx := map[string]interface{}{
		"book": map[string]interface{}{"title": "Test"},
		"file": map[string]interface{}{"fileType": "epub"},
	}

	result, err := manager.RunMetadataEnricher(ctx, rt, enrichCtx)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Modified)
	assert.Equal(t, "custom: enriched", result.Metadata.Description)
}

// TestPluginLifecycle_LoadAll verifies that LoadAll loads all enabled plugins
// and properly skips disabled ones.
func TestPluginLifecycle_LoadAll(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	// Create two plugins: one enabled, one disabled
	for _, p := range []struct {
		id      string
		enabled bool
	}{
		{"enabled-plugin", true},
		{"disabled-plugin", false},
	} {
		destDir := filepath.Join(pluginDir, "test", p.id)
		err := os.MkdirAll(destDir, 0755)
		require.NoError(t, err)

		manifest := `{
  "manifestVersion": 1,
  "id": "` + p.id + `",
  "name": "` + p.id + `",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "types": ["testfmt"],
      "description": "test"
    }
  }
}`
		mainJS := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "` + p.id + `" };
      }
    }
  };
})();`

		err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJS), 0644)
		require.NoError(t, err)

		plugin := &models.Plugin{
			Scope:       "test",
			ID:          p.id,
			Name:        p.id,
			Version:     "1.0.0",
			Enabled:     p.enabled,
			InstalledAt: time.Now(),
		}
		err = service.InstallPlugin(context.Background(), plugin)
		require.NoError(t, err)
	}

	// Load all
	manager := NewManager(service, pluginDir)
	err := manager.LoadAll(context.Background())
	require.NoError(t, err)

	// Enabled should be loaded
	rt := manager.GetRuntime("test", "enabled-plugin")
	require.NotNil(t, rt)

	// Disabled should not be loaded
	rt = manager.GetRuntime("test", "disabled-plugin")
	assert.Nil(t, rt)

	// Verify the enabled plugin's hook works
	ctx := context.Background()
	enabledRT := manager.GetRuntime("test", "enabled-plugin")
	md, err := manager.RunFileParser(ctx, enabledRT, "/some/file.testfmt", "testfmt")
	require.NoError(t, err)
	assert.Equal(t, "enabled-plugin", md.Title)
}

// TestPluginLifecycle_ReloadPlugin verifies hot-reload swaps the runtime.
func TestPluginLifecycle_ReloadPlugin(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)
	pluginDir := t.TempDir()

	scope := "test"
	pluginID := "reload-plugin"
	destDir := filepath.Join(pluginDir, scope, pluginID)
	err := os.MkdirAll(destDir, 0755)
	require.NoError(t, err)

	manifest := `{
  "manifestVersion": 1,
  "id": "reload-plugin",
  "name": "Reload Plugin",
  "version": "1.0.0",
  "capabilities": {
    "fileParser": {
      "types": ["testfmt"],
      "description": "test"
    }
  }
}`
	mainJSV1 := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Version 1" };
      }
    }
  };
})();`

	err = os.WriteFile(filepath.Join(destDir, "manifest.json"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJSV1), 0644)
	require.NoError(t, err)

	plugin := &models.Plugin{
		Scope:       scope,
		ID:          pluginID,
		Name:        "Reload Plugin",
		Version:     "1.0.0",
		Enabled:     true,
		InstalledAt: time.Now(),
	}
	err = service.InstallPlugin(context.Background(), plugin)
	require.NoError(t, err)

	manager := NewManager(service, pluginDir)
	ctx := context.Background()

	err = manager.LoadPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	// Verify v1
	rt := manager.GetRuntime(scope, pluginID)
	md, err := manager.RunFileParser(ctx, rt, "/file.testfmt", "testfmt")
	require.NoError(t, err)
	assert.Equal(t, "Version 1", md.Title)

	// Update main.js to v2
	mainJSV2 := `var plugin = (function() {
  return {
    fileParser: {
      parse: function(ctx) {
        return { title: "Version 2" };
      }
    }
  };
})();`
	err = os.WriteFile(filepath.Join(destDir, "main.js"), []byte(mainJSV2), 0644)
	require.NoError(t, err)

	// Reload
	err = manager.ReloadPlugin(ctx, scope, pluginID)
	require.NoError(t, err)

	// Verify v2
	rt = manager.GetRuntime(scope, pluginID)
	require.NotNil(t, rt)
	md, err = manager.RunFileParser(ctx, rt, "/file.testfmt", "testfmt")
	require.NoError(t, err)
	assert.Equal(t, "Version 2", md.Title)
}
