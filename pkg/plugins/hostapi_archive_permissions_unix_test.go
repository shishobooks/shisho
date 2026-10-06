//go:build unix

package plugins

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test changes the process umask, so it must not call t.Parallel().
func TestArchive_ExtractZip_KeepsOnlyOwnerUsablePermissionBits(t *testing.T) {
	testumask.Set(t, 0o022)

	// Each entry's stored mode, and the mode its extracted file should get.
	entries := map[string]struct{ stored, want os.FileMode }{
		"setuid.sh":   {os.ModeSetuid | 0o755, 0o755},
		"setgid.sh":   {os.ModeSetgid | 0o755, 0o755},
		"sticky.sh":   {os.ModeSticky | 0o755, 0o755},
		"readonly.md": {0o444, 0o644},
		"nomode.txt":  {0, 0o600},
	}

	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "modes.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for name, entry := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(entry.stored)
		// Empty entries: writing content as an unprivileged user makes the
		// kernel clear setuid and setgid, which would hide the bug.
		_, err := w.CreateHeader(header)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	destDir := filepath.Join(tmpDir, "out")
	require.NoError(t, doExtractZip(zipPath, destDir))
	// Extracting again overwrites every file, which fails if any is unwritable.
	require.NoError(t, doExtractZip(zipPath, destDir))

	for name, entry := range entries {
		info, err := os.Stat(filepath.Join(destDir, name))
		require.NoError(t, err)
		assert.Equal(t, entry.want, info.Mode(), name)
	}
}
