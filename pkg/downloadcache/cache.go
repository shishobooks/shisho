package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
)

// Cache manages the download cache for generated files.
type Cache struct {
	dir     string
	maxSize int64
}

// NewCache creates a new Cache with the given directory and max size.
func NewCache(dir string, maxSizeBytes int64) *Cache {
	return &Cache{
		dir:     dir,
		maxSize: maxSizeBytes,
	}
}

// GetOrGenerate returns the path to a cached file, generating it if necessary.
// It returns the cached file path, the formatted download filename, and any error.
func (c *Cache) GetOrGenerate(ctx context.Context, book *models.Book, file *models.File) (cachedPath string, downloadFilename string, err error) {
	// Compute the fingerprint for the current state
	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compute fingerprint")
	}

	hash, err := fp.Hash()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to hash fingerprint")
	}

	// Check if we have a valid cached file
	existingPath, err := GetCachedFilePath(c.dir, file.ID, file.FileType, hash)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to check cache")
	}

	downloadFilename = FormatDownloadFilename(book, file)

	if existingPath != "" {
		// Update last accessed time (non-fatal if it fails)
		_ = UpdateLastAccessed(c.dir, file.ID)
		return existingPath, downloadFilename, nil
	}

	// Need to generate a new file
	destPath := cachedFilename(c.dir, file.ID, file.FileType)

	// Get the appropriate generator
	generator, err := filegen.GetGenerator(file.FileType)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get generator")
	}

	// Generate the file
	if err := generator.Generate(ctx, file.Filepath, destPath, book, file); err != nil {
		return "", "", errors.Wrap(err, "failed to generate file")
	}

	// Get the size of the generated file
	info, err := os.Stat(destPath)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to stat generated file")
	}

	// Write the metadata
	now := time.Now()
	meta := &CacheMetadata{
		FileID:          file.ID,
		FingerprintHash: hash,
		GeneratedAt:     now,
		LastAccessedAt:  now,
		SizeBytes:       info.Size(),
	}
	if err := WriteMetadata(c.dir, meta); err != nil {
		// Clean up the generated file if we can't write metadata
		os.Remove(destPath)
		return "", "", errors.Wrap(err, "failed to write cache metadata")
	}

	// Trigger cleanup in background
	go c.TriggerCleanup()

	return destPath, downloadFilename, nil
}

// Invalidate removes the cached file for a given file ID.
func (c *Cache) Invalidate(fileID int, fileType string) error {
	return DeleteCachedFile(c.dir, fileID, fileType)
}

// GetCachedPath returns the path to a cached file if it exists and is valid.
// Returns empty string if the cache doesn't exist or is invalid.
func (c *Cache) GetCachedPath(fileID int, fileType string, book *models.Book, file *models.File) (string, error) {
	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return "", errors.Wrap(err, "failed to compute fingerprint")
	}

	hash, err := fp.Hash()
	if err != nil {
		return "", errors.Wrap(err, "failed to hash fingerprint")
	}

	return GetCachedFilePath(c.dir, fileID, fileType, hash)
}

// TriggerCleanup runs cache cleanup if the cache exceeds the max size.
// This runs in the current goroutine - call with `go` to run in background.
func (c *Cache) TriggerCleanup() {
	// Cleanup errors are non-fatal - best effort only
	_ = c.runCleanup()
}

// runCleanup performs the actual cleanup operation.
func (c *Cache) runCleanup() error {
	// Get lock file to prevent concurrent cleanups
	lockPath := filepath.Join(c.dir, ".cleanup.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// Another cleanup is running or we can't create the lock
		return nil
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath)
	}()

	return RunCleanup(c.dir, c.maxSize)
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}

// MaxSize returns the maximum cache size in bytes.
func (c *Cache) MaxSize() int64 {
	return c.maxSize
}
