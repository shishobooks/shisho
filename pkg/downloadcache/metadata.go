package downloadcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
)

// CacheMetadata stores information about a cached file.
type CacheMetadata struct {
	FileID int `json:"file_id"`
	// DeliveredFileType is the File Type the entry delivers its File as.
	// Each one has its own entry.
	DeliveredFileType string    `json:"delivered_file_type"`
	FingerprintHash   string    `json:"fingerprint_hash"`
	GeneratedAt       time.Time `json:"generated_at"`
	LastAccessedAt    time.Time `json:"last_accessed_at"`
	SizeBytes         int64     `json:"size_bytes"`
	// Failure records a build that failed after bytes were sent. It applies
	// only while the file's fingerprint still matches, and a successful
	// build replaces it.
	Failure *FailureRecord `json:"failure,omitempty"`
}

// FailureRecord is a build that failed partway through a download.
type FailureRecord struct {
	FingerprintHash string `json:"fingerprint_hash"`
	// Version is the Shisho release the build failed in. A record from
	// another release does not apply.
	Version  string    `json:"version"`
	Message  string    `json:"message"`
	FailedAt time.Time `json:"failed_at"`
}

// metadataFilename returns the metadata file path for a file delivered as
// fileType.
func metadataFilename(cacheDir string, fileID int, fileType string) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%d-%s.meta.json", fileID, fileType))
}

// cachedFilename returns the cached file path for a file delivered as
// fileType. Downloads are named separately, so it carries no extension.
func cachedFilename(cacheDir string, fileID int, fileType string) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%d-%s", fileID, fileType))
}

// ReadMetadata reads the cache metadata for a file delivered as fileType.
// Returns nil if the metadata file doesn't exist.
func ReadMetadata(cacheDir string, fileID int, fileType string) (*CacheMetadata, error) {
	path := metadataFilename(cacheDir, fileID, fileType)

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

// WriteMetadata writes the cache metadata for the entry meta describes.
func WriteMetadata(cacheDir string, meta *CacheMetadata) error {
	path := metadataFilename(cacheDir, meta.FileID, meta.DeliveredFileType)

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal cache metadata")
	}

	if err := fileutils.WriteFileAtomic(path, data, 0600); err != nil {
		return errors.Wrapf(err, "failed to write cache metadata: %s", path)
	}

	return nil
}

// UpdateLastAccessed updates the last accessed time for a cached file.
func UpdateLastAccessed(cacheDir string, fileID int, fileType string) error {
	meta, err := ReadMetadata(cacheDir, fileID, fileType)
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
func DeleteCachedFile(cacheDir string, fileID int, fileType string) error {
	return removeAll(cachedFilename(cacheDir, fileID, fileType), metadataFilename(cacheDir, fileID, fileType))
}

// removeAll removes each path, ignoring ones that do not exist.
func removeAll(paths ...string) error {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.Wrapf(err, "failed to delete cache file: %s", path)
		}
	}
	return nil
}

// GetCachedFilePath returns the path to a cached file if it exists and is valid.
// Returns empty string if the cache is invalid or doesn't exist.
func GetCachedFilePath(cacheDir string, fileID int, fileType string, currentHash string) (string, error) {
	meta, err := ReadMetadata(cacheDir, fileID, fileType)
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
	cachedPath := cachedFilename(cacheDir, fileID, fileType)
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
		if meta.FingerprintHash == "" && meta.Failure != nil {
			continue // Only a remembered failure; no file is cached
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
