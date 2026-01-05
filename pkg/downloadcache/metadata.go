package downloadcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// CacheMetadata stores information about a cached file.
type CacheMetadata struct {
	FileID          int       `json:"file_id"`
	FingerprintHash string    `json:"fingerprint_hash"`
	GeneratedAt     time.Time `json:"generated_at"`
	LastAccessedAt  time.Time `json:"last_accessed_at"`
	SizeBytes       int64     `json:"size_bytes"`
}

// metadataFilename returns the metadata file path for a given file ID.
func metadataFilename(cacheDir string, fileID int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%d.meta.json", fileID))
}

// cachedFilename returns the cached file path for a given file ID and extension.
func cachedFilename(cacheDir string, fileID int, ext string) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%d.%s", fileID, ext))
}

// ReadMetadata reads the cache metadata for a file ID.
// Returns nil if the metadata file doesn't exist.
func ReadMetadata(cacheDir string, fileID int) (*CacheMetadata, error) {
	path := metadataFilename(cacheDir, fileID)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to read cache metadata: %s", path)
	}

	var meta CacheMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, errors.Wrapf(err, "failed to parse cache metadata: %s", path)
	}

	return &meta, nil
}

// WriteMetadata writes the cache metadata for a file ID.
func WriteMetadata(cacheDir string, meta *CacheMetadata) error {
	path := metadataFilename(cacheDir, meta.FileID)

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal cache metadata")
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return errors.Wrapf(err, "failed to write cache metadata: %s", path)
	}

	return nil
}

// UpdateLastAccessed updates the last accessed time for a cached file.
func UpdateLastAccessed(cacheDir string, fileID int) error {
	meta, err := ReadMetadata(cacheDir, fileID)
	if err != nil {
		return err
	}
	if meta == nil {
		return errors.New("cache metadata not found")
	}

	meta.LastAccessedAt = time.Now()
	return WriteMetadata(cacheDir, meta)
}

// DeleteCachedFile removes both the cached file and its metadata.
func DeleteCachedFile(cacheDir string, fileID int, ext string) error {
	// Delete the cached file
	cachedPath := cachedFilename(cacheDir, fileID, ext)
	if err := os.Remove(cachedPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to delete cached file: %s", cachedPath)
	}

	// Delete the metadata file
	metaPath := metadataFilename(cacheDir, fileID)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to delete cache metadata: %s", metaPath)
	}

	return nil
}

// GetCachedFilePath returns the path to a cached file if it exists and is valid.
// Returns empty string if the cache is invalid or doesn't exist.
func GetCachedFilePath(cacheDir string, fileID int, ext string, currentHash string) (string, error) {
	meta, err := ReadMetadata(cacheDir, fileID)
	if err != nil {
		return "", err
	}

	// No metadata means no cached file
	if meta == nil {
		return "", nil
	}

	// Check if fingerprint matches
	if meta.FingerprintHash != currentHash {
		return "", nil
	}

	// Check if cached file exists
	cachedPath := cachedFilename(cacheDir, fileID, ext)
	if _, err := os.Stat(cachedPath); os.IsNotExist(err) {
		return "", nil
	}

	return cachedPath, nil
}

// ListCacheEntries returns all cache entries in the directory.
func ListCacheEntries(cacheDir string) ([]*CacheMetadata, error) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to read cache directory: %s", cacheDir)
	}

	var results []*CacheMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .meta.json files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(cacheDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files we can't read
		}

		var meta CacheMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue // Skip invalid metadata files
		}

		results = append(results, &meta)
	}

	return results, nil
}

// GetTotalCacheSize returns the total size of all cached files in bytes.
func GetTotalCacheSize(cacheDir string) (int64, error) {
	entries, err := ListCacheEntries(cacheDir)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, entry := range entries {
		total += entry.SizeBytes
	}

	return total, nil
}
