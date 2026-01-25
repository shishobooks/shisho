package plugins

import (
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
