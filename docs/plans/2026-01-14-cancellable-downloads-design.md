# Cancellable Downloads Design

> **Status:** Implemented - see commit history on `task/cancel-download` branch

## Problem

Downloads can take a while, especially KePub conversions. If a user clicks download by accident, they're stuck waiting with no way to stop it. The current UI shows a spinner but no cancel option.

## Solution

Add a cancel button that appears during download generation. When clicked:
- Frontend aborts the HTTP request
- Backend detects the cancelled context and stops all generation work
- Any partial/temp files are cleaned up

## Frontend Changes

### State (`BookDetail.tsx`)

Add an `AbortController` ref:
```typescript
const downloadAbortController = useRef<AbortController | null>(null)
```

### Modified `handleDownloadWithEndpoint()`

```typescript
const handleDownloadWithEndpoint = async (fileId: number, endpoint: string) => {
  setDownloadError(null)
  setDownloadingFileId(fileId)

  // Create abort controller for this download
  const abortController = new AbortController()
  downloadAbortController.current = abortController

  try {
    const response = await fetch(endpoint, {
      method: "HEAD",
      signal: abortController.signal,
    })

    if (!response.ok) {
      // Fetch error body with GET...
    }

    // Trigger actual download
    window.location.href = endpoint
  } catch (error) {
    // Don't show error dialog for user-initiated cancellation
    if (error instanceof DOMException && error.name === "AbortError") {
      return
    }
    // Handle other errors...
  } finally {
    downloadAbortController.current = null
    setDownloadingFileId(null)
  }
}
```

### Cancel Handler

```typescript
const handleCancelDownload = () => {
  downloadAbortController.current?.abort()
  setDownloadingFileId(null)
}
```

### UI

Show a cancel button (X icon) next to the spinner when `downloadingFileId` is set.

## Backend Changes

### Handler Changes (`pkg/books/handlers.go`)

Pass request context to cache functions:

```go
func (h *Handler) downloadFile(c echo.Context) error {
    ctx := c.Request().Context()
    // ...
    cachedPath, err := h.downloadCache.GetOrGenerate(ctx, book, file, h.config)
    // ...
}

func (h *Handler) downloadKepubFile(c echo.Context) error {
    ctx := c.Request().Context()
    // ...
    cachedPath, err := h.downloadCache.GetOrGenerateKepub(ctx, book, file, h.config)
    // ...
}
```

### Cache Changes (`pkg/downloadcache/cache.go`)

Update function signatures to accept context:

```go
func (c *Cache) GetOrGenerate(ctx context.Context, book *models.Book, file *models.File, config *config.Config) (string, error)

func (c *Cache) GetOrGenerateKepub(ctx context.Context, book *models.Book, file *models.File, config *config.Config) (string, error)
```

Pass context to generators and check for cancellation before writing cache metadata.

### Generator Changes

Update all file generators to accept context and check for cancellation:

**`pkg/filegen/epub.go`:**
```go
func GenerateEPUB(ctx context.Context, ...) error {
    // Check cancellation before expensive operations
    if err := ctx.Err(); err != nil {
        return err
    }
    // ... generation logic with periodic ctx.Err() checks
}
```

**`pkg/filegen/cbz.go`:**
```go
func GenerateCBZ(ctx context.Context, ...) error {
    for _, page := range pages {
        if err := ctx.Err(); err != nil {
            return err
        }
        // process page
    }
}
```

**`pkg/filegen/m4b.go`:**
```go
func GenerateM4B(ctx context.Context, ...) error {
    // Similar pattern
}
```

**`pkg/kepub/kepub.go`:**
```go
func ConvertToKepub(ctx context.Context, ...) error {
    // Check cancellation during transformation steps
}
```

**`pkg/kepub/cbz.go`:**
```go
func ConvertCBZToKepub(ctx context.Context, ...) error {
    for _, page := range pages {
        if err := ctx.Err(); err != nil {
            return err
        }
        // process page
    }
}
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| User cancels during cache check | No cleanup needed |
| User cancels during generation | Temp files cleaned via `defer`; partial output deleted |
| User cancels after generation completes | File stays cached; user just doesn't get it this time |
| Cancel right as generation finishes | File cached normally; race is harmless |

## Files to Modify

| File | Changes |
|------|---------|
| `app/components/pages/BookDetail.tsx` | AbortController ref, cancel button, abort error handling |
| `pkg/downloadcache/cache.go` | Add ctx param to GetOrGenerate/GetOrGenerateKepub |
| `pkg/books/handlers.go` | Pass c.Request().Context() to cache functions |
| `pkg/filegen/epub.go` | Accept context, add cancellation checks |
| `pkg/filegen/cbz.go` | Accept context, add cancellation checks |
| `pkg/filegen/m4b.go` | Accept context, add cancellation checks |
| `pkg/kepub/kepub.go` | Accept context, add cancellation checks |
| `pkg/kepub/cbz.go` | Accept context, add cancellation checks |

## Not Modified

- `downloadOriginalFile` - serves files directly, no generation needed
