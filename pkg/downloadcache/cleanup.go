package downloadcache

import (
	"os"
	"sort"

	"github.com/pkg/errors"
)

// CleanupThreshold is the percentage of maxSize to reduce the cache to during cleanup.
// For example, 0.8 means cleanup will reduce the cache to 80% of maxSize.
const CleanupThreshold = 0.8

// RunCleanup removes cached files until the total size is below the threshold.
// Files are removed in LRU (least recently used) order.
func RunCleanup(cacheDir string, maxSizeBytes int64) error {
	// Get current total size
	totalSize, err := GetTotalCacheSize(cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to get cache size")
	}

	// Check if cleanup is needed
	if totalSize <= maxSizeBytes {
		return nil
	}

	// Get all cache entries
	entries, err := ListCacheEntries(cacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to list cache entries")
	}

	if len(entries) == 0 {
		return nil
	}

	// Sort by last accessed time (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LastAccessedAt.Before(entries[j].LastAccessedAt)
	})

	// Calculate target size (80% of max)
	targetSize := int64(float64(maxSizeBytes) * CleanupThreshold)

	// Remove files until we're under the target
	for _, entry := range entries {
		if totalSize <= targetSize {
			break
		}

		// Try to determine the file extension from the cache
		// We need to find the actual file to know its extension
		ext := findCachedFileExtension(cacheDir, entry.FileID)
		if ext == "" {
			continue // Skip if we can't find the file
		}

		if err := DeleteCachedFile(cacheDir, entry.FileID, ext); err != nil {
			// Log error but continue with other files
			continue
		}

		totalSize -= entry.SizeBytes
	}

	return nil
}

// findCachedFileExtension finds the extension of a cached file by file ID.
func findCachedFileExtension(cacheDir string, fileID int) string {
	// Try common extensions
	extensions := []string{"epub", "m4b", "cbz"}
	for _, ext := range extensions {
		path := cachedFilename(cacheDir, fileID, ext)
		if _, err := os.Stat(path); err == nil {
			return ext
		}
	}
	return ""
}

// CleanupStats holds statistics about a cleanup operation.
type CleanupStats struct {
	FilesRemoved  int
	BytesRemoved  int64
	FilesRemained int
	BytesRemained int64
}

// RunCleanupWithStats performs cleanup and returns statistics.
func RunCleanupWithStats(cacheDir string, maxSizeBytes int64) (*CleanupStats, error) {
	stats := &CleanupStats{}

	// Get initial state
	entriesBefore, err := ListCacheEntries(cacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list cache entries")
	}

	var totalBefore int64
	for _, e := range entriesBefore {
		totalBefore += e.SizeBytes
	}

	// Run cleanup
	if err := RunCleanup(cacheDir, maxSizeBytes); err != nil {
		return nil, err
	}

	// Get final state
	entriesAfter, err := ListCacheEntries(cacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list cache entries after cleanup")
	}

	var totalAfter int64
	for _, e := range entriesAfter {
		totalAfter += e.SizeBytes
	}

	stats.FilesRemoved = len(entriesBefore) - len(entriesAfter)
	stats.BytesRemoved = totalBefore - totalAfter
	stats.FilesRemained = len(entriesAfter)
	stats.BytesRemained = totalAfter

	return stats, nil
}
