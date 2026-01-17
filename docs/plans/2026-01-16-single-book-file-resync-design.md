# Single Book/File Resync Design

## Overview

Add the ability to resync metadata for individual books and files, with two modes:
- **Scan for new metadata**: Respects priority (manual > file_metadata > filepath)
- **Refresh all metadata**: Bypasses priority, overwrites all metadata from source files

## API

### Endpoints

```
POST /books/:id/resync
POST /files/:id/resync
```

### Request Body

```json
{
  "refresh": false
}
```

- `refresh: false` → Normal scan respecting priority
- `refresh: true` → Bypass priority checks, overwrite all fields

### Response

Returns the updated book/file object directly (inline processing, no job created).

### Error Responses

| Scenario | Status | Behavior |
|----------|--------|----------|
| Book/file not found in DB | 404 | Return error message |
| File on disk missing | 200 | Delete file record from DB; if last file in book, delete book too |
| Book has no files | 200 | Delete orphaned book record from DB |
| File parsing error | 422 | Return error details (e.g., "Failed to parse EPUB: invalid OPF structure") |

## Backend Implementation

### Handler Functions

Add to `pkg/books/handlers.go`:
```go
func (h *Handler) ResyncBook(c echo.Context) error
```

Add to `pkg/files/handlers.go`:
```go
func (h *Handler) ResyncFile(c echo.Context) error
```

### Scan Logic Changes

Modify `pkg/worker/scan_helpers.go` to accept a `forceRefresh` parameter:

```go
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string, forceRefresh bool) bool {
    if forceRefresh {
        return newValue != ""  // Update if we have a value, ignore priority
    }
    // ... existing priority logic
}

func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string, forceRefresh bool) bool {
    if forceRefresh {
        return len(newItems) > 0  // Update if we have items, ignore priority
    }
    // ... existing priority logic
}
```

### Reusable Scan Function

Extract per-file scanning code from `ProcessScanJob` into:
```go
func ScanSingleFile(filepath string, forceRefresh bool) (*models.File, error)
```

### Book Resync Scope

When rescanning a book, rescan all its files too (book metadata often derives from file metadata).

## Frontend Implementation

### UI Location

Add to existing context menus ("..." dropdowns):
- Book card context menu (grid/list views)
- File row context menu (book detail page)

### Menu Items

Two separate items:
1. "Scan for new metadata"
2. "Refresh all metadata"

### Confirmation Dialog

Show for "Refresh all metadata" only:

```
Title: Refresh All Metadata

Body: This will rescan the [book/file] and overwrite all metadata
with values from the source file(s). Any manual changes you've
made will be lost.

Buttons: [Cancel] [Refresh]
```

### Mutation Hooks

Add to `app/hooks/queries/`:

```ts
useResyncBook(bookId: number)
useResyncFile(fileId: number)
```

Both accept `{ refresh: boolean }` as the mutation payload.

### Cache Invalidation

```ts
// useResyncBook
onSuccess: (updatedBook, { bookId }) => {
  queryClient.invalidateQueries({ queryKey: ['book', bookId] })
  queryClient.invalidateQueries({ queryKey: ['books'] })
}

// useResyncFile
onSuccess: (updatedFile, { fileId }) => {
  queryClient.invalidateQueries({ queryKey: ['file', fileId] })
  queryClient.invalidateQueries({ queryKey: ['book', updatedFile.bookId] })
}
```

### Feedback

- Loading: Spinner/disabled state while processing
- Success: Toast notification ("Metadata refreshed" / "Metadata scanned")
- Error: Toast with error message

## Concurrency

No locking needed. If a library scan job is running concurrently:
- The inline resync completes quickly
- The library scan will overwrite with the same values when it reaches that file
- SQLite handles concurrent writes
