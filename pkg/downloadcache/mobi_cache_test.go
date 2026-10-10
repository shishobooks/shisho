package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetOrGenerate_MOBIRegeneratesOnMetadataChange(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		fileType string
		kind     testgen.MOBIKind
	}{
		{models.FileTypeMOBI, testgen.MOBIKindCombo},
		{models.FileTypeAZW3, testgen.MOBIKindKF8},
	} {
		t.Run(tc.fileType, func(t *testing.T) {
			t.Parallel()
			cacheDir := t.TempDir()
			cache := NewCache(cacheDir, 1<<30)
			t.Cleanup(cache.Wait)

			srcPath := testgen.GenerateMOBI(t, t.TempDir(), "book."+tc.fileType, testgen.MOBIOptions{Kind: tc.kind, Title: "In File"})
			book := &models.Book{Title: "First"}
			file := &models.File{ID: 7, FileType: tc.fileType, Filepath: srcPath}

			path, filename, err := cache.GetOrGenerate(context.Background(), book, file)
			require.NoError(t, err)
			assert.Equal(t, "First."+tc.fileType, filename)
			meta, err := mobi.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, "First", meta.Title)

			book.Title = "Second"
			path, _, err = cache.GetOrGenerate(context.Background(), book, file)
			require.NoError(t, err)
			meta, err = mobi.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, "Second", meta.Title)
		})
	}
}

// A plugin output generator may use "mobi" as its id. Its cache entry and
// metadata live under their own names, so it neither serves nor evicts the
// built-in MOBI download.
func TestPluginMOBIFormatDoesNotCollideWithBuiltIn(t *testing.T) {
	t.Parallel()
	assert.NotEqual(t, cachedFilename("/cache", 1, models.FileTypeMOBI), pluginCachedFilename("/cache", 1, models.FileTypeMOBI))
	assert.NotEqual(t, metadataFilename("/cache", 1), pluginMetadataFilename("/cache", 1, models.FileTypeMOBI))
}

func TestRunCleanup_EvictsEveryBuiltInType(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	old := time.Now().Add(-time.Hour)
	for i, fileType := range models.BuiltInFileTypes {
		createCachedFile(t, cacheDir, i+1, fileType, 1000, old)
	}

	require.NoError(t, RunCleanup(cacheDir, 100))

	for i, fileType := range models.BuiltInFileTypes {
		_, err := os.Stat(filepath.Join(cacheDir, filepath.Base(cachedFilename(cacheDir, i+1, fileType))))
		assert.True(t, os.IsNotExist(err), "%s entry was not evicted", fileType)
	}
}
