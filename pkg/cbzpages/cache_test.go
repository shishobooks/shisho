package cbzpages

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_SizeBytes_Empty(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir())

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_MissingRoot(t *testing.T) {
	t.Parallel()
	c := NewCache(filepath.Join(t.TempDir(), "does-not-exist"))

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(0), bytes)
	assert.Equal(t, 0, count)
}

func TestCache_SizeBytes_CountsNestedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir)

	// Simulate cached pages under {dir}/cbz/{fileID}/
	pageDir := filepath.Join(dir, "cbz", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("abc"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_1.jpg"), []byte("defgh"), 0644))

	bytes, count, err := c.SizeBytes()
	require.NoError(t, err)
	assert.Equal(t, int64(8), bytes)
	assert.Equal(t, 2, count)
}

func TestCache_Clear_RemovesCbzSubtree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(dir)

	pageDir := filepath.Join(dir, "cbz", "42")
	require.NoError(t, os.MkdirAll(pageDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pageDir, "page_0.jpg"), []byte("x"), 0644))

	// An unrelated sibling should not be affected
	siblingDir := filepath.Join(dir, "pdf", "99")
	require.NoError(t, os.MkdirAll(siblingDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(siblingDir, "page_0.jpg"), []byte("y"), 0644))

	require.NoError(t, c.Clear())

	// cbz subtree gone
	_, err := os.Stat(filepath.Join(dir, "cbz"))
	assert.True(t, os.IsNotExist(err))

	// pdf sibling untouched
	_, err = os.Stat(filepath.Join(siblingDir, "page_0.jpg"))
	require.NoError(t, err)
}

func TestCache_Clear_IdempotentWhenMissing(t *testing.T) {
	t.Parallel()
	c := NewCache(t.TempDir())

	require.NoError(t, c.Clear())
}

// writeOldPage writes a cached page last used two hours ago, outside the
// window that protects pages a reader just fetched.
func writeOldPage(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, make([]byte, size), 0644))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
}

func TestCache_SetMaxBytes_EvictsLeastRecentlyUsedPages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	c := NewCache(filepath.Join(dir, "cache"))
	old := filepath.Join(dir, "cache", "cbz", "7", pageFileName(0, ".jpg"))
	writeOldPage(t, old, 600<<10)

	require.NoError(t, c.SetMaxBytes(1<<20))
	require.FileExists(t, old, "a cache under its limit evicts nothing")

	cbzPath := writeCBZ(t, dir, randomBytes(t, 600<<10))
	fresh, _, err := c.GetPage(cbzPath, 8, 0)
	require.NoError(t, err)
	assert.FileExists(t, fresh)
	assert.NoFileExists(t, old, "publishing a page over the limit evicts the least recently used page")

	require.NoError(t, c.SetMaxBytes(0))
	assert.FileExists(t, fresh, "a page a reader just fetched survives until it is served")
}
