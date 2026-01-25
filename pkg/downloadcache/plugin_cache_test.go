package downloadcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginCachedFilename(t *testing.T) {
	path := pluginCachedFilename("/cache", 42, "mobi")
	assert.Equal(t, filepath.Join("/cache", "42.plugin.mobi"), path)
}

func TestPluginMetadataFilename(t *testing.T) {
	path := pluginMetadataFilename("/cache", 42, "mobi")
	assert.Equal(t, filepath.Join("/cache", "42.plugin.mobi.meta.json"), path)
}

func TestWriteAndReadPluginMetadata(t *testing.T) {
	cacheDir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &CacheMetadata{
		FileID:          1,
		Format:          "plugin:mobi",
		FingerprintHash: "abc123",
		GeneratedAt:     now,
		LastAccessedAt:  now,
		SizeBytes:       1024,
	}

	err := WritePluginMetadata(cacheDir, 1, "mobi", meta)
	require.NoError(t, err)

	// Verify metadata file exists
	metaPath := pluginMetadataFilename(cacheDir, 1, "mobi")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)

	// Read it back
	readMeta, err := ReadPluginMetadata(cacheDir, 1, "mobi")
	require.NoError(t, err)
	require.NotNil(t, readMeta)
	assert.Equal(t, 1, readMeta.FileID)
	assert.Equal(t, "plugin:mobi", readMeta.Format)
	assert.Equal(t, "abc123", readMeta.FingerprintHash)
	assert.Equal(t, int64(1024), readMeta.SizeBytes)
}

func TestReadPluginMetadata_NotFound(t *testing.T) {
	cacheDir := t.TempDir()

	meta, err := ReadPluginMetadata(cacheDir, 999, "mobi")
	require.NoError(t, err)
	assert.Nil(t, meta)
}

func TestUpdatePluginLastAccessed(t *testing.T) {
	cacheDir := t.TempDir()

	past := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	meta := &CacheMetadata{
		FileID:          1,
		Format:          "plugin:mobi",
		FingerprintHash: "abc123",
		GeneratedAt:     past,
		LastAccessedAt:  past,
		SizeBytes:       1024,
	}
	err := WritePluginMetadata(cacheDir, 1, "mobi", meta)
	require.NoError(t, err)

	// Update last accessed
	err = UpdatePluginLastAccessed(cacheDir, 1, "mobi")
	require.NoError(t, err)

	// Read back and check it was updated
	readMeta, err := ReadPluginMetadata(cacheDir, 1, "mobi")
	require.NoError(t, err)
	assert.True(t, readMeta.LastAccessedAt.After(past))
}

func TestUpdatePluginLastAccessed_NotFound(t *testing.T) {
	cacheDir := t.TempDir()

	err := UpdatePluginLastAccessed(cacheDir, 999, "mobi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetPluginCachedFilePath(t *testing.T) {
	cacheDir := t.TempDir()

	t.Run("returns path when cache is valid", func(t *testing.T) {
		// Write metadata
		meta := &CacheMetadata{
			FileID:          1,
			Format:          "plugin:mobi",
			FingerprintHash: "valid-hash",
			GeneratedAt:     time.Now(),
			LastAccessedAt:  time.Now(),
			SizeBytes:       512,
		}
		err := WritePluginMetadata(cacheDir, 1, "mobi", meta)
		require.NoError(t, err)

		// Create the cached file
		cachedPath := pluginCachedFilename(cacheDir, 1, "mobi")
		err = os.WriteFile(cachedPath, []byte("cached content"), 0644)
		require.NoError(t, err)

		// Should return the path
		result, err := GetPluginCachedFilePath(cacheDir, 1, "mobi", "valid-hash")
		require.NoError(t, err)
		assert.Equal(t, cachedPath, result)
	})

	t.Run("returns empty when hash mismatch", func(t *testing.T) {
		result, err := GetPluginCachedFilePath(cacheDir, 1, "mobi", "wrong-hash")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns empty when no metadata", func(t *testing.T) {
		result, err := GetPluginCachedFilePath(cacheDir, 999, "mobi", "any-hash")
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("returns empty when file missing", func(t *testing.T) {
		// Write metadata with different format
		meta := &CacheMetadata{
			FileID:          2,
			Format:          "plugin:pdf",
			FingerprintHash: "pdf-hash",
			GeneratedAt:     time.Now(),
			LastAccessedAt:  time.Now(),
			SizeBytes:       100,
		}
		err := WritePluginMetadata(cacheDir, 2, "pdf", meta)
		require.NoError(t, err)

		// Don't create the cached file
		result, err := GetPluginCachedFilePath(cacheDir, 2, "pdf", "pdf-hash")
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestDeletePluginCachedFile(t *testing.T) {
	cacheDir := t.TempDir()

	// Create cached file and metadata
	cachedPath := pluginCachedFilename(cacheDir, 1, "mobi")
	err := os.WriteFile(cachedPath, []byte("content"), 0644)
	require.NoError(t, err)

	meta := &CacheMetadata{
		FileID:          1,
		Format:          "plugin:mobi",
		FingerprintHash: "hash",
		GeneratedAt:     time.Now(),
		LastAccessedAt:  time.Now(),
		SizeBytes:       7,
	}
	err = WritePluginMetadata(cacheDir, 1, "mobi", meta)
	require.NoError(t, err)

	// Delete
	err = DeletePluginCachedFile(cacheDir, 1, "mobi")
	require.NoError(t, err)

	// Both should be gone
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err))

	metaPath := pluginMetadataFilename(cacheDir, 1, "mobi")
	_, err = os.Stat(metaPath)
	assert.True(t, os.IsNotExist(err))
}

func TestDeletePluginCachedFile_NotExist(t *testing.T) {
	cacheDir := t.TempDir()

	// Should not error when files don't exist
	err := DeletePluginCachedFile(cacheDir, 999, "mobi")
	require.NoError(t, err)
}

func TestFormatPluginDownloadFilename(t *testing.T) {
	tests := []struct {
		name     string
		book     *models.Book
		file     *models.File
		formatID string
		expected string
	}{
		{
			name: "replaces extension with format ID",
			book: &models.Book{
				Title: "Test Book",
				Authors: []*models.Author{
					{SortOrder: 0, Person: &models.Person{Name: "Author"}},
				},
			},
			file:     &models.File{FileType: "epub"},
			formatID: "mobi",
			expected: "[Author] Test Book.mobi",
		},
		{
			name: "works with cbz source",
			book: &models.Book{
				Title: "Manga Vol 1",
			},
			file:     &models.File{FileType: "cbz"},
			formatID: "pdf",
			expected: "Manga Vol 001.pdf",
		},
		{
			name: "uses file.Name when available",
			book: &models.Book{
				Title: "Book Title",
			},
			file: &models.File{
				FileType: "epub",
				Name:     strPtr("Custom Name"),
			},
			formatID: "mobi",
			expected: "Custom Name.mobi",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatPluginDownloadFilename(tc.book, tc.file, tc.formatID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFingerprint_PluginFingerprint(t *testing.T) {
	t.Run("plugin fingerprint affects hash", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:             "Test",
			Format:            "plugin:mobi",
			PluginFingerprint: "version-1",
			Authors:           []FingerprintAuthor{},
			Narrators:         []FingerprintNarrator{},
			Series:            []FingerprintSeries{},
			Genres:            []string{},
			Tags:              []string{},
			Chapters:          []FingerprintChapter{},
		}
		fp2 := &Fingerprint{
			Title:             "Test",
			Format:            "plugin:mobi",
			PluginFingerprint: "version-2",
			Authors:           []FingerprintAuthor{},
			Narrators:         []FingerprintNarrator{},
			Series:            []FingerprintSeries{},
			Genres:            []string{},
			Tags:              []string{},
			Chapters:          []FingerprintChapter{},
		}

		hash1, err := fp1.Hash()
		require.NoError(t, err)
		hash2, err := fp2.Hash()
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "different plugin fingerprints should produce different hashes")
	})

	t.Run("empty plugin fingerprint is same as omitted", func(t *testing.T) {
		fp1 := &Fingerprint{
			Title:             "Test",
			Format:            "original",
			PluginFingerprint: "",
			Authors:           []FingerprintAuthor{},
			Narrators:         []FingerprintNarrator{},
			Series:            []FingerprintSeries{},
			Genres:            []string{},
			Tags:              []string{},
			Chapters:          []FingerprintChapter{},
		}
		fp2 := &Fingerprint{
			Title:     "Test",
			Format:    "original",
			Authors:   []FingerprintAuthor{},
			Narrators: []FingerprintNarrator{},
			Series:    []FingerprintSeries{},
			Genres:    []string{},
			Tags:      []string{},
			Chapters:  []FingerprintChapter{},
		}

		hash1, err := fp1.Hash()
		require.NoError(t, err)
		hash2, err := fp2.Hash()
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "empty plugin fingerprint should produce same hash as omitted")
	})
}

func TestCache_InvalidatePlugin(t *testing.T) {
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 1024*1024*1024)

	// Create cached file and metadata
	cachedPath := pluginCachedFilename(cacheDir, 1, "mobi")
	err := os.WriteFile(cachedPath, []byte("content"), 0644)
	require.NoError(t, err)

	meta := &CacheMetadata{
		FileID:          1,
		Format:          "plugin:mobi",
		FingerprintHash: "hash",
		GeneratedAt:     time.Now(),
		LastAccessedAt:  time.Now(),
		SizeBytes:       7,
	}
	err = WritePluginMetadata(cacheDir, 1, "mobi", meta)
	require.NoError(t, err)

	// Invalidate
	err = cache.InvalidatePlugin(1, "mobi")
	require.NoError(t, err)

	// Both should be gone
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err))
}
