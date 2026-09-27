package downloadcache

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
)

// Cache manages the download cache for generated files.
type Cache struct {
	dir               string
	maxSize           int64
	ShouldSkipCleanup func() bool

	// cleanups tracks background cleanups so Wait can drain them.
	cleanups sync.WaitGroup

	// generating serializes generation per destination path. Requests for the
	// same output wait and then reuse the file the first one published.
	generating keyedLock
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

	downloadFilename = FormatDownloadFilename(book, file)

	cachedPath, err = c.getOrGenerate(ctx, cachedFilename(c.dir, file.ID, file.FileType), cacheOps{
		lookup: func() (string, error) { return GetCachedFilePath(c.dir, file.ID, file.FileType, hash) },
		touch:  func() { _ = UpdateLastAccessed(c.dir, file.ID) },
		generate: func(stagedPath string) error {
			generator, err := filegen.GetGenerator(file.FileType)
			if err != nil {
				return errors.Wrap(err, "failed to get generator")
			}
			return errors.Wrap(generator.Generate(ctx, file.Filepath, stagedPath, book, file), "failed to generate file")
		},
		writeMeta: func(meta *CacheMetadata) error {
			return errors.Wrap(WriteMetadata(c.dir, meta), "failed to write cache metadata")
		},
		meta: CacheMetadata{FileID: file.ID, FingerprintHash: hash},
	})
	if err != nil {
		return "", "", err
	}
	return cachedPath, downloadFilename, nil
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

	downloadFilename = FormatKepubDownloadFilename(book, file)

	cachedPath, err = c.getOrGenerate(ctx, kepubCachedFilename(c.dir, file.ID), cacheOps{
		lookup: func() (string, error) { return GetKepubCachedFilePath(c.dir, file.ID, hash) },
		touch:  func() { _ = UpdateKepubLastAccessed(c.dir, file.ID) },
		generate: func(stagedPath string) error {
			generator, err := filegen.GetKepubGenerator(file.FileType)
			if err != nil {
				return errors.Wrap(err, "failed to get kepub generator")
			}
			return errors.Wrap(generator.Generate(ctx, file.Filepath, stagedPath, book, file), "failed to generate kepub file")
		},
		writeMeta: func(meta *CacheMetadata) error {
			return errors.Wrap(WriteKepubMetadata(c.dir, meta), "failed to write kepub cache metadata")
		},
		meta: CacheMetadata{FileID: file.ID, Format: FormatKepub, FingerprintHash: hash},
	})
	if err != nil {
		return "", "", err
	}
	return cachedPath, downloadFilename, nil
}

// cacheOps describes one kind of cache entry for getOrGenerate.
type cacheOps struct {
	// lookup returns the published path if it matches the current
	// fingerprint, or "" on a miss.
	lookup func() (string, error)
	// touch records a cache hit. Failures are ignored.
	touch func()
	// generate writes the output to stagedPath.
	generate func(stagedPath string) error
	// writeMeta publishes metadata for a freshly generated file.
	writeMeta func(meta *CacheMetadata) error
	// meta carries the identifying fields; timestamps and size are filled in.
	meta CacheMetadata
}

// getOrGenerate returns destPath when lookup reports a hit, and otherwise
// generates it. Generation for a given destPath is serialized, and every
// waiter checks the cache again once it holds the lock, so concurrent cold
// reads of the same file generate it once. Different fingerprints of one file
// share a destPath, so they are serialized too.
func (c *Cache) getOrGenerate(ctx context.Context, destPath string, ops cacheOps) (string, error) {
	if existing, err := ops.lookup(); err != nil {
		return "", errors.Wrap(err, "failed to check cache")
	} else if existing != "" {
		ops.touch()
		return existing, nil
	}

	release, err := c.generating.acquire(ctx, destPath)
	if err != nil {
		return "", errors.Wrap(err, "waiting for cache generation")
	}
	defer release()

	if existing, err := ops.lookup(); err != nil {
		return "", errors.Wrap(err, "failed to check cache")
	} else if existing != "" {
		ops.touch()
		return existing, nil
	}

	size, err := generateInto(c.dir, destPath, ops.generate)
	if err != nil {
		return "", err
	}

	now := time.Now()
	meta := ops.meta
	meta.GeneratedAt = now
	meta.LastAccessedAt = now
	meta.SizeBytes = size
	if err := ops.writeMeta(&meta); err != nil {
		// Without metadata the file is unreachable, so don't leave it behind.
		os.Remove(destPath)
		return "", err
	}

	c.triggerCleanupAsync()

	return destPath, nil
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

	downloadFilename = FormatPluginDownloadFilename(book, file, formatID)

	cachedPath, err = c.getOrGenerate(ctx, pluginCachedFilename(c.dir, file.ID, formatID), cacheOps{
		lookup: func() (string, error) { return GetPluginCachedFilePath(c.dir, file.ID, formatID, hash) },
		touch:  func() { _ = UpdatePluginLastAccessed(c.dir, file.ID, formatID) },
		generate: func(stagedPath string) error {
			return errors.Wrap(generator.Generate(ctx, file.Filepath, stagedPath, book, file), "failed to generate plugin file")
		},
		writeMeta: func(meta *CacheMetadata) error {
			return errors.Wrap(WritePluginMetadata(c.dir, file.ID, formatID, meta), "failed to write plugin cache metadata")
		},
		meta: CacheMetadata{FileID: file.ID, Format: "plugin:" + formatID, FingerprintHash: hash},
	})
	if err != nil {
		return "", "", err
	}
	return cachedPath, downloadFilename, nil
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

// BulkZipDir returns the directory path for bulk zip files.
func (c *Cache) BulkZipDir() string {
	return filepath.Join(c.dir, "bulk")
}

// BulkZipPath returns the full path for a bulk zip file identified by its fingerprint hash.
func (c *Cache) BulkZipPath(fingerprintHash string) string {
	return filepath.Join(c.dir, "bulk", fingerprintHash+".zip")
}

// BulkZipExists returns true if a bulk zip file with the given fingerprint hash exists.
func (c *Cache) BulkZipExists(fingerprintHash string) bool {
	_, err := os.Stat(c.BulkZipPath(fingerprintHash))
	return err == nil
}

// TriggerCleanup runs cache cleanup if the cache exceeds the max size.
// This runs in the current goroutine - call with `go` to run in background.
func (c *Cache) TriggerCleanup() {
	// Cleanup errors are non-fatal - best effort only
	_ = c.runCleanup()
}

// runCleanup performs the actual cleanup operation.
func (c *Cache) runCleanup() error {
	if c.ShouldSkipCleanup != nil && c.ShouldSkipCleanup() {
		return nil
	}

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

// triggerCleanupAsync runs a best-effort cleanup in the background so a
// download never waits on cache eviction.
func (c *Cache) triggerCleanupAsync() {
	c.cleanups.Add(1)
	go func() {
		defer c.cleanups.Done()
		c.TriggerCleanup()
	}()
}

// Wait blocks until every background cleanup started so far has finished. A
// cleanup writes a lock file into the cache directory, so anything that
// removes the directory (tests using t.TempDir, mostly) must Wait first or the
// removal can fail with "directory not empty".
func (c *Cache) Wait() {
	c.cleanups.Wait()
}

// Dir returns the cache directory path.
func (c *Cache) Dir() string {
	return c.dir
}

// MaxSize returns the maximum cache size in bytes.
func (c *Cache) MaxSize() int64 {
	return c.maxSize
}

// SizeBytes returns the total size in bytes and file count of the cache.
// Includes individual cached files (tracked via metadata) and bulk zip files.
// A missing root directory is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error) {
	var totalBytes int64
	var totalCount int

	entries, err := ListCacheEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list cache entries")
	}
	for _, e := range entries {
		totalBytes += e.SizeBytes
		totalCount++
	}

	bulkEntries, bulkBytes, err := listBulkZipEntries(c.dir)
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to list bulk entries")
	}
	totalBytes += bulkBytes
	totalCount += len(bulkEntries)

	return totalBytes, totalCount, nil
}

// Clear removes all cached content while preserving the root directory itself.
// Safe to call when the root does not exist.
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "failed to read cache directory")
	}
	for _, e := range entries {
		path := filepath.Join(c.dir, e.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}
