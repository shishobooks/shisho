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

func TestInstaller_StagePackage_Success(t *testing.T) {
	manifest := &Manifest{
		ManifestVersion: 1,
		ID:              "test-plugin",
		Name:            "Test Plugin",
		Version:         "1.0.0",
		Description:     "A test plugin",
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

	pkg, err := inst.stagePackage(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", checksum)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", pkg.manifest.ID)
	assert.Equal(t, "Test Plugin", pkg.manifest.Name)
	assert.Equal(t, "1.0.0", pkg.manifest.Version)

	// The package is extracted under .staging on the plugin directory's
	// filesystem, and nothing is installed yet.
	assert.Equal(t, filepath.Join(pluginDir, "shisho", stagingDirName), filepath.Dir(pkg.dir))
	assert.FileExists(t, filepath.Join(pkg.dir, "manifest.json"))
	assert.FileExists(t, filepath.Join(pkg.dir, "main.js"))
	assert.NoDirExists(t, filepath.Join(pluginDir, "shisho", "test-plugin"))

	pkg.remove()
	assert.NoDirExists(t, pkg.dir)
}

func TestInstaller_StagePackage_BadChecksum(t *testing.T) {
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

	_, err := inst.stagePackage(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", "wrong-checksum")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA256 mismatch")

	// Verify no files were left behind
	entries, err := os.ReadDir(pluginDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestInstaller_StagePackage_InvalidURL(t *testing.T) {
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{"https://github.com/"}
	defer func() { AllowedDownloadHosts = origHosts }()

	pluginDir := t.TempDir()
	inst := NewInstaller(pluginDir)

	_, err := inst.stagePackage(context.Background(), "shisho", "test-plugin", "https://evil.com/plugin.zip", "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid download URL")
}

// stageDir writes a directory under .staging holding one file.
func stageDir(t *testing.T, pluginDir, name, content string) string {
	t.Helper()
	dir := filepath.Join(pluginDir, "shisho", stagingDirName, "pkg")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	return dir
}

// swapIn replaces the whole live directory, and commit deletes the old one.
func TestSwapIn_Commit(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	live := filepath.Join(pluginDir, "shisho", "p")
	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "old.txt"), []byte("old"), 0o644))

	swap, err := swapIn(pluginDir, "shisho", "p", stageDir(t, pluginDir, "new.txt", "new"))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(live, "new.txt"))
	assert.NoFileExists(t, filepath.Join(live, "old.txt"))

	swap.commit()
	entries, err := os.ReadDir(filepath.Join(pluginDir, "shisho", trashDirName))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// rollback puts the replaced directory back, and removes a directory that
// replaced nothing.
func TestSwapIn_Rollback(t *testing.T) {
	t.Parallel()
	pluginDir := t.TempDir()
	live := filepath.Join(pluginDir, "shisho", "p")
	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "old.txt"), []byte("old"), 0o644))

	swap, err := swapIn(pluginDir, "shisho", "p", stageDir(t, pluginDir, "new.txt", "new"))
	require.NoError(t, err)
	require.NoError(t, swap.rollback())
	assert.FileExists(t, filepath.Join(live, "old.txt"))
	assert.NoFileExists(t, filepath.Join(live, "new.txt"))

	fresh, err := swapIn(pluginDir, "shisho", "fresh", stageDir(t, pluginDir, "new.txt", "new"))
	require.NoError(t, err)
	require.NoError(t, fresh.rollback())
	assert.NoDirExists(t, filepath.Join(pluginDir, "shisho", "fresh"))
}

func TestIsAllowedDownloadURL_AcceptsLocalhostWhenConfigured(t *testing.T) {
	// Not parallel: mutates package-level AllowedDownloadHosts, which is also
	// mutated by other tests in this file. Running in parallel would race.
	orig := AllowedDownloadHosts
	defer func() { AllowedDownloadHosts = orig }()
	AllowedDownloadHosts = append(orig, "http://127.0.0.1:")

	assert.True(t, isAllowedDownloadURL("http://127.0.0.1:9876/test/plugins/fixture.zip"))
	assert.False(t, isAllowedDownloadURL("http://evil.example.com/x.zip"))
}
