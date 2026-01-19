# Unified Scan Refactor Design

## Overview

Refactor the scan system to unify batch library scans and single book/file resyncs into a single `Scan()` function. This eliminates code duplication, ensures feature parity, and fixes the "delete if missing" bug.

## Goals

- Single source of truth for all scan/resync logic
- Support three entry points: FilePath (batch), FileID (single file), BookID (book's files)
- Support `ForceRefresh` flag for all scan modes
- Fix missing file cleanup (delete from DB when file no longer on disk)
- Eliminate ~400 lines of duplicate code in `resync.go`

## Core Types

```go
// ScanOptions configures a scan operation.
type ScanOptions struct {
    // Entry points (mutually exclusive - only one should be set)
    FilePath  string // Batch scan: discover/create by path
    FileID    int    // Single file resync: file already in DB
    BookID    int    // Book resync: scan all files in book

    // Context (required for FilePath mode)
    LibraryID int

    // Behavior
    ForceRefresh bool // Bypass priority checks, overwrite all metadata

    // Logging (optional, for batch scan job context)
    JobLog *joblogs.JobLogger
}

// ScanResult contains the results of a scan operation.
type ScanResult struct {
    // For single file scans
    File        *models.File
    Book        *models.Book
    FileCreated bool // True if file was newly created
    FileDeleted bool // True if file was deleted (no longer on disk)
    BookDeleted bool // True if book was also deleted (was last file)

    // For book scans (multiple files)
    Files []*ScanResult // Results for each file in the book
}
```

## Internal Routing

```go
func (w *Worker) Scan(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
    // Validate exactly one entry point is set
    entryPoints := 0
    if opts.FilePath != "" { entryPoints++ }
    if opts.FileID != 0 { entryPoints++ }
    if opts.BookID != 0 { entryPoints++ }
    if entryPoints != 1 {
        return nil, errors.New("exactly one of FilePath, FileID, or BookID must be set")
    }

    // Route to appropriate handler
    switch {
    case opts.BookID != 0:
        return w.scanBook(ctx, opts)
    case opts.FileID != 0:
        return w.scanFileByID(ctx, opts)
    default:
        return w.scanFileByPath(ctx, opts)
    }
}
```

## Internal Functions

### scanBook

```go
func (w *Worker) scanBook(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
    // Fetch book with files
    // If no files, delete book, return BookDeleted: true
    // Otherwise, loop through files calling scanFileByID for each
    // Aggregate results into ScanResult.Files
    // Return updated book
}
```

### scanFileByID

Handles the delete-if-missing logic:

```go
func (w *Worker) scanFileByID(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
    // Fetch file from DB (with relations)
    file, err := w.bookService.RetrieveFileWithRelations(ctx, opts.FileID)
    if err != nil {
        return nil, err
    }

    // Check if file exists on disk
    if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
        // File is gone - delete from DB
        book, _ := w.bookService.RetrieveBook(ctx, file.BookID)
        bookDeleted := len(book.Files) == 1  // Was this the last file?

        // Delete file record
        w.bookService.DeleteFile(ctx, file.ID)

        // If last file, delete orphaned book too
        if bookDeleted {
            w.searchService.DeleteFromBookIndex(ctx, book.ID)
            w.bookService.DeleteBook(ctx, book.ID)
        }

        return &ScanResult{
            FileDeleted: true,
            BookDeleted: bookDeleted,
        }, nil
    }

    // File exists - parse metadata and update
    metadata, err := parseFileMetadata(file.Filepath, file.FileType)
    if err != nil {
        return nil, err
    }

    return w.scanFileCore(ctx, file, metadata, opts.ForceRefresh)
}
```

### scanFileByPath

```go
func (w *Worker) scanFileByPath(ctx context.Context, opts ScanOptions) (*ScanResult, error) {
    // Check if file already exists in DB
    existingFile, err := w.bookService.RetrieveFile(ctx, opts.FilePath, opts.LibraryID)

    if existingFile != nil {
        // Existing file - delegate to scanFileByID (which handles delete-if-missing)
        return w.scanFileByID(ctx, ScanOptions{
            FileID:       existingFile.ID,
            ForceRefresh: opts.ForceRefresh,
            JobLog:       opts.JobLog,
        })
    }

    // New file - verify it exists on disk before creating records
    if _, err := os.Stat(opts.FilePath); os.IsNotExist(err) {
        return nil, nil  // Skip silently for batch scan
    }

    // Create new file/book records, then call scanFileCore
    // ... (existing creation logic from scanFile)
}
```

### scanFileCore

The shared metadata update logic (extracted from existing `scanFile` lines ~788-1600):

```go
func (w *Worker) scanFileCore(
    ctx context.Context,
    file *models.File,
    book *models.Book,
    metadata *mediafile.ParsedMetadata,
    forceRefresh bool,
) (*ScanResult, error) {
    // Update book scalar fields (all 8)
    // - Title / SortTitle
    // - Subtitle
    // - Description
    // - Publisher
    // - Imprint
    // - Release Date

    // Update book relationships (all 4)
    // - Authors
    // - Series
    // - Genres
    // - Tags

    // Update file scalar fields
    // - Name (CBZ)
    // - URL

    // Update file relationships
    // - Narrators (M4B)
    // - Identifiers

    // Write sidecar files
    // Update search index

    // Return updated file and book
}
```

## Batch Scan Orchestration

`ProcessScanJob` handles orphan cleanup after the filesystem walk:

```go
func (w *Worker) ProcessScanJob(ctx context.Context, job *models.Job) error {
    // Track which files we've seen during this scan
    seenFileIDs := make(map[int]struct{})

    // Walk filesystem and scan each file
    for path := range walkLibraryPaths(library) {
        result, err := w.Scan(ctx, ScanOptions{
            FilePath:     path,
            LibraryID:    library.ID,
            ForceRefresh: job.ForceRefresh,  // future: support this on jobs
            JobLog:       jobLog,
        })
        if err != nil {
            log.Warn("failed to scan file", ...)
            continue
        }
        if result != nil && result.File != nil {
            seenFileIDs[result.File.ID] = struct{}{}
        }
    }

    // Cleanup: delete files/books in DB that weren't seen
    existingFiles := w.bookService.ListFilesForLibrary(ctx, library.ID)
    for _, file := range existingFiles {
        if _, seen := seenFileIDs[file.ID]; !seen {
            // File no longer on disk - delete it
            w.Scan(ctx, ScanOptions{FileID: file.ID})
            // scanFileByID will detect missing file and delete it
        }
    }
}
```

## Handler Changes

Handlers become thin wrappers:

```go
func (h *handler) resyncFile(c echo.Context) error {
    fileID := parseID(c.Param("id"))
    params := ResyncPayload{}
    c.Bind(&params)

    // Check library access (existing logic)

    result, err := h.worker.Scan(ctx, worker.ScanOptions{
        FileID:       fileID,
        ForceRefresh: params.Refresh,
    })
    if err != nil {
        return errcodes.ValidationError(err.Error())
    }

    if result.FileDeleted {
        return c.JSON(http.StatusOK, map[string]any{
            "file_deleted": true,
            "book_deleted": result.BookDeleted,
        })
    }
    return c.JSON(http.StatusOK, result.File)
}

func (h *handler) resyncBook(c echo.Context) error {
    // Same pattern, using ScanOptions{BookID: id}
}
```

## Code Changes

### To delete:
- `pkg/worker/resync.go` (~400 lines)
- `FileRescanner` interface from `pkg/books/handlers.go`
- `FileRescannerOptions`, `FileRescannerResult`, `BookRescannerResult` types

### To refactor:
- `pkg/worker/scan.go` - extract `scanFileCore` from existing `scanFile`
- Keep all metadata update logic, reorganize into new structure

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| FileID not found in DB | Return 404 error |
| BookID not found in DB | Return 404 error |
| File on disk is unreadable (permissions) | Return error with details |
| File parsing fails (corrupt EPUB, etc.) | Return 422 with parse error |
| Book has no files | Delete book, return `BookDeleted: true` |
| Concurrent scan + resync on same file | SQLite handles concurrent writes; last write wins |

## Migration Strategy

1. Create new `Scan()` function and internal helpers alongside existing code
2. Migrate handlers to use new `Scan()`
3. Migrate `ProcessScanJob` to use new `Scan()`
4. Delete old `resync.go` and unused code
5. Run tests throughout
