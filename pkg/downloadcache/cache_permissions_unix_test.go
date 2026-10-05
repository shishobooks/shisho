//go:build unix

package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testumask"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file change the process umask, so none of them may call
// t.Parallel().

func TestCache_GetOrGenerate_M4BWorldReadableMetadataPrivate(t *testing.T) {
	testgen.SkipIfNoFFmpeg(t)
	testumask.Set(t, 0o022)
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0755))

	srcPath := testgen.GenerateM4B(t, tmpDir, "source.m4b", testgen.M4BOptions{
		Title:    "Original Title",
		Artist:   "Original Author",
		Duration: 1.0,
	})
	require.NoError(t, os.Chmod(srcPath, 0o600))

	cache := NewCache(cacheDir, 1024*1024*1024)
	t.Cleanup(cache.Wait)

	book := &models.Book{
		Title: "Test Book",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "Test Author"}},
		},
	}
	file := &models.File{ID: 1, FileType: models.FileTypeM4B, Filepath: srcPath}

	cachedPath, _, err := cache.GetOrGenerate(context.Background(), book, file)
	require.NoError(t, err)

	info, err := os.Stat(cachedPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	// The cache metadata stays private.
	metaInfo, err := os.Stat(metadataFilename(cacheDir, file.ID))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), metaInfo.Mode().Perm())
}
