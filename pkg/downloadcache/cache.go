package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
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

// GetOrGenerateKepub returns the path to a cached KePub file, generating it if necessary.
// It returns the cached file path, the formatted download filename, and any error.
// Returns ErrKepubNotSupported if the file type cannot be converted to KePub.
func (c *Cache) GetOrGenerateKepub(ctx context.Context, book *models.Book, file *models.File) (cachedPath string, downloadFilename string, err error) {
	// Check if this file type supports KePub conversion
	if !filegen.SupportsKepub(file.FileType) {
		return "", "", filegen.ErrKepubNotSupported
	}

	// Compute the fingerprint for the current state with KePub format
	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compute fingerprint")
	}
	fp.Format = FormatKepub

	hash, err := fp.Hash()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to hash fingerprint")
	}

	// Check if we have a valid cached file
	existingPath, err := GetKepubCachedFilePath(c.dir, file.ID, hash)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to check kepub cache")
	}

	downloadFilename = FormatKepubDownloadFilename(book, file)

	if existingPath != "" {
		// Update last accessed time (non-fatal if it fails)
		_ = UpdateKepubLastAccessed(c.dir, file.ID)
		return existingPath, downloadFilename, nil
	}

	// Need to generate a new file
	destPath := kepubCachedFilename(c.dir, file.ID)

	// Get the appropriate KePub generator
	generator, err := filegen.GetKepubGenerator(file.FileType)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get kepub generator")
	}

	// Generate the file
	if err := generator.Generate(ctx, file.Filepath, destPath, book, file); err != nil {
		return "", "", errors.Wrap(err, "failed to generate kepub file")
	}

	// Get the size of the generated file
	info, err := os.Stat(destPath)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to stat generated kepub file")
	}

	// Write the metadata
	now := time.Now()
	meta := &CacheMetadata{
		FileID:          file.ID,
		Format:          FormatKepub,
		FingerprintHash: hash,
		GeneratedAt:     now,
		LastAccessedAt:  now,
		SizeBytes:       info.Size(),
	}
	if err := WriteKepubMetadata(c.dir, meta); err != nil {
		// Clean up the generated file if we can't write metadata
		os.Remove(destPath)
		return "", "", errors.Wrap(err, "failed to write kepub cache metadata")
	}

	// Trigger cleanup in background
	go c.TriggerCleanup()

	return destPath, downloadFilename, nil
}

// Invalidate removes the cached file for a given file ID.
func (c *Cache) Invalidate(fileID int, fileType string) error {
	return DeleteCachedFile(c.dir, fileID, fileType)
}

// InvalidateKepub removes the cached KePub file for a given file ID.
func (c *Cache) InvalidateKepub(fileID int) error {
	return DeleteKepubCachedFile(c.dir, fileID)
}

// GetOrGeneratePlugin returns the path to a cached plugin-generated file.
// The pluginGenerator handles both generation and fingerprinting.
func (c *Cache) GetOrGeneratePlugin(ctx context.Context, book *models.Book, file *models.File, generator *plugins.PluginGenerator) (cachedPath string, downloadFilename string, err error) {
	formatID := generator.SupportedType()

	// Get the plugin's fingerprint for cache invalidation
	pluginFP, err := generator.Fingerprint(book, file)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compute plugin fingerprint")
	}

	// Compute a hash that includes both the standard fingerprint and the plugin fingerprint
	fp, err := ComputeFingerprint(book, file)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to compute fingerprint")
	}
	fp.Format = "plugin:" + formatID
	fp.PluginFingerprint = pluginFP

	hash, err := fp.Hash()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to hash fingerprint")
	}

	// Check if we have a valid cached file
	existingPath, err := GetPluginCachedFilePath(c.dir, file.ID, formatID, hash)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to check plugin cache")
	}

	downloadFilename = FormatPluginDownloadFilename(book, file, formatID)

	if existingPath != "" {
		// Update last accessed time (non-fatal if it fails)
		_ = UpdatePluginLastAccessed(c.dir, file.ID, formatID)
		return existingPath, downloadFilename, nil
	}

	// Need to generate a new file
	destPath := pluginCachedFilename(c.dir, file.ID, formatID)

	// Generate the file
	if err := generator.Generate(ctx, file.Filepath, destPath, book, file); err != nil {
		return "", "", errors.Wrap(err, "failed to generate plugin file")
	}

	// Get the size of the generated file
	info, err := os.Stat(destPath)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to stat generated plugin file")
	}

	// Write the metadata
	now := time.Now()
	meta := &CacheMetadata{
		FileID:          file.ID,
		Format:          "plugin:" + formatID,
		FingerprintHash: hash,
		GeneratedAt:     now,
		LastAccessedAt:  now,
		SizeBytes:       info.Size(),
	}
	if err := WritePluginMetadata(c.dir, file.ID, formatID, meta); err != nil {
		// Clean up the generated file if we can't write metadata
		os.Remove(destPath)
		return "", "", errors.Wrap(err, "failed to write plugin cache metadata")
	}

	// Trigger cleanup in background
	go c.TriggerCleanup()

	return destPath, downloadFilename, nil
}

// InvalidatePlugin removes the cached plugin file for a given file ID and format.
func (c *Cache) InvalidatePlugin(fileID int, formatID string) error {
	return DeletePluginCachedFile(c.dir, fileID, formatID)
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
