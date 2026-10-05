//go:build unix

package plugins

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test changes the process umask and sets AllowedDownloadHosts, so it
// must not call t.Parallel().
func TestInstaller_StagePackage_StagingDirModeComesFromCreation(t *testing.T) {
	for umask, want := range map[int]os.FileMode{0o022: 0o755, 0o077: 0o700} {
		testumask.Set(t, umask)
		assert.Equal(t, want, stagedPackageDirMode(t), "umask %#o", umask)
	}
}

func stagedPackageDirMode(t *testing.T) os.FileMode {
	t.Helper()
	zipData := createPluginZip(t, &Manifest{ManifestVersion: 1, ID: "test-plugin", Name: "Test Plugin", Version: "1.0.0"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(zipData)
	}))
	defer server.Close()
	origHosts := AllowedDownloadHosts
	AllowedDownloadHosts = []string{server.URL}
	defer func() { AllowedDownloadHosts = origHosts }()

	inst := NewInstaller(t.TempDir())
	pkg, err := inst.stagePackage(context.Background(), "shisho", "test-plugin", server.URL+"/test-plugin.zip", sha256Hex(zipData))
	require.NoError(t, err)
	defer pkg.remove()

	info, err := os.Stat(pkg.dir)
	require.NoError(t, err)
	return info.Mode().Perm()
}
