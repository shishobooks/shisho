package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createPluginZip creates a ZIP archive in memory containing a manifest.json and main.js.
func createPluginZip(t *testing.T, manifest *Manifest) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// Write manifest.json
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)

	f, err := w.Create("manifest.json")
	require.NoError(t, err)
	_, err = f.Write(manifestData)
	require.NoError(t, err)

	// Write main.js
	f, err = w.Create("main.js")
	require.NoError(t, err)
	_, err = f.Write([]byte(`export function onMetadataEnrich(ctx) { return ctx; }`))
	require.NoError(t, err)

	require.NoError(t, w.Close())
	return buf.Bytes()
}

// sha256Hex computes the hex-encoded SHA256 of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestInstaller_InstallPlugin_Success(t *testing.T) {
	manifest := &Manifest{
		ManifestVersion: 1,
		ID:              "test-plugin",
		Name:            "Test Plugin",
		Version:         "1.0.0",
		Description:     "A test plugin",
		Author:          "Test Author",
	}

	zipData := createPluginZip(t, manifest)
	checksum := sha256Hex(zipData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer server.Close()

	// Override allowed hosts to allow test server
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	result, err := inst.InstallPlugin(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", checksum)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", result.ID)
	assert.Equal(t, "Test Plugin", result.Name)
	assert.Equal(t, "1.0.0", result.Version)

	// Verify files were extracted
	manifestPath := filepath.Join(pluginDir, "shisho", "test-plugin", "manifest.json")
	assert.FileExists(t, manifestPath)

	mainJsPath := filepath.Join(pluginDir, "shisho", "test-plugin", "main.js")
	assert.FileExists(t, mainJsPath)
}

func TestInstaller_InstallPlugin_BadChecksum(t *testing.T) {
	manifest := &Manifest{
		ManifestVersion: 1,
		ID:              "test-plugin",
		Name:            "Test Plugin",
		Version:         "1.0.0",
	}

	zipData := createPluginZip(t, manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer server.Close()

	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	_, err := inst.InstallPlugin(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", "wrong-checksum")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA256 mismatch")

	// Verify no files were left behind
	pluginPath := filepath.Join(pluginDir, "shisho", "test-plugin")
	_, statErr := os.Stat(pluginPath)
	assert.True(t, os.IsNotExist(statErr))
}

func TestInstaller_InstallPlugin_InvalidURL(t *testing.T) {
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{"https://github.com/"}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	_, err := inst.InstallPlugin(context.Background(), "shisho", "test-plugin", "https://evil.com/plugin.zip", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid download URL")
}

func TestInstaller_UninstallPlugin(t *testing.T) {
	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	// Create a plugin directory with some files
	pluginPath := filepath.Join(pluginDir, "shisho", "test-plugin")
	require.NoError(t, os.MkdirAll(pluginPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "main.js"), []byte(""), 0644))

	err := inst.UninstallPlugin("shisho", "test-plugin")
	require.NoError(t, err)

	// Verify directory was removed
	_, err = os.Stat(pluginPath)
	assert.True(t, os.IsNotExist(err))
}

func TestInstaller_UninstallPlugin_NotExists(t *testing.T) {
	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	// Should not error when directory doesn't exist
	err := inst.UninstallPlugin("shisho", "nonexistent")
	require.NoError(t, err)
}

func TestInstaller_UpdatePlugin(t *testing.T) {
	manifest := &Manifest{
		ManifestVersion: 1,
		ID:              "test-plugin",
		Name:            "Test Plugin Updated",
		Version:         "2.0.0",
		Description:     "Updated plugin",
		Author:          "Test Author",
	}

	zipData := createPluginZip(t, manifest)
	checksum := sha256Hex(zipData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer server.Close()

	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	// Create an existing plugin directory (old version)
	pluginPath := filepath.Join(pluginDir, "shisho", "test-plugin")
	require.NoError(t, os.MkdirAll(pluginPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "old-file.txt"), []byte("old content"), 0644))

	result, err := inst.UpdatePlugin(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", checksum)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", result.ID)
	assert.Equal(t, "Test Plugin Updated", result.Name)
	assert.Equal(t, "2.0.0", result.Version)

	// Verify new files exist
	manifestPath := filepath.Join(pluginPath, "manifest.json")
	assert.FileExists(t, manifestPath)

	// Verify old files were removed
	oldFilePath := filepath.Join(pluginPath, "old-file.txt")
	_, err = os.Stat(oldFilePath)
	assert.True(t, os.IsNotExist(err))
}

func TestInstaller_UpdatePlugin_BadChecksum(t *testing.T) {
	manifest := &Manifest{
		ManifestVersion: 1,
		ID:              "test-plugin",
		Name:            "Test Plugin",
		Version:         "2.0.0",
	}

	zipData := createPluginZip(t, manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(zipData)
	}))
	defer server.Close()

	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	// Create an existing plugin directory
	pluginPath := filepath.Join(pluginDir, "shisho", "test-plugin")
	require.NoError(t, os.MkdirAll(pluginPath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte("{}"), 0644))

	_, err := inst.UpdatePlugin(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", "bad-checksum")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA256 mismatch")

	// Original files should still be intact
	assert.FileExists(t, filepath.Join(pluginPath, "manifest.json"))
}
