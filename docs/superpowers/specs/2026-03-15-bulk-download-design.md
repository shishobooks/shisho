# Bulk Download Design

## Overview

Add bulk download functionality to the gallery's select mode. Users select multiple books, click "Download", and receive a zip file containing the primary file (with metadata injected) for each selected book. The operation is backed by the existing job system with SSE progress events, allowing users to navigate away and return to download the completed zip.

## Decisions

- **Files per book**: Primary file only
- **File version**: Metadata-injected (same as single-file download)
- **Zip compression**: Store mode (`zip -0`) — source files are already compressed internally, so re-compression wastes CPU for negligible savings. This also makes the size estimate accurate.
- **Long-running UX**: Job-based with SSE progress events (reuses existing infrastructure)
- **Navigate away**: Supported — job runs in background, toast notification appears on completion
- **Size limits**: None — self-hosted app, user manages their own storage
- **Size estimate**: Computed client-side from already-loaded `file.filesize_bytes` data. Accurate because store mode means zip size ≈ sum of file sizes.
- **Cache strategy**: Zip stored in download cache directory (`bulk/` subdirectory). Individual files cached via existing `GetOrGenerate()`. Both persist for future reuse.
- **Cache cleanup protection**: Skip cache cleanup while a `bulk_download` job is in progress.
- **Notification**: Toast only (no persistent badge). Job history available as fallback.

## Backend

### Job Type & Data Model

**New constant**: `models.JobTypeBulkDownload = "bulk_download"`

**Job data payload** (stored in `job.Data`):

```go
type JobBulkDownloadData struct {
    FileIDs            []int `json:"file_ids"`
    EstimatedSizeBytes int64 `json:"estimated_size_bytes"`
}
```

**Job result** (stored in `job.Data` on completion):

```go
type JobBulkDownloadResult struct {
    ZipFilename     string `json:"zip_filename"`
    SizeBytes       int64  `json:"size_bytes"`
    FileCount       int    `json:"file_count"`
    FingerprintHash string `json:"fingerprint_hash"`
}
```

### Worker Flow (`ProcessBulkDownloadJob`)

1. Parse `JobBulkDownloadData` from job
2. Load all files with book relations from DB (need relations for `ComputeFingerprint` and `GetOrGenerate`)
3. Compute composite fingerprint: sort file IDs, concatenate individual `ComputeFingerprint()` hashes, SHA256 the result
4. Check if `{cacheDir}/bulk/{fingerprintHash}.zip` already exists — if so, mark job complete immediately
5. Iterate files:
   - Call `downloadCache.GetOrGenerate(ctx, book, file)` for each
   - Publish `bulk_download.progress` event after each file (current/total)
   - Check `ctx.Err()` for cancellation between files
6. Create zip at `{cacheDir}/bulk/{fingerprintHash}.zip` using store mode (no compression)
   - Filenames from `FormatDownloadFilename(book, file)`
   - Handle duplicate filenames by appending ` (2)`, ` (3)`, etc.
7. Store `JobBulkDownloadResult` on job, mark complete

### SSE Events

**New event type**: `bulk_download.progress`

```json
{
    "job_id": 1,
    "status": "generating",
    "current": 3,
    "total": 25,
    "estimated_size_bytes": 4500000000
}
```

**Statuses**: `generating` (file-by-file progress) → `zipping` (building zip) → then standard `job.status_changed` with `completed`/`failed`

The `generating` → `zipping` → `completed` progression uses the dedicated event type for granular progress, then falls back to the existing `job.status_changed` event for terminal states. This means existing `useSSE` cache invalidation logic works without modification for job completion.

### Download Endpoint

**`GET /jobs/:id/download`**

1. Look up job by ID, verify it's a `bulk_download` type and `completed` status
2. Parse `JobBulkDownloadResult` from job data
3. Verify zip file exists on disk
4. Serve zip with `Content-Disposition: attachment; filename="shisho-download-{N}-books.zip"`
5. Requires authentication (same as other download endpoints)

### Cache Integration

**Zip storage**: `{cacheDir}/bulk/{fingerprintHash}.zip`

- New `bulk/` subdirectory within the existing download cache directory
- Follows existing flat-file naming pattern (no dot prefix — consistent with codebase)
- Included in cache size calculations for TTL/size-based eviction

**Individual file caching**: Each `GetOrGenerate()` call populates the standard per-file cache. Future single-file downloads benefit from this warm cache.

**Cache cleanup protection**: `TriggerCleanup()` checks for active `bulk_download` jobs (status `in_progress`) before running. If one is active, skip cleanup. Cleanup resumes after the job completes.

**Cache reuse**: Before generating anything, compute composite fingerprint and check for existing zip. If metadata hasn't changed for any of the selected books, the cached zip is served immediately.

### Permissions

- Requires authentication
- Requires `books:read` permission (same as single-file download)
- Library access checked per-file during generation

## Frontend

### Selection Toolbar

Add "Download" button to `SelectionToolbar.tsx` alongside existing Add to List / Merge / Delete actions.

- Icon: `Download` from lucide-react
- Label shows estimated size: `Download (4.2 GB)` computed from `filesize_bytes` of each selected book's primary file
- On click: `POST /jobs` with `{ type: "bulk_download", data: { file_ids: [...], estimated_size_bytes: N } }`
- After job creation: exit selection mode, show progress toast

### Progress Toast

Persistent toast component rendered at the app layout level (survives navigation).

**States:**
1. **Generating**: Progress bar + "Preparing 3 of 25 files..." + estimated size
2. **Zipping**: "Creating zip file..."
3. **Completed**: "Download ready (4.2 GB)" + download button → `GET /jobs/:id/download`
4. **Failed**: Error message

**SSE integration:**
- Listen for `bulk_download.progress` events in `useSSE` hook
- Update toast state on each progress event
- On `job.status_changed` → `completed`: show download button
- On `job.status_changed` → `failed`: show error

**Behavior:**
- Toast appears when job is created
- Persists across page navigation (app-level component)
- Dismissible (doesn't cancel the job — job runs to completion regardless)
- If dismissed and job completes, a new completion toast appears

### Mutation & Query Hooks

- `useCreateBulkDownload()` — wraps `POST /jobs` for the bulk download job type
- Reuses existing `useSSE` hook with new event listener
- No new query hooks needed — job status comes via SSE, not polling

## File Structure

### New files
- `pkg/worker/bulk_download.go` — `ProcessBulkDownloadJob` worker method
- `app/components/library/BulkDownloadToast.tsx` — progress toast component

### Modified files
- `pkg/models/job.go` — add `JobTypeBulkDownload` constant and data structs
- `pkg/worker/worker.go` — register `bulk_download` process function
- `pkg/jobs/handlers.go` — handle `bulk_download` job creation (duplicate prevention optional)
- `pkg/books/routes.go` — add `GET /jobs/:id/download` route (or in `pkg/jobs/routes.go`)
- `pkg/downloadcache/cache.go` — add bulk zip methods, cleanup protection
- `pkg/events/broker.go` — add `NewBulkDownloadProgressEvent` helper
- `app/components/library/SelectionToolbar.tsx` — add Download button
- `app/hooks/useSSE.ts` — add `bulk_download.progress` event listener
- `app/components/App.tsx` (or layout) — render `BulkDownloadToast`
