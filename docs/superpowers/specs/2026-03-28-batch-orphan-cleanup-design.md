# Batch Orphan File Cleanup During Scan

## Problem

Orphan file cleanup (files in DB but no longer on disk) runs sequentially after the parallel scan, calling `scanInternal` individually for each orphan. Each call does a full DB retrieval, `os.Stat()`, conditional logic, and individual deletes. For libraries where many files were deleted between scans, this is slow — ~100 sequential DB round-trips for 100 deleted files.

## Solution

Replace the sequential orphan loop with batch identification, grouping by book, and batch database operations. This reduces orphan cleanup from ~N sequential DB round-trips to ~5 batch operations regardless of N.

## Design

### New file: `pkg/worker/scan_orphans.go`

#### `cleanupOrphanedFiles` method

```go
func (w *Worker) cleanupOrphanedFiles(
    ctx context.Context,
    existingFiles []*models.File,
    scannedPaths map[string]struct{},
    library *models.Library,
    jobLog *joblogs.JobLog,
)
```

Returns no error — all failures are logged as warnings and the method continues. This matches the current behavior where individual `scanInternal` errors are logged and the loop continues.

**Step 1: Collect orphans and group by book**

Single pass over `existingFiles` (which are only main files, from `ListFilesForLibrary`):
- Build `totalFilesByBook: map[int]int` — count of all main files per book
- Build `orphansByBook: map[int][]*models.File` — orphaned files grouped by book ID
- A file is orphaned if its `Filepath` is not in `scannedPaths`

**Step 2: Categorize books**

- **Full orphan books**: `len(orphansByBook[bookID]) == totalFilesByBook[bookID]` — all main files are gone
- **Partial orphan books**: `len(orphansByBook[bookID]) < totalFilesByBook[bookID]` — some files remain

**Step 3: Handle partial orphan books**

Collect all orphaned file IDs across all partial-orphan books into a single slice. Call `bookService.DeleteFilesByIDs(ctx, fileIDs)` to batch-delete them.

For each affected book, check if any deleted file was the primary file. If so, call `bookService.PromoteNextPrimaryFile(ctx, bookID)` to assign a new primary from the remaining files.

**Step 4: Handle full orphan books**

For each full-orphan book (sequential, since supplement promotion requires per-book logic):

1. Load the book with files via `RetrieveBook` to check for supplements
2. Collect remaining supplements (excluding the orphaned main files)
3. Build the supported file types set (built-in + plugin-registered)
4. If a supplement with a supported type exists:
   - Promote it via `bookService.PromoteSupplementToMain(ctx, suppID)`
   - Add the orphaned main file IDs to a "files to delete" batch (handled by `DeleteFilesByIDs`)
5. If no promotable supplement:
   - Add book ID to the "books to delete" batch (handled by `DeleteBooksByIDs`, which cascade-deletes all files, supplements included)
   - Remove book from search index
   - Note: supplement files on disk are NOT explicitly deleted — this matches current `scanFileByID` behavior. Directory cleanup in Step 5 may remove the directory if empty.

After iterating all full-orphan books:
- `bookService.DeleteFilesByIDs(ctx, promotedBookOrphanFileIDs)` — orphaned main files from books where a supplement was promoted (NOT files from books being fully deleted — `DeleteBooksByIDs` handles those)
- `bookService.DeleteBooksByIDs(ctx, bookIDsToDelete)` — fully removed books (cascade-deletes all their files and relations)

**Step 5: Directory cleanup**

Collect unique directories from all orphaned file paths. Build ignore patterns (Shisho special files + supplement exclude patterns). Call `fileutils.CleanupEmptyParentDirectories` for each directory, bounded by the library paths.

### New methods in `pkg/books/service.go`

#### `DeleteFilesByIDs(ctx context.Context, fileIDs []int) error`

Returns nil immediately if `fileIDs` is empty.

Single transaction:
1. `DELETE FROM narrators WHERE file_id IN (?)`
2. `DELETE FROM file_identifiers WHERE file_id IN (?)`
3. `DELETE FROM chapters WHERE file_id IN (?)`
4. `DELETE FROM files WHERE id IN (?)`

Uses `bun.In(fileIDs)` for all IN clauses.

Does not handle primary file promotion — the caller manages that separately since it requires per-book logic.

#### `DeleteBooksByIDs(ctx context.Context, bookIDs []int) error`

Returns nil immediately if `bookIDs` is empty.

Single transaction:
1. `SELECT id FROM files WHERE book_id IN (?)` → collect file IDs
2. If file IDs exist: delete narrators, identifiers, chapters for those files
3. `DELETE FROM files WHERE book_id IN (?)`
4. `DELETE FROM authors WHERE book_id IN (?)`
5. `DELETE FROM book_series WHERE book_id IN (?)`
6. `DELETE FROM book_genres WHERE book_id IN (?)`
7. `DELETE FROM book_tags WHERE book_id IN (?)`
8. `DELETE FROM books WHERE id IN (?)`

Mirrors the existing `DeleteBook()` but operates on multiple books.

#### `PromoteNextPrimaryFile(ctx context.Context, bookID int) error`

Sets the primary file for a book to the next best candidate:
```sql
UPDATE books SET primary_file_id = (
    SELECT id FROM files WHERE book_id = ?
    ORDER BY CASE WHEN file_role = 'main' THEN 0 ELSE 1 END, created_at ASC
    LIMIT 1
) WHERE id = ?
```

Used after batch file deletion to fix up primary file pointers for partially-orphaned books.

### Change in `pkg/worker/scan.go`

Replace lines 417-434 (the orphan cleanup loop):

```go
// Before:
for _, file := range existingFiles {
    if _, seen := scannedPaths[file.Filepath]; !seen {
        _, err := w.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
        ...
    }
}

// After:
scannedPaths := make(map[string]struct{}, len(filesToScan))
for _, path := range filesToScan {
    scannedPaths[path] = struct{}{}
}
w.cleanupOrphanedFiles(ctx, existingFiles, scannedPaths, library, jobLog)
```

## Error Handling

- `cleanupOrphanedFiles` is non-fatal: all errors are logged as warnings, execution continues.
- If `DeleteFilesByIDs` fails, skip directory cleanup for those files.
- If `DeleteBooksByIDs` fails, skip. The existing `cleanupOrphanedEntities` at the end of the scan will catch dangling records on the next scan.
- If `RetrieveBook` fails for a full-orphan book (e.g., already deleted), log and skip that book.

## Testing

- **`DeleteFilesByIDs`**: verify narrators, identifiers, chapters, and files are deleted; empty slice is no-op.
- **`DeleteBooksByIDs`**: verify all book-level relations cascade; empty slice is no-op.
- **`PromoteNextPrimaryFile`**: verify correct promotion order (main preferred, oldest first).
- **`cleanupOrphanedFiles`**: integration-style test with scenarios:
  - Partial orphan: book has 2 main files, 1 orphaned → only orphan deleted, primary promoted if needed
  - Full orphan with promotable supplement → supplement promoted, orphaned files deleted, book preserved
  - Full orphan with no supplements → book and all files deleted
  - Full orphan with non-promotable supplements → book and all files (including supplements) deleted from DB; supplement files left on disk (matches current behavior)

## Files Modified

| File | Change |
|------|--------|
| `pkg/worker/scan.go` | Replace orphan loop with call to `cleanupOrphanedFiles`; extract `scannedPaths` construction |
| `pkg/worker/scan_orphans.go` | New file: `cleanupOrphanedFiles` method |
| `pkg/books/service.go` | New: `DeleteFilesByIDs`, `DeleteBooksByIDs`, `PromoteNextPrimaryFile` |
| `pkg/books/service_test.go` | Tests for new batch methods |
| `pkg/worker/scan_orphans_test.go` | Tests for `cleanupOrphanedFiles` |

## Performance Impact

For a library where 100 files were deleted between scans:
- **Before**: ~100 individual `scanInternal` calls, each doing 3-8 DB queries = ~300-800 DB round-trips
- **After**: ~5 batch queries + 1 query per full-orphan book needing supplement check = ~10-20 DB round-trips
