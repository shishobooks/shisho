# File Hashing & Move Detection — MVP Design

**Date:** 2026-04-12
**Status:** Design
**Related:** [Notion — Investigate perceptual/fuzzy hashing for file & book dedup](https://www.notion.so/33ef24d3107d814c92daf1478800dfd4)

## Goal

Add exact-content file hashing (sha256) to Shisho to preserve file/book identity when files are moved or renamed on disk. Establish the data model and async computation infrastructure as the foundation for future fuzzy/perceptual fingerprints (cover pHash, text SimHash, Chromaprint, etc.), which are deferred to separate follow-up tasks.

## Problem

Files are currently identified by `(filepath, library_id)` with a `UNIQUE` constraint. There is no content hash. This causes two concrete user-facing bugs:

1. **Monitor folder renames.** When a user renames a directory in Finder, fsnotify emits events on the directory path. The monitor expands these into per-file events, sees the old paths are gone, deletes the file rows, and then creates new file/book rows at the new paths. The original book rows — along with any user-edited metadata — are lost. In some cases the recreation races with organize and hits `UNIQUE constraint failed: books.filepath, books.library_id`.

2. **Scan-time reorganizations.** If the server is down while a user reorganizes their library, the monitor never sees the events. On the next scan, files at new paths are treated as brand new, and the old file rows are deleted in the orphan cleanup phase.

Both cases lose the book row and anything the user had manually edited on it. A content hash lets the system match "the file at this new path is the same file that used to be at the old path" and just update the filepath instead of deleting and recreating.

## Non-Goals

This MVP is deliberately narrow. The following are **out of scope** and become separate follow-up tasks:

- **Cross-library move detection** — moving files between libraries requires re-wiring library-scoped entities (persons, genres, series, publishers, imprints). Complex, orthogonal, and will not be tracked as a follow-up task unless it becomes necessary.
- **Cover pHash**, **text SimHash for EPUBs**, **Chromaprint for audiobooks**, **CBZ page fingerprints**, **TLSH fallback** — all fuzzy algorithms deferred to per-algorithm follow-up tasks.
- **Dedup UI** — user-facing "potential duplicates" view with merge/delete actions. Will reuse the fingerprint infrastructure from this MVP.
- **Auto-associating copies with the same book** — when two files have identical content, the MVP creates them as separate books. Consolidation is a user-facing decision that belongs in the future dedup flow.
- **Similarity thresholds, LSH bucketing, cross-install fingerprint sharing, config knobs** — all fuzzy/advanced features.

## Design

### 1. Data Model

New table: `file_fingerprints`.

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Auto-increment |
| `file_id` | INTEGER NOT NULL | FK → `files.id` ON DELETE CASCADE |
| `algorithm` | TEXT NOT NULL | `'sha256'` for MVP. Future: `'phash'`, `'simhash'`, etc. |
| `value` | TEXT NOT NULL | Hex-encoded hash |
| `created_at` | DATETIME | When the fingerprint was computed |

**Constraints and indexes:**

- `UNIQUE(file_id, algorithm)` — one fingerprint per algorithm per file
- `INDEX(algorithm, value)` — fast lookup for move detection ("find file by sha256")
- `file_id → files.id ON DELETE CASCADE` — per project convention for child rows

No `library_id` denormalization: move-detection queries JOIN through `files` to filter by library. The extra JOIN is negligible for a personal library, and avoids keeping two sources of truth for library membership.

**Go model** (`pkg/models/file_fingerprint.go`):

```go
type FileFingerprint struct {
    bun.BaseModel `bun:"table:file_fingerprints"`

    ID        int64     `bun:"id,pk,autoincrement" json:"id"`
    FileID    int64     `bun:"file_id,notnull" json:"file_id"`
    Algorithm string    `bun:"algorithm,notnull" json:"algorithm"`
    Value     string    `bun:"value,notnull" json:"value"`
    CreatedAt time.Time `bun:"created_at,nullzero" json:"created_at"`

    File *File `bun:"rel:belongs-to,join:file_id=id" json:"-"`
}
```

### 2. Hash Computation Package

New package `pkg/fingerprint/`. For the MVP it exposes a single function:

```go
// ComputeSHA256 computes the sha256 hash of a file's contents.
// Reads in streaming chunks; does not load entire file into memory.
// Returns lowercase hex-encoded hash.
func ComputeSHA256(filepath string) (string, error)
```

Future algorithms (pHash, SimHash, Chromaprint) will add new functions in this package following the same pattern. The package is the single home for "how do we compute a fingerprint" so the rest of the codebase never directly reaches for `crypto/sha256`.

### 3. Async Hash Generation Job

New job type: `JobTypeHashGeneration`.

**Job data:**

```go
type JobHashGenerationData struct {
    LibraryID int64 `json:"library_id"`
}
```

**Handler** (`pkg/worker/hash_generation.go`):

1. Query all files in the library without a sha256 fingerprint:
   ```sql
   SELECT f.* FROM files f
   LEFT JOIN file_fingerprints fp
     ON fp.file_id = f.id AND fp.algorithm = 'sha256'
   WHERE f.library_id = ? AND fp.id IS NULL
   ```
2. Spawn a worker pool (`max(NumCPU, 4)`, matching the scan pattern).
3. Each worker: stat the file, verify it exists, call `fingerprint.ComputeSHA256`, insert with `ON CONFLICT DO NOTHING`.
4. Progress logging via `jobLog.Infof` at reasonable intervals.
5. Per-file errors (missing file, permission denied, read error) are logged and the job continues. The job only fails on infrastructure errors (DB unavailable, etc.).

**Deduplication — `EnsureHashGenerationJob(ctx, libraryID)`:**

Before creating a new hash generation job, check whether a pending or in-progress one already exists for this library. If yes, return without creating. If no, create one. Called from:

- End of the scan job (after organize, orphan cleanup, and search index updates)
- End of the monitor batch (when new files were created during a CREATE-only batch)

Idempotent inserts on the hash generation side act as a safety net in case two jobs somehow overlap.

### 4. Monitor Move Detection (real-time path)

The monitor is the primary mechanism for move detection. When a user renames a folder in Finder, the monitor catches it within the debounce window (default 5s).

**Trigger heuristic:** `needsSyncHash = (pending batch contains any REMOVE events)`. REMOVE presence is a cheap signal that the batch might contain moves, and justifies computing sha256 synchronously for CREATE events in the same batch.

**Processing order within `processPendingEvents()`:**

1. **Determine `needsSyncHash`** by scanning the pending map for REMOVE events (file-remove, file-rename, directory-remove).
2. **Process CREATE events first:**
   - If `needsSyncHash`, compute sha256 inline (`fingerprint.ComputeSHA256`) for each CREATE target.
   - Query `file_fingerprints` for any file in this library with matching hash:
     ```sql
     SELECT f.* FROM files f
     JOIN file_fingerprints fp ON fp.file_id = f.id
     WHERE fp.algorithm = 'sha256' AND fp.value = ? AND f.library_id = ?
     ```
   - For each match, `os.Stat` the stored `filepath` to classify it as "path gone" or "path exists."
   - **Selection rule**: if one or more matches have "path gone," the new CREATE is a move. Pick the matched file whose stored `file_modified_at` is most recent and update its `filepath` to the new path. Mark the new CREATE as consumed (no new file row created). Record that file ID so the downstream REMOVE processing skips it.
   - If all matches have "path exists" → it's a copy (same content, two locations on disk). Proceed with normal new-file creation (copy handling is deferred to the future dedup flow).
   - If no match at all → genuinely new file, proceed with normal `scanInternal` create path.
3. **Process REMOVE events second:**
   - For each file targeted by a REMOVE, check whether it was repurposed as a move target above.
   - If repurposed → skip (the row now points to the new path; nothing to delete).
   - If not → delete as today.
4. **At end of batch**, if any new files were created without move detection (either a CREATE-only batch or unmatched CREATEs in a mixed batch), call `EnsureHashGenerationJob` to queue async hashing.

**Directory rename handling** (existing code in `processDirectoryEvent()`): a REMOVE on a directory already expands into per-file remove intents for every file whose path has that directory as a prefix. The new flow fits this naturally — those file IDs land in the REMOVE set, and CREATE events for the new directory's contents match against them via hash.

**Event self-suppression**: `filepath` updates are DB-only writes; they don't touch the disk, so they don't generate fsnotify events. The existing `IgnorePath()` mechanism is not needed here. (To verify during implementation.)

### 5. Scan Move Detection (safety-net path)

The scan is the safety net for renames that happened while the server was offline (so the monitor never saw them).

**New phase inserted between discovery and parallel processing: "move reconciliation."**

After the walk completes, the scan has three sets:

- **Known-seen** — DB rows whose paths were found on disk (normal path: rescan if changed, otherwise skip).
- **Unknown-new** — paths on disk not yet in the DB (candidate new files).
- **Candidate-orphans** — DB rows whose paths were not found on disk (currently deleted in the cleanup phase).

**Reconciliation algorithm:**

1. Build an orphan fingerprint index: for each candidate-orphan that has a sha256 fingerprint, bucket by `filesize_bytes → []orphan`. Orphans without sha256 are excluded (can't match them).
2. **Size-based pruning**: for each candidate-new file, look up its on-disk size in the orphan bucket. If no orphan has that size, skip sync hashing entirely for this file.
3. For candidate-new files that pass the size filter:
   - Compute sha256 synchronously.
   - Look up in the orphan index by `(size, hash)`.
   - **Match** → it's a move. If multiple orphans match, pick the one with the most recent `file_modified_at`.
     - Update the picked orphan's `filepath` to the new path.
     - Remove the file from `unknown-new`.
     - Remove the picked orphan from `candidate-orphans`.
     - Add the picked orphan (with updated path) to a new **rescan-with-new-path** set so metadata gets re-checked if size/mtime changed.
   - **No match** → leave in `unknown-new`.
4. **Proceed with existing parallel processing**:
   - Known-seen → rescan if changed (unchanged).
   - Unknown-new (unmatched) → `scanFileCreateNew` (unchanged).
   - Rescan-with-new-path (moved orphans) → `scanFileByID` with the new path.
5. **Orphan cleanup** at the end: delete any remaining candidate-orphans (unchanged behavior).
6. **End of scan**: call `EnsureHashGenerationJob` to queue async hashing for any files still without fingerprints.

**Cost profile:**

- Clean scan (no orphans) → zero sync hashing, same performance as today.
- First scan after deploy → no fingerprints exist yet, orphan index is empty, reconciliation is a no-op. Move detection begins working after the first hash generation job completes.
- Scan with server-restart-during-rename → sync hashing only for new files whose size matches an orphan's size. Tightly bounded.
- Scan with many unrelated new files + few orphans → bounded by `len(distinct orphan sizes)`, not `len(new_files)`.

### 6. File Change Invalidation

When an existing file's size or mtime changes (indicating content was modified out-of-band), the existing rescan path already reprocesses it. The new behavior: **delete any fingerprints for that file** as part of the rescan, so the next hash generation job will recompute sha256 against the new content. A stale sha256 would cause incorrect move-detection matches.

### 7. Known Limitations

- **First scan after deploy**: move detection doesn't work until the hash generation job has populated fingerprints. Graceful degradation into current behavior.
- **Split-debounce case**: if a rename straddles two monitor debounce windows (REMOVE in window N, CREATE in window N+1), the CREATE-only batch won't trigger sync hashing. The scan path is the safety net here. Rare enough that adding an in-memory "recently removed" cache isn't worth the complexity for the MVP.
- **File content changed AND moved simultaneously**: hash won't match (content differs). Treated as delete + create. Correct, but the user loses custom metadata on the old row.
- **Multiple identical files**: matching prefers an existing file whose path is missing from disk. If multiple orphans have the same hash, pick the most recently modified. If no orphans match, treat as a new copy.

## Testing Strategy

Per project convention: Red-Green-Refactor TDD, all new tests use `t.Parallel()` unless they touch shared global state.

**`pkg/fingerprint/fingerprint_test.go`:**
- `TestComputeSHA256` — fixed content produces known hash
- `TestComputeSHA256_LargeFile` — streaming works for a multi-MB test file
- `TestComputeSHA256_Missing` — returns error for nonexistent file
- `TestComputeSHA256_Permission` — returns error for unreadable file

**`pkg/worker/hash_generation_test.go`:**
- `TestHashGenerationJob` — files without fingerprints get inserted after job runs
- `TestHashGenerationJob_Idempotent` — running twice produces no duplicates or errors
- `TestHashGenerationJob_MissingFile` — logs error, continues with remaining files
- `TestEnsureHashGenerationJob_Dedupe` — second call while first is pending/running doesn't create a duplicate job

**`pkg/worker/scan_move_reconciliation_test.go`:**
- `TestScanMoveReconciliation_SameLibrary` — orphan with fingerprint, new file at different path with same hash → orphan's path updated, no new book created
- `TestScanMoveReconciliation_SizeMismatch` — orphan and new file have different sizes → no reconciliation, orphan deleted, new file created
- `TestScanMoveReconciliation_ContentChanged` — same size but different content → hash mismatch, no reconciliation
- `TestScanMoveReconciliation_NoFingerprint` — orphan has no sha256 yet → excluded from reconciliation, deleted
- `TestScanMoveReconciliation_MultipleMoves` — multiple orphans + multiple new files, all match correctly
- `TestScanMoveReconciliation_InvalidateOnContentChange` — existing file whose size/mtime changed has its fingerprint deleted during rescan

**`pkg/worker/monitor_test.go` additions:**
- `TestMonitor_FolderRename_DetectsMove` — simulated directory rename via fsnotify events preserves file rows and updates paths
- `TestMonitor_NewFolderDrop_NoSyncHashing` — CREATE-only batch doesn't compute hashes inline, queues hash generation job
- `TestMonitor_RenameWithContentChange` — file content changed mid-rename → treated as separate delete + create
- `TestMonitor_PathExists_TreatAsCopy` — hash match but old path still exists → new file created, old file untouched

**E2E (`e2e/`):** one Playwright scenario — upload a book, verify it appears, rename the book's folder on disk, verify the book still appears (not duplicated) and points to the renamed location.

## Documentation Updates

- **`website/docs/supported-formats.md`** — note that file fingerprinting is supported (sha256 only for MVP; format-specific fuzzy algorithms coming later).
- **`website/docs/file-fingerprints.md`** (new) — explain what file hashing is used for, when the monitor detects moves (real-time, Finder renames), when the scan detects moves (safety net for server-down cases), the first-scan-after-deploy limitation, and cross-links to the future dedup feature.
- **`website/docs/metadata.md`** — brief mention and cross-link to the new page.
- **`pkg/CLAUDE.md`** — add a gotcha bullet about the `file_fingerprints` table and the async hash job pattern; note that file change invalidation must delete fingerprints.
- **`pkg/worker/CLAUDE.md`** (if it exists, otherwise the appropriate subdirectory `CLAUDE.md`) — document the monitor's move-detection flow and the scan's reconciliation phase.

## Rollout

- The migration is purely additive: new table, no changes to `files`. Rollback is straightforward via the migration's `down()`.
- First scan after deploy queues a hash generation job that processes every file in every library. For large libraries with multi-GB audiobooks the job may run for a while. It's visible in the jobs UI with progress logging.
- Until the first hash job completes, move detection doesn't work — the feature degrades gracefully into today's behavior.
- No config knobs in the MVP. sha256 is always on; the cost is bounded by the design.

## Follow-Up Tasks (Notion)

The fuzzy/perceptual work from the parent Notion task will be split into per-algorithm follow-up tasks **after this MVP lands**. Each adds a new `algorithm` value in `file_fingerprints`, a new function in `pkg/fingerprint/`, and a hook into the hash generation job:

1. Cover pHash (image fingerprints for all formats)
2. Text SimHash for EPUBs
3. CBZ per-page pHash
4. Image-based PDF per-page pHash
5. Chromaprint for audiobooks (M4B/MP3)
6. TLSH fallback for arbitrary bytes
7. Dedup UI (surface potential duplicates, user-facing merge/delete actions — consumes the above fingerprints)

Cross-library move detection is not being tracked as a follow-up task.
