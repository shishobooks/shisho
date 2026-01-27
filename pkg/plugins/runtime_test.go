package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testdataRoot = "testdata"

func TestLoadPlugin_SimpleEnricher(t *testing.T) {
	dir := filepath.Join(testdataRoot, "simple-enricher")
	rt, err := LoadPlugin(dir, "official", "simple-enricher")
	require.NoError(t, err)
	require.NotNil(t, rt)

	hooks := rt.HookTypes()
	assert.Equal(t, []string{"metadataEnricher"}, hooks)
}

func TestLoadPlugin_MultiHook(t *testing.T) {
	dir := filepath.Join(testdataRoot, "multi-hook")
	rt, err := LoadPlugin(dir, "official", "multi-hook")
	require.NoError(t, err)
	require.NotNil(t, rt)

	hooks := rt.HookTypes()
	assert.Equal(t, []string{"fileParser", "inputConverter"}, hooks)
}

func TestLoadPlugin_UndeclaredHook(t *testing.T) {
	dir := filepath.Join(testdataRoot, "undeclared-hook")
	_, err := LoadPlugin(dir, "official", "undeclared")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exports 'metadataEnricher' but manifest does not declare it")
}

func TestLoadPlugin_MissingMainJS(t *testing.T) {
	dir := filepath.Join(testdataRoot, "missing-mainjs")
	_, err := LoadPlugin(dir, "official", "missing-mainjs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read main.js")
}

func TestLoadPlugin_InvalidJS(t *testing.T) {
	dir := filepath.Join(testdataRoot, "invalid-js")
	_, err := LoadPlugin(dir, "official", "invalid-js")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute main.js")
}

func TestLoadPlugin_ManifestReturned(t *testing.T) {
	dir := filepath.Join(testdataRoot, "simple-enricher")
	rt, err := LoadPlugin(dir, "official", "simple-enricher")
	require.NoError(t, err)
	require.NotNil(t, rt)

	m := rt.Manifest()
	require.NotNil(t, m)
	assert.Equal(t, "simple-enricher", m.ID)
	assert.Equal(t, "Simple Enricher", m.Name)
	assert.Equal(t, "1.0.0", m.Version)
	assert.Equal(t, 1, m.ManifestVersion)
	require.NotNil(t, m.Capabilities.MetadataEnricher)
	assert.Equal(t, "Test enricher", m.Capabilities.MetadataEnricher.Description)
	assert.Equal(t, []string{"epub"}, m.Capabilities.MetadataEnricher.FileTypes)
}

func TestLoadPlugin_MetadataEnricherFieldValidation(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		mainJS      string
		wantErr     string
		wantFields  []string
		wantWarning string
	}{
		{
			name: "valid fields - should load successfully",
			manifest: `{
				"manifestVersion": 1,
				"id": "test-enricher",
				"name": "Test Enricher",
				"version": "1.0.0",
				"capabilities": {
					"metadataEnricher": {
						"fileTypes": ["epub"],
						"fields": ["title", "authors", "cover"]
					}
				}
			}`,
			mainJS: `var plugin = (function() {
				return { metadataEnricher: { enrich: function() { return { modified: false }; } } };
			})();`,
			wantFields: []string{"title", "authors", "cover"},
		},
		{
			name: "invalid field name - should fail with error",
			manifest: `{
				"manifestVersion": 1,
				"id": "test-enricher",
				"name": "Test Enricher",
				"version": "1.0.0",
				"capabilities": {
					"metadataEnricher": {
						"fileTypes": ["epub"],
						"fields": ["title", "invalid_field"]
					}
				}
			}`,
			mainJS: `var plugin = (function() {
				return { metadataEnricher: { enrich: function() { return { modified: false }; } } };
			})();`,
			wantErr: `invalid metadata field "invalid_field"`,
		},
		{
			name: "missing fields - enricher disabled with warning",
			manifest: `{
				"manifestVersion": 1,
				"id": "test-enricher",
				"name": "Test Enricher",
				"version": "1.0.0",
				"capabilities": {
					"metadataEnricher": {
						"fileTypes": ["epub"]
					}
				}
			}`,
			mainJS: `var plugin = (function() {
				return { metadataEnricher: { enrich: function() { return { modified: false }; } } };
			})();`,
			wantErr:     "",
			wantWarning: "metadataEnricher requires fields declaration",
		},
		{
			name: "empty fields array - enricher disabled with warning",
			manifest: `{
				"manifestVersion": 1,
				"id": "test-enricher",
				"name": "Test Enricher",
				"version": "1.0.0",
				"capabilities": {
					"metadataEnricher": {
						"fileTypes": ["epub"],
						"fields": []
					}
				}
			}`,
			mainJS: `var plugin = (function() {
				return { metadataEnricher: { enrich: function() { return { modified: false }; } } };
			})();`,
			wantErr:     "",
			wantWarning: "metadataEnricher requires fields declaration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with plugin files
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(tt.manifest), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(tt.mainJS), 0644))

			rt, err := LoadPlugin(dir, "test", "test-enricher")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, rt)

			if len(tt.wantFields) > 0 {
				// Valid fields case: enricher should be enabled
				enricherCap := rt.Manifest().Capabilities.MetadataEnricher
				require.NotNil(t, enricherCap)
				assert.Equal(t, tt.wantFields, enricherCap.Fields)
				assert.Empty(t, rt.LoadWarning())
				assert.Contains(t, rt.HookTypes(), "metadataEnricher")
			}

			if tt.wantWarning != "" {
				// Missing fields case: enricher should be disabled with warning
				assert.Equal(t, tt.wantWarning, rt.LoadWarning())
				// Enricher hook should not be in the hook types
				assert.NotContains(t, rt.HookTypes(), "metadataEnricher")
			}
		})
	}
}
