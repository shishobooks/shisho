//go:build unix

package worker

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests change the process umask, so they must not call t.Parallel().

func TestWriteBulkZip_PublishesWorldReadableZip(t *testing.T) {
	testumask.Set(t, 0o022)
	dir := t.TempDir()
	bulkDir, zipPath := writeTestBulkZip(t, dir)

	info, err := os.Stat(zipPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	zr, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer zr.Close()
	require.Len(t, zr.File, 1)
	assert.Equal(t, "Book.epub", zr.File[0].Name)
	r, err := zr.File[0].Open()
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "book", string(got))

	entries, err := os.ReadDir(bulkDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no partial zip is left behind")
}

func TestWriteBulkZip_RespectsRestrictiveUmask(t *testing.T) {
	testumask.Set(t, 0o077)
	_, zipPath := writeTestBulkZip(t, t.TempDir())

	info, err := os.Stat(zipPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func writeTestBulkZip(t *testing.T, dir string) (bulkDir, zipPath string) {
	t.Helper()
	cached := filepath.Join(dir, "cached.epub")
	require.NoError(t, os.WriteFile(cached, []byte("book"), 0o600))
	bulkDir = filepath.Join(dir, "bulk")
	require.NoError(t, os.Mkdir(bulkDir, 0o755))
	zipPath = filepath.Join(bulkDir, "abc.zip")
	require.NoError(t, writeBulkZip(bulkDir, zipPath, map[int]string{1: cached}, map[int]string{1: "Book.epub"}))
	return bulkDir, zipPath
}
