package downloadcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCleanup(t *testing.T) {
	t.Run("does nothing when under max size", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create two cached files
		createCachedFile(t, tmpDir, 1, "epub", 1000, time.Now().Add(-1*time.Hour))
		createCachedFile(t, tmpDir, 2, "epub", 1000, time.Now())

		// Max size is 10KB - we have 2KB
		err := RunCleanup(tmpDir, 10*1024)
		require.NoError(t, err)

		// Both files should still exist
		entries, err := ListCacheEntries(tmpDir)
		require.NoError(t, err)
		assert.Len(t, entries, 2)
	})

	t.Run("removes oldest files when over max size", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create three cached files with different access times
		// Total: 3000 bytes
		createCachedFile(t, tmpDir, 1, "epub", 1000, time.Now().Add(-3*time.Hour)) // oldest
		createCachedFile(t, tmpDir, 2, "epub", 1000, time.Now().Add(-2*time.Hour)) // middle
		createCachedFile(t, tmpDir, 3, "epub", 1000, time.Now().Add(-1*time.Hour)) // newest

		// Max size is 2KB - need to remove at least one file to get to 80% (1600 bytes)
		err := RunCleanup(tmpDir, 2000)
		require.NoError(t, err)

		// Should have removed the oldest file(s)
		entries, err := ListCacheEntries(tmpDir)
		require.NoError(t, err)

		// After removing 1 file (1000 bytes), we have 2000 bytes
		// But target is 80% of 2000 = 1600 bytes
		// So we need to remove 2 files to get to 1000 bytes
		assert.LessOrEqual(t, len(entries), 2)

		// The newest file should still exist
		newest := false
		for _, e := range entries {
			if e.FileID == 3 {
				newest = true
			}
		}
		assert.True(t, newest, "newest file should be preserved")
	})

	t.Run("handles empty cache directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := RunCleanup(tmpDir, 1000)
		require.NoError(t, err)
	})

	t.Run("removes files to reach 80% threshold", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files totaling 10000 bytes
		createCachedFile(t, tmpDir, 1, "epub", 2000, time.Now().Add(-5*time.Hour))
		createCachedFile(t, tmpDir, 2, "epub", 2000, time.Now().Add(-4*time.Hour))
		createCachedFile(t, tmpDir, 3, "epub", 2000, time.Now().Add(-3*time.Hour))
		createCachedFile(t, tmpDir, 4, "epub", 2000, time.Now().Add(-2*time.Hour))
		createCachedFile(t, tmpDir, 5, "epub", 2000, time.Now().Add(-1*time.Hour))

		// Total: 10000 bytes, Max: 8000 bytes, Target: 6400 bytes (80%)
		// Need to remove at least 3600 bytes = 2 files
		err := RunCleanup(tmpDir, 8000)
		require.NoError(t, err)

		// Should have at most 3 files remaining (6000 bytes < 6400)
		entries, err := ListCacheEntries(tmpDir)
		require.NoError(t, err)

		totalSize := int64(0)
		for _, e := range entries {
			totalSize += e.SizeBytes
		}

		assert.LessOrEqual(t, totalSize, int64(6400))
	})
}

func TestRunCleanupWithStats(t *testing.T) {
	t.Run("returns correct statistics", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files
		createCachedFile(t, tmpDir, 1, "epub", 1000, time.Now().Add(-2*time.Hour))
		createCachedFile(t, tmpDir, 2, "epub", 1000, time.Now().Add(-1*time.Hour))

		// Max size is 1500, target is 1200 (80%)
		// Need to remove 1 file (1000 bytes)
		stats, err := RunCleanupWithStats(tmpDir, 1500)
		require.NoError(t, err)

		assert.Equal(t, 1, stats.FilesRemoved)
		assert.Equal(t, int64(1000), stats.BytesRemoved)
		assert.Equal(t, 1, stats.FilesRemained)
		assert.Equal(t, int64(1000), stats.BytesRemained)
	})

	t.Run("returns zero stats when no cleanup needed", func(t *testing.T) {
		tmpDir := t.TempDir()

		createCachedFile(t, tmpDir, 1, "epub", 1000, time.Now())

		// Max size is way larger than actual
		stats, err := RunCleanupWithStats(tmpDir, 100000)
		require.NoError(t, err)

		assert.Equal(t, 0, stats.FilesRemoved)
		assert.Equal(t, int64(0), stats.BytesRemoved)
		assert.Equal(t, 1, stats.FilesRemained)
		assert.Equal(t, int64(1000), stats.BytesRemained)
	})
}

// createCachedFile creates a fake cached file and its metadata for testing.
//
//nolint:unparam // ext is always "epub" in tests but kept for future flexibility
func createCachedFile(t *testing.T, cacheDir string, fileID int, ext string, sizeBytes int64, lastAccessed time.Time) {
	t.Helper()

	// Create the cached file with the specified size
	filePath := filepath.Join(cacheDir, filepath.Base(cachedFilename(cacheDir, fileID, ext)))
	data := make([]byte, sizeBytes)
	err := os.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	// Create the metadata
	meta := &CacheMetadata{
		FileID:          fileID,
		FingerprintHash: "test-hash",
		GeneratedAt:     lastAccessed.Add(-1 * time.Minute),
		LastAccessedAt:  lastAccessed,
		SizeBytes:       sizeBytes,
	}
	err = WriteMetadata(cacheDir, meta)
	require.NoError(t, err)
}
