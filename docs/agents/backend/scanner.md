# Scanner, Monitor, and File Organization

The background worker (`pkg/worker/`) runs jobs from the database queue. The main job is the scan; `pkg/worker/scan_unified.go` holds the per-file paths (`scanFileByPath`, `scanFileByID`, `scanFileCreateNew`, `scanFileCore`). Format parsing lives in the format packages (`pkg/epub`, `pkg/cbz`, `pkg/mp4`, `pkg/pdf`, `pkg/kepub`), each with its own `AGENTS.md`. Search indexing during scans is in `search-fts.md`; metadata merging and provenance in `data-sources-and-merges.md`.

## Library monitor (`monitor.go`)

Watches library paths with fsnotify, debounces events, and triggers targeted single-file rescans.

- **Directory events fan out.** fsnotify emits Remove/Rename for the directory itself, not its files, so those are queued as `pendingEvent{IsDirectory: true}` and expand to per-file cleanup for every DB file under that path. Without it, removing or renaming a book folder orphans its rows.
- **Move detection by content hash.** When a batch contains any REMOVE, the monitor hashes that batch's CREATE files synchronously and looks them up in `file_fingerprints`. A match whose stored path is gone has its row's `filepath` repurposed instead of delete plus recreate, preserving book identity and user edits. The scan job does the same reconciliation after its walk, for renames made while the server was offline.
- Hashes come from a `hash_generation` job queued at the end of every scan and every monitor batch that creates files. A size or mtime change invalidates the fingerprint so the next run rehashes.
- **kqueue watch setup.** `watchRecursive` adds directories through `addDirWatch`. On macOS/BSD, fsnotify's `Add` lists the directory and fails with not-exist when a file vanishes mid-listing (organize deleting an old sidecar during a folder rename). `addDirWatch` retries once with `Remove` then `Add` while the directory still exists, because a plain second `Add` is a no-op on kqueue. The `Remove` also clears the directory from its parent's seen set, so the parent's next change sends a Create and `handleEvent` re-queues the directory's files (skipping at debug level if it is gone). Only a second failure, or any other error, logs "failed to watch directory". Tests drive this through the `dirWatcher` seam.

## Unreadable files

- **`files.scan_error`** records a parse failure for a file already in the library. `scanFileByID` sets it (innermost cause only, e.g. `zip: not a valid zip file`) and clears it on the next good parse. New files that fail to parse are never inserted, so their only signal is the job log warning. `fileContentChanged` treats a flagged file as changed so the size/mtime shortcut cannot strand the flag, and `tryDetectMove` rescans a repurposed row that carries one.
- **Only a file-caused parse failure becomes `books.ErrFileUnreadable`.** The scanner wraps the error only when `isUnparseableFile` blames the file itself, not a filesystem, plugin runtime, or cancellation error, so the resync route's 422 (with a fixed message, since parse errors name library paths) never hides a server fault.
- **Sidecar removal waits for a successful parse.** When a file is swapped on disk or scanned with refresh, decide `discardSidecar` before the parse and call `removeFileSidecar` after it. Removing first and then failing to parse destroys the last on-disk record of the metadata.
- **`checkExpectedMimeType` is the one content check for built-in extensions**, called by both the scan walker (`ProcessScanJob`) and the monitor's new-file path (`processEvent`), so neither can import what the other rejects. `.m4b` accepts `audio/x-m4a` (`M4A `), `audio/mp4` (`M4B `), and `video/mp4` (`isom`/`mp42`); all are real audiobooks from different tools. Files already in the DB skip the check in both places, including a Create on a tracked path (temp file plus rename), which is rescanned so an unreadable replacement gets flagged.
- **A file row whose Book is missing** (left by a delete that ran without foreign keys) goes to `scanOrphanedFile`: it deletes the row, deletes the Book's other rows with `DeleteOrphanedBookChildren` once no file points at it, and re-imports a file still on disk through `scanFileCreateNew` with `FileCreated` set. Returning an error here instead would make every scan fail on that path forever.

## Scan cache and supplements

`ScanCache.knownFiles`, preloaded in `ProcessScanJob`, must hold main **and** supplement files: preload with `ListAllFilesForLibrary`, not the main-only `ListFilesForLibrary`. Supplements can share scannable extensions with main files (a demoted `Cribsheet.pdf` beside `Cribsheet.epub`); every scannable file becomes a scan target, and a supplement missing from the cache falls through to `scanFileCreateNew` and fails on `UNIQUE(filepath, library_id)`. `scanFileByPath` returns `&ScanResult{File: existing}` early for a cached supplement, since supplements have no metadata to rescan.

**Full-scan orphan cleanup decides a missing supplement from the disk, never from `filesToScan`.** `filesToScan` (and `scannedPaths`) holds only scannable extensions, so `.txt` or `.jpg` supplements never appear in it. `missingSupplements` in `scan_orphans.go` loads supplement rows at cleanup time (so a row moved during the scan is checked at its current path) and stats each; only not-exist counts as missing, other stat errors keep the row and warn. An unreadable library root aborts the scan before cleanup. Missing supplements are deleted before main-file orphans are handled, so promotion never picks one and deleting one never deletes its Book. Discovery otherwise runs only when a main file is first imported, so cleanup reruns it (`rediscoverSupplements`) for each surviving Book that lost a supplement; otherwise a renamed supplement, or one moved with a reconciled folder, drops off its Book for good.

## Auto-classified supplement PDFs

`scanFileCreateNew` creates a new PDF as `FileRole=Supplement`, with no cover extraction, when its basename matches `config.PDFSupplementFilenames` and a sibling main file (EPUB, CBZ, M4B, or plugin-registered extension) exists on disk or a book row already exists at the same `bookPath`. Keep the `if !classifyAsSupplement { ... }` guard around cover extraction. Rescans do not re-run the rule: existing main PDFs with matching names keep their role.

## Chapters on scan

`scanFileCore` syncs `ParsedMetadata.Chapters` after file metadata is saved, through `chapterService.ReplaceChapters` (atomic); a failure fails the file's scan. Positions: `StartPage` (0-indexed) for CBZ and PDF, `StartTimestampMs` for M4B, `Href` for EPUB.

- **Negative start pages are dropped, whatever the source.** Parsed chapters (built-in and plugin parsers) and sidecar chapters (via `convertSidecarChapters`) pass through `dropNegativeStartPageChapters`, which drops the chapter and its children and warn-logs path, source, count, and whether the rest was stored. Older scans wrote page-less PDF bookmarks to sidecars as -1, and plugins can return any negative. A stored negative page shows as "Page 0", is skipped by downloads, and makes `validateChapters` reject every save on the file. If all are dropped, existing chapters stay (`ShouldUpdateChapters` never applies an empty list).
- Enricher search results carry no chapters (`parseSearchResponse` never reads them).
- The PUT route's `validateChapters` requires CBZ/PDF `start_page >= 0` and `< file.PageCount` when known, and M4B `start_timestamp_ms <= AudiobookDurationSeconds * 1000`.

## File organization

`scanInternal(FilePath)` defers organization (see `pkg/AGENTS.md`). The two existing callers: `ProcessScanJob` collects `booksToOrganize` and organizes in a batch after the walk; `Monitor.processPendingEvents` collects book IDs from `FileCreated` results and calls `organizeBooks()`.

**Resync narrator changes trigger organization after relationship persistence.** The earlier filename check sees pre-scan narrators, so include M4B narrator updates in the post-`UpdateBookRelationships` condition even when Title and Authors are unchanged. A hybrid book's EPUB may restore Authors before its M4B restores Narrators. Restoring or replacing a series membership during resync also reorganizes (`seriesChanged`) when the book has a main CBZ.

### Reorganizing after an API edit

When a path-affecting field changes and the library has `OrganizeFileStructure` on, the handler reorganizes. Path-affecting fields: `file.Name` and `file.Narrators` (filename), `book.Title` and `book.Authors` (directory), and `book.BookSeries` membership and series number fields (CBZ and hybrid folder suffix). Removals count: Identify sends a present but empty series collection to clear memberships, which must stay distinct from an absent field.

- For directory-backed books, folder organization owns the book sidecar, so rename the files inside with `RenameOrganizedFileOnly`, not `RenameOrganizedFile`. A file once moved from the library root can carry a leftover basename sidecar, and renaming it can overwrite the folder sidecar and restore stale metadata on the next scan.
- Title comes from `file.Name` when set (see "File-level vs book-level fields" in `pkg/AGENTS.md`).

```go
if fieldChanged && library.OrganizeFileStructure {
    organizeOpts := fileutils.OrganizedNameOptions{
        AuthorNames:   authorNames,
        NarratorNames: narratorNames,
        Title:         title,
        FileType:      file.FileType,
        Claimed:       bookService.FilepathClaimedByOtherFile(ctx, file.LibraryID, file.ID),
    }
    newPath, err := fileutils.RenameOrganizedFileOnly(file.Filepath, organizeOpts)
    if err != nil {
        return errors.WithStack(err)
    }
    if newPath != file.Filepath {
        // Moves the file back if the write fails.
        if err := bookService.RecordOrganizedFilepath(ctx, file, file.Filepath, newPath, false); err != nil {
            return errors.WithStack(err)
        }
    }
}
```

**Folder renames** record the Book and all its files in one transaction and rename the folder back on failure (`recordRenamedBookFolder`). The same transaction rewrites every `books` and `files` row at or under the old folder, in any library, because a nested Book moves with it on disk; organize then reindexes those nested Books, since callers reindex only the organized Book. The prefix match is a byte range (`underFolder`), not LIKE, so a sibling `Foo 2/`, LIKE wildcards, and a case-only difference are left alone.
