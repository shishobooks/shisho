package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each File Type a File is delivered as has its own cache entry, and a
// metadata change regenerates every one of them.
func TestCache_DeliveredFileTypesCachedSeparately(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	srcPath := testgen.GenerateCBZ(t, tmpDir, "source.cbz", testgen.CBZOptions{})

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)
	book := &models.Book{Title: "Comic"}
	file := &models.File{ID: 7, FileType: models.FileTypeCBZ, Filepath: srcPath}
	ctx := context.Background()

	types := []string{models.FileTypeCBZ, models.FileTypeKePub, models.FileTypeEPUB}
	wantNames := map[string]string{
		models.FileTypeCBZ:   "Comic.cbz",
		models.FileTypeKePub: "Comic.kepub.epub",
		models.FileTypeEPUB:  "Comic.epub",
	}
	paths := map[string]string{}
	hashes := map[string]string{}
	for _, ft := range types {
		path, name, err := cache.GetOrGenerate(ctx, book, file, ft)
		require.NoError(t, err, ft)
		assert.Equal(t, wantNames[ft], name)
		paths[ft] = path
		meta, err := ReadMetadata(cacheDir, file.ID, ft)
		require.NoError(t, err)
		require.NotNil(t, meta, ft)
		assert.Equal(t, ft, meta.DeliveredFileType)
		hashes[ft] = meta.FingerprintHash
	}
	assert.Len(t, map[string]bool{paths[types[0]]: true, paths[types[1]]: true, paths[types[2]]: true}, 3, "each File Type has its own cached file")
	assert.Len(t, map[string]bool{hashes[types[0]]: true, hashes[types[1]]: true, hashes[types[2]]: true}, 3, "each File Type has its own fingerprint")
	for _, ft := range types {
		_, err := os.Stat(paths[ft])
		require.NoError(t, err, "%s is still cached after the others were generated", ft)
		path, _, err := cache.GetOrGenerate(ctx, book, file, ft)
		require.NoError(t, err)
		assert.Equal(t, paths[ft], path, "%s is served from the cache", ft)
	}

	book.Title = "Renamed Comic"
	for _, ft := range types {
		_, _, err := cache.GetOrGenerate(ctx, book, file, ft)
		require.NoError(t, err)
		meta, err := ReadMetadata(cacheDir, file.ID, ft)
		require.NoError(t, err)
		assert.NotEqual(t, hashes[ft], meta.FingerprintHash, "%s regenerated after the metadata change", ft)
	}
}

func TestCache_UnreachableFileTypeHasNoEntry(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	srcPath := testgen.GenerateCBZ(t, tmpDir, "source.cbz", testgen.CBZOptions{})

	cache := NewCache(cacheDir, 1<<30)
	t.Cleanup(cache.Wait)
	file := &models.File{ID: 7, FileType: models.FileTypeCBZ, Filepath: srcPath}

	_, err := cache.Open(context.Background(), &models.Book{Title: "Comic"}, file, models.FileTypePDF)
	require.ErrorIs(t, err, filegen.ErrNoConversion)
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Entries cached before each File Type had its own entry are never served,
// so the startup sweep removes them with their files, including a record of
// a failure only. Current entries and files that are not cache entries stay.
func TestRemoveLeftovers_RemovesEntriesFromBeforeDeliveredFileTypes(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	for _, name := range []string{
		"3.epub", "3.meta.json", "3.kepub.epub", "3.kepub.meta.json",
		"4.cbz", "4.meta.json", "5.meta.json",
		"6-epub", "6-epub.meta.json", "notes.txt",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(cacheDir, name), []byte("x"), 0o600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(cacheDir, "bulk"), 0o755))

	require.NoError(t, NewCache(cacheDir, 1<<30).RemoveLeftovers())

	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"6-epub", "6-epub.meta.json", "notes.txt", "bulk"}, names)
}
