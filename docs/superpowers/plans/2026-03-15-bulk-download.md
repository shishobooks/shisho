# Bulk Download Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add bulk download to gallery select mode — users select books, download a zip of primary files with metadata injected, backed by the job system with SSE progress.

**Architecture:** New `bulk_download` job type processed by the worker, which generates metadata-injected files via the existing download cache, zips them in store mode, and caches the zip. Frontend adds a Download button to the selection toolbar, tracks progress via SSE in a React context, and shows a persistent toast with a download link on completion.

**Tech Stack:** Go (Echo, Bun, archive/zip), React (Tanstack Query, sonner toasts, React context), SSE

**Spec:** `docs/superpowers/specs/2026-03-15-bulk-download-design.md`

---

## Chunk 1: Backend — Model, Events, Cache, and Worker

### Task 1: Add bulk_download job type and data struct

**Files:**
- Modify: `pkg/models/job.go`

- [ ] **Step 1: Add `JobTypeBulkDownload` constant and data struct**

In `pkg/models/job.go`, add `JobTypeBulkDownload` to the constants block and create the data struct:

```go
// Update the tygo:emit comment to include the new type:
//tygo:emit export type JobType = typeof JobTypeExport | typeof JobTypeScan | typeof JobTypeBulkDownload;

// Add the new constant:
JobTypeBulkDownload = "bulk_download"

// Add the data struct (after JobScanData):
type JobBulkDownloadData struct {
	// Input (set on creation)
	FileIDs            []int `json:"file_ids"`
	EstimatedSizeBytes int64 `json:"estimated_size_bytes"`

	// Result (set on completion)
	ZipFilename     string `json:"zip_filename,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	FileCount       int    `json:"file_count,omitempty"`
	FingerprintHash string `json:"fingerprint_hash,omitempty"`
}
```

- [ ] **Step 2: Update `UnmarshalData()` to handle `bulk_download`**

Add a case in the switch statement:

```go
case JobTypeBulkDownload:
	job.DataParsed = &JobBulkDownloadData{}
```

- [ ] **Step 3: Update `DataParsed` tstype annotation**

Change the `tstype` tag on `DataParsed` from:
```
tstype:"JobExportData | JobScanData"
```
to:
```
tstype:"JobExportData | JobScanData | JobBulkDownloadData"
```

- [ ] **Step 4: Run `make tygo` and verify**

Run: `make tygo`

Check that `app/types/generated/models.ts` now includes `JobBulkDownloadData` and the updated `JobType` union.

- [ ] **Step 5: Commit**

```bash
git add pkg/models/job.go
git commit -m "[Backend] Add bulk_download job type and data model"
```

### Task 2: Update job validators

**Files:**
- Modify: `pkg/jobs/validators.go`

- [ ] **Step 1: Add `bulk_download` to validation rules**

In `CreateJobPayload.Type`, change:
```
validate:"required,oneof=export scan"
```
to:
```
validate:"required,oneof=export scan bulk_download"
```

Also update the `tstype` on `CreateJobPayload.Data`:
```
tstype:"JobExportData | JobScanData | JobBulkDownloadData"
```

In `ListJobsQuery.Type`, change:
```
validate:"omitempty,oneof=export scan"
```
to:
```
validate:"omitempty,oneof=export scan bulk_download"
```

- [ ] **Step 2: Run `make tygo` to regenerate types**

Run: `make tygo`

- [ ] **Step 3: Commit**

```bash
git add pkg/jobs/validators.go
git commit -m "[Backend] Add bulk_download to job validation rules"
```

### Task 3: Add bulk download progress event helper

**Files:**
- Modify: `pkg/events/broker.go`

- [ ] **Step 1: Add `NewBulkDownloadProgressEvent` function**

```go
// NewBulkDownloadProgressEvent builds a progress event for bulk download jobs.
func NewBulkDownloadProgressEvent(jobID int, status string, current, total int, estimatedSizeBytes int64) Event {
	data := fmt.Sprintf(
		`{"job_id":%d,"status":"%s","current":%d,"total":%d,"estimated_size_bytes":%d}`,
		jobID, status, current, total, estimatedSizeBytes,
	)
	return Event{Type: "bulk_download.progress", Data: data}
}
```

- [ ] **Step 2: Commit**

```bash
git add pkg/events/broker.go
git commit -m "[Backend] Add SSE event helper for bulk download progress"
```

### Task 4: Add bulk zip cache methods to download cache

**Files:**
- Modify: `pkg/downloadcache/cache.go`
- Modify: `pkg/downloadcache/cleanup.go`

- [ ] **Step 1: Add `BulkZipPath` and `BulkZipExists` methods to `Cache`**

In `cache.go`, add:

```go
// BulkZipDir returns the path to the bulk zip cache subdirectory.
func (c *Cache) BulkZipDir() string {
	return filepath.Join(c.dir, "bulk")
}

// BulkZipPath returns the expected path for a bulk zip with the given fingerprint hash.
func (c *Cache) BulkZipPath(fingerprintHash string) string {
	return filepath.Join(c.dir, "bulk", fingerprintHash+".zip")
}

// BulkZipExists checks if a bulk zip with the given fingerprint exists.
func (c *Cache) BulkZipExists(fingerprintHash string) bool {
	_, err := os.Stat(c.BulkZipPath(fingerprintHash))
	return err == nil
}
```

- [ ] **Step 2: Add `ShouldSkipCleanup` callback field**

In the `Cache` struct, add a callback field:

```go
type Cache struct {
	dir              string
	maxSize          int64
	ShouldSkipCleanup func() bool // If set and returns true, cleanup is skipped
}
```

Update `runCleanup()` to check it:

```go
func (c *Cache) runCleanup() error {
	if c.ShouldSkipCleanup != nil && c.ShouldSkipCleanup() {
		return nil
	}
	// ... rest of existing code
```

- [ ] **Step 3: Add `pdf` to `findCachedFileExtension`**

In `cleanup.go`, update the extensions slice:

```go
extensions := []string{"epub", "m4b", "cbz", "pdf"}
```

- [ ] **Step 4: Commit**

```bash
git add pkg/downloadcache/cache.go pkg/downloadcache/cleanup.go
git commit -m "[Backend] Add bulk zip cache methods and cleanup skip callback"
```

### Task 5: Write the bulk download test

**Files:**
- Create: `pkg/worker/bulk_download_test.go`

- [ ] **Step 1: Write a test for `ComputeBulkFingerprint`**

```go
package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBulkFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("deterministic for same inputs", func(t *testing.T) {
		t.Parallel()
		hashes := []string{"abc123", "def456", "ghi789"}
		hash1 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		hash2 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("sorts file IDs for consistency", func(t *testing.T) {
		t.Parallel()
		hashes1 := []string{"abc123", "def456"}
		hashes2 := []string{"def456", "abc123"}
		hash1 := ComputeBulkFingerprint([]int{1, 2}, hashes1)
		hash2 := ComputeBulkFingerprint([]int{2, 1}, hashes2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different hashes produce different fingerprint", func(t *testing.T) {
		t.Parallel()
		hash1 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "def"})
		hash2 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "xyz"})
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestDeduplicateFilenames(t *testing.T) {
	t.Parallel()

	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book A.epub", 2: "Book B.epub"}
		result := DeduplicateFilenames(names)
		assert.Equal(t, "Book A.epub", result[1])
		assert.Equal(t, "Book B.epub", result[2])
	})

	t.Run("duplicates get numbered", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book.epub", 2: "Book.epub", 3: "Book.epub"}
		result := DeduplicateFilenames(names)
		// One should keep original, others get (2), (3)
		values := []string{result[1], result[2], result[3]}
		require.Contains(t, values, "Book.epub")
		require.Contains(t, values, "Book (2).epub")
		require.Contains(t, values, "Book (3).epub")
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && go test ./pkg/worker/ -run "TestComputeBulkFingerprint|TestDeduplicateFilenames" -v`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Commit**

```bash
git add pkg/worker/bulk_download_test.go
git commit -m "[Test] Add tests for bulk download fingerprint and filename deduplication"
```

### Task 6: Implement bulk download worker

**Files:**
- Create: `pkg/worker/bulk_download.go`
- Modify: `pkg/worker/worker.go`

- [ ] **Step 1: Create `pkg/worker/bulk_download.go` with helper functions**

```go
package worker

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
)

// ComputeBulkFingerprint computes a composite fingerprint hash from sorted file IDs and their individual hashes.
func ComputeBulkFingerprint(fileIDs []int, fileHashes []string) string {
	// Create paired entries for sorting
	type entry struct {
		id   int
		hash string
	}
	entries := make([]entry, len(fileIDs))
	for i := range fileIDs {
		entries[i] = entry{id: fileIDs[i], hash: fileHashes[i]}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].id < entries[j].id
	})

	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%d:%s\n", e.id, e.hash)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// DeduplicateFilenames takes a map of fileID → filename and appends (2), (3), etc. for duplicates.
func DeduplicateFilenames(names map[int]string) map[int]string {
	// Count occurrences of each name
	counts := make(map[string][]int) // name → list of file IDs
	for id, name := range names {
		counts[name] = append(counts[name], id)
	}

	result := make(map[int]string, len(names))
	for name, ids := range counts {
		if len(ids) == 1 {
			result[ids[0]] = name
			continue
		}
		sort.Ints(ids)
		for i, id := range ids {
			if i == 0 {
				result[id] = name
			} else {
				ext := filepath.Ext(name)
				base := strings.TrimSuffix(name, ext)
				result[id] = fmt.Sprintf("%s (%d)%s", base, i+1, ext)
			}
		}
	}
	return result
}

// ProcessBulkDownloadJob generates metadata-injected files and creates a zip archive.
func (w *Worker) ProcessBulkDownloadJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	log := logger.FromContext(ctx)

	// Parse job data
	var data models.JobBulkDownloadData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.Wrap(err, "failed to parse bulk download job data")
	}

	if len(data.FileIDs) == 0 {
		return errors.New("no file IDs provided")
	}

	jobLog.Info(fmt.Sprintf("starting bulk download for %d files", len(data.FileIDs)), nil)

	// Load all files with their book relations
	type fileWithBook struct {
		file *models.File
		book *models.Book
	}
	filesWithBooks := make([]fileWithBook, 0, len(data.FileIDs))

	for _, fileID := range data.FileIDs {
		file, err := w.bookService.RetrieveFileWithRelations(ctx, fileID)
		if err != nil {
			jobLog.Warn(fmt.Sprintf("skipping file %d: %v", fileID, err), nil)
			continue
		}
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			jobLog.Warn(fmt.Sprintf("skipping file %d: failed to load book: %v", fileID, err), nil)
			continue
		}
		filesWithBooks = append(filesWithBooks, fileWithBook{file: file, book: book})
	}

	if len(filesWithBooks) == 0 {
		return errors.New("no valid files found for bulk download")
	}

	// Compute composite fingerprint
	fileIDs := make([]int, len(filesWithBooks))
	fileHashes := make([]string, len(filesWithBooks))
	for i, fw := range filesWithBooks {
		fileIDs[i] = fw.file.ID
		fp, err := downloadcache.ComputeFingerprint(fw.book, fw.file)
		if err != nil {
			return errors.Wrapf(err, "failed to compute fingerprint for file %d", fw.file.ID)
		}
		hash, err := fp.Hash()
		if err != nil {
			return errors.Wrapf(err, "failed to hash fingerprint for file %d", fw.file.ID)
		}
		fileHashes[i] = hash
	}
	compositeHash := ComputeBulkFingerprint(fileIDs, fileHashes)

	// Check if zip already exists in cache
	if w.downloadCache.BulkZipExists(compositeHash) {
		zipPath := w.downloadCache.BulkZipPath(compositeHash)
		info, err := os.Stat(zipPath)
		if err == nil {
			jobLog.Info("bulk zip already cached, skipping generation", nil)
			data.ZipFilename = filepath.Base(zipPath)
			data.SizeBytes = info.Size()
			data.FileCount = len(filesWithBooks)
			data.FingerprintHash = compositeHash
			return w.completeBulkDownloadJob(ctx, job, &data)
		}
	}

	// Generate each file via download cache
	total := len(filesWithBooks)
	cachedPaths := make(map[int]string, total)
	downloadNames := make(map[int]string, total)

	for i, fw := range filesWithBooks {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "job cancelled")
		}

		cachedPath, downloadFilename, err := w.downloadCache.GetOrGenerate(ctx, fw.book, fw.file)
		if err != nil {
			jobLog.Warn(fmt.Sprintf("failed to generate file %d (%s): %v", fw.file.ID, fw.file.Filepath, err), nil)
			continue
		}
		cachedPaths[fw.file.ID] = cachedPath
		downloadNames[fw.file.ID] = downloadFilename

		// Publish progress event
		if w.broker != nil {
			w.broker.Publish(events.NewBulkDownloadProgressEvent(
				job.ID, "generating", i+1, total, data.EstimatedSizeBytes,
			))
		}

		log.Debug("generated file for bulk download", logger.Data{
			"file_id": fw.file.ID, "progress": fmt.Sprintf("%d/%d", i+1, total),
		})
	}

	if len(cachedPaths) == 0 {
		return errors.New("no files were successfully generated")
	}

	// Publish zipping status
	if w.broker != nil {
		w.broker.Publish(events.NewBulkDownloadProgressEvent(
			job.ID, "zipping", total, total, data.EstimatedSizeBytes,
		))
	}

	// Deduplicate filenames
	dedupedNames := DeduplicateFilenames(downloadNames)

	// Create the zip file
	bulkDir := w.downloadCache.BulkZipDir()
	if err := os.MkdirAll(bulkDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create bulk zip directory")
	}

	zipPath := w.downloadCache.BulkZipPath(compositeHash)
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to create zip file")
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for fileID, cachedPath := range cachedPaths {
		filename := dedupedNames[fileID]

		// Use Store method (no compression) since ebook files are already compressed
		header := &zip.FileHeader{
			Name:   filename,
			Method: zip.Store,
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return errors.Wrapf(err, "failed to create zip entry for %s", filename)
		}

		f, err := os.Open(cachedPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open cached file %s", cachedPath)
		}

		_, err = io.Copy(writer, f)
		f.Close()
		if err != nil {
			return errors.Wrapf(err, "failed to write %s to zip", filename)
		}
	}

	// Close the zip writer to flush
	if err := zipWriter.Close(); err != nil {
		return errors.Wrap(err, "failed to finalize zip")
	}

	// Get zip file size
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to stat zip file")
	}

	jobLog.Info(fmt.Sprintf("bulk download zip created: %d files, %d bytes", len(cachedPaths), zipInfo.Size()), nil)

	data.ZipFilename = filepath.Base(zipPath)
	data.SizeBytes = zipInfo.Size()
	data.FileCount = len(cachedPaths)
	data.FingerprintHash = compositeHash

	return w.completeBulkDownloadJob(ctx, job, &data)
}

// completeBulkDownloadJob updates the job data with result fields.
func (w *Worker) completeBulkDownloadJob(ctx context.Context, job *models.Job, data *models.JobBulkDownloadData) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "failed to marshal bulk download result")
	}
	job.Data = string(dataBytes)
	job.DataParsed = data

	return w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
		Columns: []string{"data"},
	})
}
```

- [ ] **Step 2: Wire up the worker — add `downloadCache` field and register process function**

In `pkg/worker/worker.go`:

Add import:
```go
"github.com/shishobooks/shisho/pkg/downloadcache"
```

Add field to `Worker` struct:
```go
downloadCache *downloadcache.Cache
```

Update `New()` signature to accept the cache:
```go
func New(cfg *config.Config, db *bun.DB, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) *Worker {
```

Set it in the worker initialization:
```go
downloadCache: dlCache,
```

Register the process function:
```go
w.processFuncs = map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error{
	models.JobTypeScan:         w.ProcessScanJob,
	models.JobTypeBulkDownload: w.ProcessBulkDownloadJob,
}
```

Set the cleanup skip callback on the cache (after `w` is constructed, use `w.jobService`):
```go
if dlCache != nil {
	dlCache.ShouldSkipCleanup = func() bool {
		hasActive, err := w.jobService.HasActiveJob(context.Background(), models.JobTypeBulkDownload, nil)
		if err != nil {
			return false
		}
		return hasActive
	}
}
```

- [ ] **Step 3: Update `cmd/api/main.go` to create the download cache once and pass it everywhere**

In `cmd/api/main.go`, create the download cache before the worker and server, then pass it to both. This ensures a single cache instance is shared (so the `ShouldSkipCleanup` callback protects all cleanup calls).

```go
// After broker creation, before worker and server creation:
dlCache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())

wrkr := worker.New(cfg, db, pluginManager, broker, dlCache)

srv, err := server.New(cfg, db, wrkr, pluginManager, broker, dlCache)
```

Add import:
```go
"github.com/shishobooks/shisho/pkg/downloadcache"
```

Then update `pkg/server/server.go` to accept the download cache instead of creating its own:

Change `New` signature:
```go
func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) (*http.Server, error) {
```

Remove the existing cache creation (line 90):
```go
// DELETE: downloadCache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())
```

Use `dlCache` instead of `downloadCache` where it's referenced (eReader, Kobo routes, and the new jobs route):
```go
ereader.RegisterRoutes(e, db, dlCache)
kobo.RegisterRoutes(e, db, dlCache)
```

And in `registerProtectedRoutes`, pass `dlCache` through:
```go
func registerProtectedRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) {
```

Update the call in `New`:
```go
registerProtectedRoutes(e, db, cfg, authMiddleware, w, pm, broker, dlCache)
```

And in `registerProtectedRoutes`, pass `dlCache` to jobs registration:
```go
jobs.RegisterRoutesWithGroup(jobsGroup, db, authMiddleware, broker, dlCache)
```

- [ ] **Step 4: Run tests to verify fingerprint/deduplication tests pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && go test ./pkg/worker/ -run "TestComputeBulkFingerprint|TestDeduplicateFilenames" -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && make test`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add pkg/worker/bulk_download.go pkg/worker/worker.go cmd/api/main.go pkg/server/server.go
git commit -m "[Feature] Implement bulk download worker with zip generation"
```

### Task 7: Add job download endpoint

**Files:**
- Modify: `pkg/jobs/handlers.go`
- Modify: `pkg/jobs/routes.go`

- [ ] **Step 1: Add download handler to `pkg/jobs/handlers.go`**

Update the handler struct to include the download cache:

```go
type handler struct {
	jobService    *Service
	broker        *events.Broker
	downloadCache *downloadcache.Cache
}
```

Add the download handler:

```go
func (h *handler) download(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Job")
	}

	job, err := h.jobService.RetrieveJob(ctx, RetrieveJobOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if job.Type != models.JobTypeBulkDownload {
		return errcodes.BadRequest("Job is not a bulk download")
	}

	if job.Status != models.JobStatusCompleted {
		return errcodes.BadRequest("Job is not completed yet")
	}

	var data models.JobBulkDownloadData
	if err := json.Unmarshal([]byte(job.Data), &data); err != nil {
		return errors.WithStack(err)
	}

	if data.FingerprintHash == "" {
		return errcodes.BadRequest("Job has no download data")
	}

	zipPath := h.downloadCache.BulkZipPath(data.FingerprintHash)
	if _, err := os.Stat(zipPath); os.IsNotExist(err) {
		return errcodes.NotFound("Download file has expired from cache")
	}

	filename := fmt.Sprintf("shisho-download-%d-books.zip", data.FileCount)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	return c.File(zipPath)
}
```

Add imports:
```go
"fmt"
"os"
"github.com/segmentio/encoding/json"
"github.com/shishobooks/shisho/pkg/downloadcache"
"github.com/shishobooks/shisho/pkg/models"
```

- [ ] **Step 2: Update route registration**

In `pkg/jobs/routes.go`, update the function signature and handler construction:

```go
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, broker *events.Broker, dlCache *downloadcache.Cache) {
	jobService := NewService(db)

	h := &handler{
		jobService:    jobService,
		broker:        broker,
		downloadCache: dlCache,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/download", h.download)
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceJobs, models.OperationWrite))
}
```

Add import:
```go
"github.com/shishobooks/shisho/pkg/downloadcache"
```

- [ ] **Step 3: Verify server.go changes from Task 6 Step 3 are in place**

The download cache is now passed through `server.New()` → `registerProtectedRoutes()` → `jobs.RegisterRoutesWithGroup()`. This was done in Task 6 Step 3. Verify the jobs registration call uses `dlCache`:

```go
jobs.RegisterRoutesWithGroup(jobsGroup, db, authMiddleware, broker, dlCache)
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && make test`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add pkg/jobs/handlers.go pkg/jobs/routes.go pkg/server/server.go
git commit -m "[Feature] Add bulk download zip download endpoint"
```

### Task 8: Add `bulk` subdirectory to cache init

**Files:**
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add `downloads/bulk` to the cache subdirectories**

In `initCacheDir`, add the bulk subdirectory:

```go
subdirs := []string{
	filepath.Join(dir, "downloads"),
	filepath.Join(dir, "downloads", "bulk"),
	filepath.Join(dir, "cbz"),
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/api/main.go
git commit -m "[Backend] Add bulk download cache subdirectory to init"
```

---

## Chunk 2: Frontend — Context, SSE, Toolbar, and Toast

### Task 9: Create BulkDownload context

**Files:**
- Create: `app/contexts/BulkDownload/context.ts`
- Create: `app/contexts/BulkDownload/BulkDownloadProvider.tsx`
- Create: `app/contexts/BulkDownload/index.ts`

- [ ] **Step 1: Create the context definition**

Create `app/contexts/BulkDownload/context.ts`:

```typescript
import { createContext } from "react";

export interface BulkDownloadProgress {
  jobId: number;
  status: "generating" | "zipping" | "completed" | "failed";
  current: number;
  total: number;
  estimatedSizeBytes: number;
  dismissed: boolean;
}

export interface BulkDownloadContextValue {
  activeDownload: BulkDownloadProgress | null;
  startDownload: (jobId: number, total: number, estimatedSizeBytes: number) => void;
  updateProgress: (jobId: number, status: string, current: number, total: number, estimatedSizeBytes: number) => void;
  completeDownload: (jobId: number) => void;
  failDownload: (jobId: number) => void;
  dismissDownload: () => void;
}

export const BulkDownloadContext = createContext<BulkDownloadContextValue | null>(null);
```

- [ ] **Step 2: Create the provider**

Create `app/contexts/BulkDownload/BulkDownloadProvider.tsx`:

```typescript
import { useCallback, useMemo, useState, type ReactNode } from "react";

import {
  BulkDownloadContext,
  type BulkDownloadContextValue,
  type BulkDownloadProgress,
} from "./context";

export const BulkDownloadProvider = ({ children }: { children: ReactNode }) => {
  const [activeDownload, setActiveDownload] =
    useState<BulkDownloadProgress | null>(null);

  const startDownload = useCallback(
    (jobId: number, total: number, estimatedSizeBytes: number) => {
      setActiveDownload({
        jobId,
        status: "generating",
        current: 0,
        total,
        estimatedSizeBytes,
        dismissed: false,
      });
    },
    [],
  );

  const updateProgress = useCallback(
    (
      jobId: number,
      status: string,
      current: number,
      total: number,
      estimatedSizeBytes: number,
    ) => {
      setActiveDownload((prev) => {
        if (!prev || prev.jobId !== jobId) return prev;
        return {
          ...prev,
          status: status as BulkDownloadProgress["status"],
          current,
          total,
          estimatedSizeBytes,
        };
      });
    },
    [],
  );

  const completeDownload = useCallback((jobId: number) => {
    setActiveDownload((prev) => {
      if (!prev || prev.jobId !== jobId) return prev;
      return { ...prev, status: "completed", dismissed: false };
    });
  }, []);

  const failDownload = useCallback((jobId: number) => {
    setActiveDownload((prev) => {
      if (!prev || prev.jobId !== jobId) return prev;
      return { ...prev, status: "failed", dismissed: false };
    });
  }, []);

  const dismissDownload = useCallback(() => {
    setActiveDownload((prev) => {
      if (!prev) return prev;
      // If completed or failed, clear it entirely
      if (prev.status === "completed" || prev.status === "failed") {
        return null;
      }
      // If still in progress, just mark as dismissed
      return { ...prev, dismissed: true };
    });
  }, []);

  const value: BulkDownloadContextValue = useMemo(
    () => ({
      activeDownload,
      startDownload,
      updateProgress,
      completeDownload,
      failDownload,
      dismissDownload,
    }),
    [
      activeDownload,
      startDownload,
      updateProgress,
      completeDownload,
      failDownload,
      dismissDownload,
    ],
  );

  return (
    <BulkDownloadContext.Provider value={value}>
      {children}
    </BulkDownloadContext.Provider>
  );
};
```

- [ ] **Step 3: Create the index file**

Create `app/contexts/BulkDownload/index.ts`:

```typescript
export { BulkDownloadProvider } from "./BulkDownloadProvider";
export { BulkDownloadContext } from "./context";
export type { BulkDownloadProgress, BulkDownloadContextValue } from "./context";
```

- [ ] **Step 4: Commit**

```bash
git add app/contexts/BulkDownload/
git commit -m "[Frontend] Add BulkDownload context for tracking download progress"
```

### Task 10: Add `useBulkDownload` hook

**Files:**
- Create: `app/hooks/useBulkDownload.ts`

- [ ] **Step 1: Create the hook**

```typescript
import { useContext } from "react";

import { BulkDownloadContext } from "@/contexts/BulkDownload";

export const useBulkDownload = () => {
  const context = useContext(BulkDownloadContext);
  if (!context) {
    throw new Error(
      "useBulkDownload must be used within a BulkDownloadProvider",
    );
  }
  return context;
};
```

- [ ] **Step 2: Commit**

```bash
git add app/hooks/useBulkDownload.ts
git commit -m "[Frontend] Add useBulkDownload hook"
```

### Task 11: Wire SSE events to BulkDownload context

**Files:**
- Modify: `app/hooks/useSSE.ts`

- [ ] **Step 1: Update `useSSE` with bulk download event listeners**

**IMPORTANT:** The bulk download context object changes on every progress update (because `activeDownload` state changes). If we put the context in the `useEffect` dependency array, the EventSource would reconnect on every progress tick. Instead, use a `useRef` to hold the latest callbacks so the effect only depends on stable values.

```typescript
import { useQueryClient } from "@tanstack/react-query";
import { useContext, useEffect, useRef } from "react";

import { BulkDownloadContext, type BulkDownloadContextValue } from "@/contexts/BulkDownload";
import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey as JobsQueryKey } from "@/hooks/queries/jobs";
import { useAuth } from "@/hooks/useAuth";

export function useSSE() {
  const queryClient = useQueryClient();
  const { isAuthenticated } = useAuth();
  const bulkDownload = useContext(BulkDownloadContext);

  // Use ref to avoid the EventSource reconnecting on every progress update
  const bulkDownloadRef = useRef<BulkDownloadContextValue | null>(bulkDownload);
  bulkDownloadRef.current = bulkDownload;

  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }

    const es = new EventSource("/api/events");

    es.onerror = () => {
      console.debug("[SSE] Connection error, will auto-reconnect");
    };

    const handleJobCreated = () => {
      queryClient.invalidateQueries({ queryKey: [JobsQueryKey.ListJobs] });
      queryClient.invalidateQueries({
        queryKey: [JobsQueryKey.LatestScanJob],
      });
    };

    const handleJobStatusChanged = (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data);
        queryClient.invalidateQueries({ queryKey: [JobsQueryKey.ListJobs] });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.LatestScanJob],
        });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.RetrieveJob, String(data.job_id)],
        });
        queryClient.invalidateQueries({
          queryKey: [JobsQueryKey.ListJobLogs, String(data.job_id)],
        });

        // When a scan completes, invalidate book queries so lists refresh
        if (data.status === "completed" && data.type === "scan") {
          queryClient.invalidateQueries({
            queryKey: [BooksQueryKey.ListBooks],
          });
        }

        // Handle bulk download completion/failure
        const bd = bulkDownloadRef.current;
        if (data.type === "bulk_download" && bd) {
          if (data.status === "completed") {
            bd.completeDownload(data.job_id);
          } else if (data.status === "failed") {
            bd.failDownload(data.job_id);
          }
        }
      } catch {
        // Ignore malformed events
      }
    };

    const handleBulkDownloadProgress = (event: MessageEvent) => {
      const bd = bulkDownloadRef.current;
      if (!bd) return;
      try {
        const data = JSON.parse(event.data);
        bd.updateProgress(
          data.job_id,
          data.status,
          data.current,
          data.total,
          data.estimated_size_bytes,
        );
      } catch {
        // Ignore malformed events
      }
    };

    es.addEventListener("job.created", handleJobCreated);
    es.addEventListener("job.status_changed", handleJobStatusChanged);
    es.addEventListener("bulk_download.progress", handleBulkDownloadProgress);

    return () => {
      es.removeEventListener("job.created", handleJobCreated);
      es.removeEventListener("job.status_changed", handleJobStatusChanged);
      es.removeEventListener(
        "bulk_download.progress",
        handleBulkDownloadProgress,
      );
      es.close();
    };
  }, [isAuthenticated, queryClient]);
}
```

- [ ] **Step 2: Commit**

```bash
git add app/hooks/useSSE.ts
git commit -m "[Frontend] Wire bulk download SSE events to context"
```

### Task 12: Wire BulkDownloadProvider into app

**Files:**
- Modify: `app/components/contexts/SSEProvider.tsx`

- [ ] **Step 1: Wrap SSEProvider children with BulkDownloadProvider**

The `BulkDownloadProvider` needs to be an ancestor of `useSSE`, so wrap it around the SSEProvider's children:

```typescript
import type { ReactNode } from "react";

import { BulkDownloadProvider } from "@/contexts/BulkDownload";
import { useSSE } from "@/hooks/useSSE";

function SSEListener() {
  useSSE();
  return null;
}

export function SSEProvider({ children }: { children: ReactNode }) {
  return (
    <BulkDownloadProvider>
      <SSEListener />
      {children}
    </BulkDownloadProvider>
  );
}
```

This ensures `useSSE` can access the `BulkDownloadContext` since the provider wraps it.

- [ ] **Step 2: Commit**

```bash
git add app/components/contexts/SSEProvider.tsx
git commit -m "[Frontend] Wire BulkDownloadProvider into SSEProvider"
```

### Task 13: Create BulkDownloadToast component

**Files:**
- Create: `app/components/library/BulkDownloadToast.tsx`

- [ ] **Step 0: Install the shadcn Progress component**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && npx shadcn@latest add progress`

This creates `app/components/ui/progress.tsx`. Commit it:

```bash
git add app/components/ui/progress.tsx
git commit -m "[Frontend] Add shadcn Progress component"
```

- [ ] **Step 1: Create the toast component**

```typescript
import { Download, Loader2, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { useBulkDownload } from "@/hooks/useBulkDownload";
import { formatFileSize } from "@/utils/format";

export const BulkDownloadToast = () => {
  const { activeDownload, dismissDownload } = useBulkDownload();

  if (!activeDownload || activeDownload.dismissed) {
    return null;
  }

  const { jobId, status, current, total, estimatedSizeBytes } = activeDownload;
  const progressPercent = total > 0 ? Math.round((current / total) * 100) : 0;

  return (
    <div className="fixed bottom-4 right-4 z-50 bg-background border rounded-lg shadow-lg p-4 w-80">
      <div className="flex items-start justify-between gap-2 mb-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          {status === "completed" ? (
            <Download className="h-4 w-4 text-green-600" />
          ) : status === "failed" ? (
            <X className="h-4 w-4 text-destructive" />
          ) : (
            <Loader2 className="h-4 w-4 animate-spin" />
          )}
          <span>
            {status === "completed"
              ? "Download ready"
              : status === "failed"
                ? "Download failed"
                : status === "zipping"
                  ? "Creating zip file..."
                  : `Preparing ${current} of ${total} files...`}
          </span>
        </div>
        <Button
          className="h-6 w-6 shrink-0"
          onClick={dismissDownload}
          size="icon"
          variant="ghost"
        >
          <X className="h-3 w-3" />
        </Button>
      </div>

      {(status === "generating" || status === "zipping") && (
        <Progress className="h-2 mb-2" value={progressPercent} />
      )}

      <div className="text-xs text-muted-foreground">
        {formatFileSize(estimatedSizeBytes)}
      </div>

      {status === "completed" && (
        <Button
          asChild
          className="w-full mt-2"
          size="sm"
        >
          <a href={`/api/jobs/${jobId}/download`}>
            <Download className="h-4 w-4" />
            Download Zip
          </a>
        </Button>
      )}
    </div>
  );
};
```

- [ ] **Step 2: Commit**

```bash
git add app/components/library/BulkDownloadToast.tsx
git commit -m "[Frontend] Create BulkDownloadToast component"
```

### Task 14: Render BulkDownloadToast in Root layout

**Files:**
- Modify: `app/components/pages/Root.tsx`

- [ ] **Step 1: Add BulkDownloadToast to Root**

```typescript
import { Outlet, ScrollRestoration } from "react-router-dom";

import { BulkDownloadToast } from "@/components/library/BulkDownloadToast";
import MobileDrawer from "@/components/library/MobileDrawer";
import { Toaster } from "@/components/ui/sonner";
import { MobileNavProvider } from "@/contexts/MobileNav";

const Root = () => {
  return (
    <MobileNavProvider>
      <ScrollRestoration />
      <div className="flex bg-background font-sans min-h-screen">
        <div className="w-full">
          <Outlet />
        </div>
        <MobileDrawer />
        <Toaster richColors />
        <BulkDownloadToast />
      </div>
    </MobileNavProvider>
  );
};

export default Root;
```

- [ ] **Step 2: Commit**

```bash
git add app/components/pages/Root.tsx
git commit -m "[Frontend] Render BulkDownloadToast in Root layout"
```

### Task 15: Add Download button to SelectionToolbar

**Files:**
- Modify: `app/components/library/SelectionToolbar.tsx`

- [ ] **Step 1: Add Download button with size estimate**

Add imports:
```typescript
import { Download } from "lucide-react";
import { useBulkDownload } from "@/hooks/useBulkDownload";
import { useCreateJob } from "@/hooks/queries/jobs";
import { formatFileSize } from "@/utils/format";
import type { Book } from "@/types";
```

Add a `books` prop for accessing book data:
```typescript
interface SelectionToolbarProps {
  library?: Library;
  books?: Book[];
}
```

Inside the component, add the download logic:

```typescript
const { startDownload } = useBulkDownload();
const createJobMutation = useCreateJob();

// Compute file IDs and estimated size from selected books
const downloadInfo = useMemo(() => {
  if (!books || selectedBookIds.length === 0) return null;

  const fileIds: number[] = [];
  let totalSize = 0;

  for (const bookId of selectedBookIds) {
    const book = books.find((b) => b.id === bookId);
    if (!book?.primary_file_id) continue;
    const primaryFile = book.files?.find((f) => f.id === book.primary_file_id);
    if (primaryFile) {
      fileIds.push(primaryFile.id);
      totalSize += primaryFile.filesize_bytes ?? 0;
    }
  }

  return { fileIds, totalSize };
}, [books, selectedBookIds]);

const handleDownload = async () => {
  if (!downloadInfo || downloadInfo.fileIds.length === 0) return;

  try {
    const job = await createJobMutation.mutateAsync({
      payload: {
        type: "bulk_download",
        data: {
          file_ids: downloadInfo.fileIds,
          estimated_size_bytes: downloadInfo.totalSize,
        },
      },
    });
    startDownload(
      job.id,
      downloadInfo.fileIds.length,
      downloadInfo.totalSize,
    );
    exitSelectionMode();
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Failed to start download";
    toast.error(message);
  }
};
```

Add the Download button in the JSX, before the Delete button:

```tsx
<Button
  disabled={!downloadInfo || downloadInfo.fileIds.length === 0 || createJobMutation.isPending}
  onClick={handleDownload}
  size="sm"
  variant="default"
>
  <Download className="h-4 w-4" />
  {createJobMutation.isPending ? (
    <Loader2 className="h-4 w-4 animate-spin" />
  ) : (
    <>
      Download
      {downloadInfo && downloadInfo.totalSize > 0 && (
        <span className="text-xs opacity-75">
          ({formatFileSize(downloadInfo.totalSize)})
        </span>
      )}
    </>
  )}
</Button>
```

- [ ] **Step 2: Update Home.tsx to pass books to SelectionToolbar**

In `app/components/pages/Home.tsx`, change:
```tsx
<SelectionToolbar library={libraryQuery.data} />
```
to:
```tsx
<SelectionToolbar
  books={booksQuery.data?.books}
  library={libraryQuery.data}
/>
```

- [ ] **Step 3: Run lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && yarn lint`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add app/components/library/SelectionToolbar.tsx app/components/pages/Home.tsx
git commit -m "[Feature] Add Download button to selection toolbar with size estimate"
```

---

## Chunk 3: Validation and Cleanup

### Task 16: Run full validation

- [ ] **Step 1: Run `make check:quiet`**

Run: `cd /Users/robinjoseph/.worktrees/shisho/bulk-download && make check:quiet`
Expected: All checks pass (tests, Go lint, JS lint)

- [ ] **Step 2: Fix any issues found**

If any checks fail, fix and commit the fixes.

### Task 17: Manual smoke test

- [ ] **Step 1: Start dev server**

Run: `make start` (in a separate terminal)

- [ ] **Step 2: Verify the flow**

1. Open the gallery and enter select mode
2. Select multiple books
3. Verify the Download button shows with size estimate
4. Click Download
5. Verify the progress toast appears with progress updates
6. Verify the download completes and the "Download Zip" button works
7. Navigate away during download and verify toast persists
8. Verify dismissing and re-receiving the completion notification
