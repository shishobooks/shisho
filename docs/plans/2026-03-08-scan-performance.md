# Scan Performance Optimization Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Optimize filesystem scanning to skip unchanged files and eliminate redundant DB queries and I/O during rescans.

**Architecture:** Add `FileModifiedAt` column to track file modification times, pre-load all known file paths before scanning to replace per-file DB lookups with map lookups, and skip MIME detection for files already in DB. These three changes together should reduce rescan time by 90%+ for libraries with no changes.

**Tech Stack:** Go, SQLite (Bun ORM), `filepath.WalkDir`

---

### Task 1: Add `FileModifiedAt` column to File model

**Files:**
- Modify: `pkg/models/file.go`
- Create: `pkg/migrations/20260308000000_add_file_modified_at.go`

**Step 1: Create the migration**

Create `pkg/migrations/20260308000000_add_file_modified_at.go`:

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN file_modified_at DATETIME`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files DROP COLUMN file_modified_at`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Add field to File model**

Add to `pkg/models/file.go` in the `File` struct, after `FilesizeBytes`:

```go
FileModifiedAt   *time.Time        `json:"file_modified_at"`
```

**Step 3: Run migration and generate types**

Run: `make db:migrate && make tygo`

**Step 4: Commit**

```
[Backend] Add FileModifiedAt column to track file modification times
```

---

### Task 2: Store FileModifiedAt when creating files

**Files:**
- Modify: `pkg/worker/scan_unified.go` (scanFileCreateNew function)

**Step 1: Store mod time when creating new files**

In `scanFileCreateNew` (~line 1907-1911), `stats` is already available from `os.Stat()`. After `size := stats.Size()`, capture the mod time and store it on the file struct (around line 2092-2102):

```go
modTime := stats.ModTime()
```

Add to the `file := &models.File{...}` struct literal:

```go
FileModifiedAt: &modTime,
```

**Step 2: Run tests**

Run: `make test`

**Step 3: Commit**

```
[Backend] Store file modification time when creating files during scan
```

---

### Task 3: Pre-load file paths and add change detection to skip unchanged files

This is the core optimization. Instead of per-file DB queries and re-parsing unchanged files, we:
1. Pre-load all known file paths for the library into a map
2. During the parallel scan, check if the file is unchanged (same size + mod time)
3. Skip re-parsing unchanged files entirely

**Files:**
- Modify: `pkg/worker/scan.go` (ProcessScanJob)
- Modify: `pkg/worker/scan_unified.go` (scanFileByPath)
- Modify: `pkg/worker/scan_cache.go` (ScanCache)
- Modify: `pkg/books/service.go` (ListFilesForLibrary - add mod time to query)

**Step 1: Update ListFilesForLibrary to return FileModifiedAt**

In `pkg/books/service.go`, `ListFilesForLibrary` currently selects all columns implicitly. Verify it returns `FileModifiedAt` (it should, since Bun selects all model columns by default). No code change needed if using `.Model(&files)`.

**Step 2: Add file lookup map to ScanCache**

In `pkg/worker/scan_cache.go`, add a field to `ScanCache`:

```go
// Pre-loaded file lookup for the current library
knownFiles sync.Map // map[string]*models.File (keyed by filepath)
```

Add methods:

```go
// LoadKnownFiles populates the known files cache from a list of existing files.
func (c *ScanCache) LoadKnownFiles(files []*models.File) {
	for _, f := range files {
		c.knownFiles.Store(f.Filepath, f)
	}
}

// GetKnownFile returns a known file by path, or nil if not found.
func (c *ScanCache) GetKnownFile(path string) *models.File {
	if v, ok := c.knownFiles.Load(path); ok {
		return v.(*models.File)
	}
	return nil
}

// KnownFilePaths returns all known file paths as a set for orphan detection.
// This avoids needing a separate ListFilesForLibrary call after scanning.
func (c *ScanCache) KnownFiles() []*models.File {
	var files []*models.File
	c.knownFiles.Range(func(_, value any) bool {
		files = append(files, value.(*models.File))
		return true
	})
	return files
}
```

**Step 3: Pre-load files in ProcessScanJob and use for orphan detection**

In `pkg/worker/scan.go` `ProcessScanJob`, move `ListFilesForLibrary` call BEFORE the parallel scan loop (currently it's at line 405, after scanning). Load into cache:

```go
cache := NewScanCache()

// Pre-load all known files for fast lookup during scan
existingFiles, err := w.bookService.ListFilesForLibrary(ctx, library.ID)
if err != nil {
	jobLog.Warn("failed to pre-load files", logger.Data{"error": err.Error()})
} else {
	cache.LoadKnownFiles(existingFiles)
	jobLog.Info("pre-loaded known files", logger.Data{"count": len(existingFiles)})
}
```

Then update the orphan cleanup section (lines 403-424) to use the already-loaded files instead of calling `ListFilesForLibrary` again:

```go
// Cleanup orphaned files (in DB but not on disk)
// Build a set of all file paths we scanned
scannedPaths := make(map[string]struct{}, len(filesToScan))
for _, path := range filesToScan {
	scannedPaths[path] = struct{}{}
}

for _, file := range existingFiles {
	if _, seen := scannedPaths[file.Filepath]; !seen {
		jobLog.Info("cleaning up orphaned file", logger.Data{"file_id": file.ID, "filepath": file.Filepath})
		_, err := w.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
		if err != nil {
			jobLog.Warn("failed to cleanup orphaned file", logger.Data{"file_id": file.ID, "error": err.Error()})
		}
	}
}
```

**Step 4: Update scanFileByPath to use cache and skip unchanged files**

Replace the per-file DB query in `scanFileByPath` with a cache lookup and add change detection:

```go
func (w *Worker) scanFileByPath(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
	if opts.LibraryID == 0 {
		return nil, errors.New("LibraryID required for FilePath mode")
	}

	// Fast path: check cache for known file (avoids per-file DB query)
	if cache != nil {
		if existingFile := cache.GetKnownFile(opts.FilePath); existingFile != nil {
			// File exists in DB — check if it changed on disk
			if !opts.ForceRefresh {
				stat, err := os.Stat(opts.FilePath)
				if err == nil && existingFile.FileModifiedAt != nil {
					// Skip if size and mod time are unchanged
					if stat.Size() == existingFile.FilesizeBytes && stat.ModTime().Equal(*existingFile.FileModifiedAt) {
						return &ScanResult{
							File: existingFile,
						}, nil
					}
				}
			}
			// File changed or ForceRefresh — delegate to scanFileByID for full rescan
			return w.scanFileByID(ctx, ScanOptions{
				FileID:       existingFile.ID,
				ForceRefresh: opts.ForceRefresh,
				JobLog:       opts.JobLog,
			}, cache)
		}
	} else {
		// No cache — fall back to DB query (backward compatible for single-file rescans)
		existingFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
			Filepath:  &opts.FilePath,
			LibraryID: &opts.LibraryID,
		})
		if err != nil && !errors.Is(err, errcodes.NotFound("File")) {
			return nil, errors.Wrap(err, "failed to check if file exists")
		}
		if existingFile != nil {
			return w.scanFileByID(ctx, ScanOptions{
				FileID:       existingFile.ID,
				ForceRefresh: opts.ForceRefresh,
				JobLog:       opts.JobLog,
			}, cache)
		}
	}

	// File doesn't exist in DB - check if it exists on disk
	_, err := os.Stat(opts.FilePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat file")
	}

	return w.scanFileCreateNew(ctx, opts, cache)
}
```

**Step 5: Update scanFileByID to store FileModifiedAt after re-parsing**

In `scanFileByID`, after the metadata is processed and the file is updated, store the new mod time. After the `os.Stat` call at line 269, capture the stat result:

```go
stat, err := os.Stat(file.Filepath)
```

Then after `scanFileCore` returns successfully, update the file's mod time:

```go
if stat != nil {
	modTime := stat.ModTime()
	file.FileModifiedAt = &modTime
	file.FilesizeBytes = stat.Size()
	_ = w.bookService.UpdateFile(ctx, file, books.UpdateFileOptions{
		Columns: []string{"file_modified_at", "filesize_bytes"},
	})
}
```

**Step 6: Run tests**

Run: `make test`

**Step 7: Commit**

```
[Backend] Skip unchanged files during rescan using mod-time and size comparison
```

---

### Task 4: Skip MIME detection for files already in DB

**Files:**
- Modify: `pkg/worker/scan.go` (ProcessScanJob - WalkDir callback)

**Step 1: Move pre-load before WalkDir and skip MIME detection for known files**

The MIME detection in the WalkDir callback (`mimetype.DetectFile`) reads the file header for every built-in file found. For files already in the DB, this is unnecessary — we already validated the MIME when first importing.

In the WalkDir callback, after the extension check succeeds for built-in types, check the cache before doing MIME detection:

```go
expectedMimeTypes, ok := extensionsToScan[ext]
if !ok {
	// ... existing plugin extension checks ...
	return nil
}

// Skip MIME detection for files we already know about
if cache != nil && cache.GetKnownFile(path) != nil {
	filesToScan = append(filesToScan, path)
	return nil
}

// New file - validate MIME type
mtype, err := mimetype.DetectFile(path)
```

**Step 2: Run tests**

Run: `make test`

**Step 3: Commit**

```
[Backend] Skip MIME type detection for files already known to the scanner
```

---

### Task 5: Write tests for change detection

**Files:**
- Modify: `pkg/worker/scan_unified_test.go`

**Step 1: Write test for skipping unchanged files**

```go
func TestScanFileByPath_SkipsUnchangedFile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a temp file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "book.epub")
	testgen.CreateMinimalEPUB(t, filePath)

	// Get mod time
	stat, _ := os.Stat(filePath)
	modTime := stat.ModTime()

	// Create library and file in DB with matching mod time
	tc.createLibrary([]string{dir})
	book := &models.Book{LibraryID: 1, Filepath: dir, Title: "Test"}
	tc.bookService.CreateBook(tc.ctx, book)
	file := &models.File{
		LibraryID:      1,
		BookID:         book.ID,
		Filepath:       filePath,
		FileType:       "epub",
		FilesizeBytes:  stat.Size(),
		FileModifiedAt: &modTime,
	}
	tc.bookService.CreateFile(tc.ctx, file)

	// Load into cache
	cache := NewScanCache()
	cache.LoadKnownFiles([]*models.File{file})

	// Scan — should skip since file is unchanged
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  filePath,
		LibraryID: 1,
	}, cache)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, file.ID, result.File.ID)
}
```

**Step 2: Write test for rescanning changed files**

```go
func TestScanFileByPath_RescansChangedFile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a temp file
	dir := t.TempDir()
	filePath := filepath.Join(dir, "book.epub")
	testgen.CreateMinimalEPUB(t, filePath)

	// Create library and file in DB with OLD mod time
	tc.createLibrary([]string{dir})
	stat, _ := os.Stat(filePath)
	oldModTime := stat.ModTime().Add(-time.Hour)
	book := &models.Book{LibraryID: 1, Filepath: dir, Title: "Test"}
	tc.bookService.CreateBook(tc.ctx, book)
	file := &models.File{
		LibraryID:      1,
		BookID:         book.ID,
		Filepath:       filePath,
		FileType:       "epub",
		FilesizeBytes:  stat.Size(),
		FileModifiedAt: &oldModTime,
	}
	tc.bookService.CreateFile(tc.ctx, file)

	// Load into cache
	cache := NewScanCache()
	cache.LoadKnownFiles([]*models.File{file})

	// Scan — should NOT skip since mod time differs
	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{
		FilePath:  filePath,
		LibraryID: 1,
	}, cache)

	require.NoError(t, err)
	// Result should reflect a full rescan (file was re-parsed)
	require.NotNil(t, result)
}
```

**Step 3: Run tests**

Run: `make test`

**Step 4: Commit**

```
[Test] Add tests for scan change detection (skip unchanged, rescan changed)
```

---

### Task 6: Final verification

**Step 1: Run full check suite**

Run: `make check`
Expected: All tests pass, linting clean.

**Step 2: Commit if any remaining changes**
