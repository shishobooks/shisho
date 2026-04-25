package cbzpages

import (
	"os"
	"path/filepath"
	"testing"

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
