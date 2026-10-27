package downloadcache

import (
	"context"
	"io"
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

			path, filename, err := cache.GetOrGenerate(context.Background(), book, file, file.FileType)
			require.NoError(t, err)
			assert.Equal(t, "First."+tc.fileType, filename)
			meta, err := mobi.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, "First", meta.Title)

			book.Title = "Second"
			path, _, err = cache.GetOrGenerate(context.Background(), book, file, file.FileType)
			require.NoError(t, err)
			meta, err = mobi.Parse(path)
			require.NoError(t, err)
			assert.Equal(t, "Second", meta.Title)
		})
	}
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

// An AZW3 converted from an EPUB lists every record's offset before the
// records, so it is built in full before it is sent: the first download
// has its exact length, and the next is served from the cache.
func TestCache_Open_AZW3IsBuiltBeforeItIsSent(t *testing.T) {
	t.Parallel()
	cache := NewCache(t.TempDir(), 1<<30)
	t.Cleanup(cache.Wait)
	srcPath := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{Title: "In File"})
	book := &models.Book{Title: "Converted"}
	file := &models.File{ID: 7, FileType: models.FileTypeEPUB, Filepath: srcPath}

	head, err := cache.Head(context.Background(), book, file, models.FileTypeAZW3)
	require.NoError(t, err)
	assert.Positive(t, head.Size, "a HEAD request knows the size")

	first, err := cache.Open(context.Background(), book, file, models.FileTypeAZW3)
	require.NoError(t, err)
	assert.Equal(t, "Converted.azw3", first.Filename)
	var data []byte
	if first.Path != "" {
		data, err = os.ReadFile(first.Path)
	} else {
		require.NotNil(t, first.Body)
		data, err = io.ReadAll(first.Body)
		first.Body.Close()
		assert.Equal(t, first.Size, int64(len(data)), "sent with its exact length")
	}
	require.NoError(t, err)
	assert.Equal(t, "BOOKMOBI", string(data[60:68]))
	cache.Wait()

	second, err := cache.Open(context.Background(), book, file, models.FileTypeAZW3)
	require.NoError(t, err)
	require.NotEmpty(t, second.Path, "served from the cache")
	assert.Nil(t, second.Body)
	cached, err := os.ReadFile(second.Path)
	require.NoError(t, err)
	assert.Equal(t, data, cached)
	meta, err := mobi.Parse(second.Path)
	require.NoError(t, err)
	assert.Equal(t, "Converted", meta.Title)
}
