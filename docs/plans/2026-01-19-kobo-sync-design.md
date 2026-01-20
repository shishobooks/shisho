# Kobo Sync Design

Wireless book transfer to Kobo e-readers by impersonating Kobo's sync API.

## Overview

Users modify their Kobo's config file to point `api_endpoint` at Shisho instead of `storeapi.kobo.com`. Shisho then serves library content as if it were purchased Kobo books, with automatic KePub conversion.

**MVP Scope:** Wireless book transfer only (no progress tracking yet).

## URL Structure

```
/kobo/{apiKey}/all/v1/...              # All libraries user can access
/kobo/{apiKey}/library/{id}/v1/...     # Specific library
/kobo/{apiKey}/list/{id}/v1/...        # Specific list
```

The scope segment determines which books appear in sync responses. The Kobo device stores this full URL in its config, so users choose their scope once during setup.

The `/v1/` is part of Kobo's API that we're impersonating—the device expects these paths.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/initialization` | Device init, custom image URL templates |
| `POST /v1/auth/device` | Dummy auth (returns placeholder tokens) |
| `GET /v1/library/sync` | Book list with change tracking |
| `GET /v1/library/{id}/metadata` | Book metadata (proxy to Kobo if not found) |
| `GET /v1/books/{id}/file/epub` | Download as KePub |
| `GET /v1/books/{id}/thumbnail/{w}/{h}/...` | Cover images |
| `*` (catch-all) | Proxy to real Kobo store |

## Permission

New `kobo_sync` permission on API keys, separate from `ereader_browser`.

Each API key should correspond to a single Kobo device—the UI should make this clear.

## Database Schema

### kobo_sync_points

Tracks the state of the library at each sync, per API key.

```sql
CREATE TABLE kobo_sync_points (
    id TEXT PRIMARY KEY,
    api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL,
    completed_at DATETIME  -- NULL while sync is in progress
);
CREATE INDEX idx_kobo_sync_points_api_key ON kobo_sync_points(api_key_id);
```

### kobo_sync_point_books

Snapshot of each book's state at the sync point.

```sql
CREATE TABLE kobo_sync_point_books (
    id TEXT PRIMARY KEY,
    sync_point_id TEXT NOT NULL REFERENCES kobo_sync_points(id) ON DELETE CASCADE,
    file_id INTEGER NOT NULL,
    file_hash TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    metadata_hash TEXT NOT NULL,
    synced BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_sync_point_books_sync_point ON kobo_sync_point_books(sync_point_id);
CREATE INDEX idx_sync_point_books_file ON kobo_sync_point_books(file_id);
```

**Why file-level:** Kobo downloads individual files. A book with multiple EPUB files would appear as separate entries. Tracking at file level matches how the device sees content.

## Sync Protocol

### Sync Token

The Kobo sends an `X-Kobo-SyncToken` header containing its last sync state. We use base64-encoded JSON:

```json
{
  "lastSyncPointId": "abc123"
}
```

Empty or missing token means first sync—return all books.

### Sync Flow

1. Kobo requests `GET /v1/library/sync` with sync token
2. Parse token to get `lastSyncPointId` (or null for first sync)
3. Create new sync point snapshot of current library state (based on URL scope)
4. Compare against last sync point to find:
   - **New books:** file_id exists in new but not old
   - **Changed books:** file_id exists in both, but hash differs
   - **Removed books:** file_id exists in old but not new
5. Return results as JSON with Kobo's expected format
6. Set `X-Kobo-SyncToken` header with new sync point ID
7. Mark sync point as completed

### Change Detection

```go
newBooks := filesInNewPoint.Except(filesInOldPoint)
removedBooks := filesInOldPoint.Except(filesInNewPoint)
changedBooks := filesInBoth.Where(oldHash != newHash)
```

**First Sync:** When no previous sync point exists, all books in scope are returned as "new."

## Kobo API Response Formats

All responses use **PascalCase** JSON keys to match Kobo's API expectations.

### Sync Response

Returns an array of change objects:

```json
// New or changed book
{
  "NewEntitlement": {
    "BookEntitlement": {
      "Accessibility": "Full",
      "ActivePeriod": { "From": "2024-01-01T00:00:00Z" },
      "Created": "2024-01-01T00:00:00Z",
      "CrossRevisionId": "shisho-{fileId}",
      "Id": "shisho-{fileId}",
      "IsHiddenFromArchive": false,
      "IsLocked": false,
      "IsRemoved": false,
      "LastModified": "2024-01-15T00:00:00Z",
      "OriginCategory": "Imported",
      "RevisionId": "shisho-{fileId}-{hash}",
      "Status": "Active"
    },
    "BookMetadata": {
      "ContributorRoles": [{ "Name": "Author Name" }],
      "CoverImageId": "shisho-{fileId}",
      "Description": "Book description...",
      "DownloadUrls": [{ "Format": "EPUB", "Url": "..." }],
      "EntitlementId": "shisho-{fileId}",
      "Language": "en",
      "PublicationDate": "2024-01-01",
      "Publisher": { "Name": "Publisher" },
      "Series": { "Name": "Series", "Number": 1 },
      "Title": "Book Title"
    }
  }
}

// Deleted book
{
  "ChangedEntitlement": {
    "BookEntitlement": {
      "Id": "shisho-{fileId}",
      "IsRemoved": true
    }
  }
}
```

**ID Format:** Prefix all IDs with `shisho-` to avoid collisions with real Kobo content.

## Kobo Store Proxy

Unhandled requests are forwarded to the real Kobo store so users retain access to store features.

### Proxy Logic

```go
func (h *KoboHandler) proxyToKoboStore(c echo.Context) error {
    targetURL := "https://storeapi.kobo.com" + stripPrefix(c.Request().URL.Path)

    proxyReq, _ := http.NewRequest(c.Request().Method, targetURL, c.Request().Body)

    // Copy relevant headers from device
    for _, h := range []string{"Authorization", "X-Kobo-SyncToken"} {
        if v := c.Request().Header.Get(h); v != "" {
            proxyReq.Header.Set(h, v)
        }
    }

    resp, err := httpClient.Do(proxyReq)
    // ... stream response to client
}
```

### Routing

- Any endpoint we don't explicitly handle → proxy
- Metadata for non-Shisho books (`GET /v1/library/{id}/metadata` where ID doesn't start with `shisho-`) → proxy
- After returning our sync changes, also proxy to Kobo store and merge results

## Initialization Endpoint

Proxy to real Kobo store, then inject custom image URLs so covers load from Shisho.

```go
func (h *KoboHandler) handleInitialization(c echo.Context) error {
    koboResp := h.proxyToKoboStore(c)

    var data map[string]interface{}
    json.Unmarshal(koboResp.Body, &data)

    resources := data["Resources"].(map[string]interface{})
    baseURL := h.getBaseURL(c)

    resources["image_host"] = baseURL
    resources["image_url_template"] = baseURL +
        "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg"
    resources["image_url_quality_template"] = baseURL +
        "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg"

    return c.JSON(http.StatusOK, data)
}
```

## Backend Architecture

### Package Structure

```
pkg/kobo/
    routes.go       -- Register routes under /kobo/{apiKey}/{scope}/v1/...
    middleware.go   -- Extract API key, validate kobo_sync permission, parse scope
    handlers.go     -- Endpoint handlers (init, auth, sync, download, covers)
    proxy.go        -- Kobo store proxy logic
    dto.go          -- Kobo API request/response types (PascalCase)
    sync.go         -- Sync point creation, change detection logic
    model.go        -- KoboSyncPoint, KoboSyncPointBook structs
```

### Middleware Chain

```go
func (h *KoboHandler) RegisterRoutes(e *echo.Echo) {
    kobo := e.Group("/kobo/:apiKey")
    kobo.Use(h.authMiddleware)      // Validate API key + kobo_sync permission
    kobo.Use(h.scopeMiddleware)     // Parse scope, store in context

    scoped := kobo.Group("/:scope")
    scoped.GET("/v1/initialization", h.handleInitialization)
    scoped.POST("/v1/auth/device", h.handleAuth)
    scoped.GET("/v1/library/sync", h.handleSync)
    scoped.GET("/v1/library/:bookId/metadata", h.handleMetadata)
    scoped.GET("/v1/books/:bookId/file/epub", h.handleDownload)
    scoped.GET("/v1/books/:imageId/thumbnail/:w/:h/*", h.handleCover)
    scoped.Any("/*", h.proxyToKoboStore)
}
```

### Scope Context

```go
type SyncScope struct {
    Type      string  // "all", "library", "list"
    LibraryID *int
    ListID    *int
}
```

## Frontend Changes

### Security Settings Page

**API Key Creation:**
- Label "Device Name" when `kobo_sync` is selected
- Placeholder: "e.g., Kobo Libra 2"
- Helper text: "Create one API key per Kobo device"

**API Key Card (with `kobo_sync` permission):**

```
┌─────────────────────────────────────────────────────────────┐
│ Kobo Libra 2                              Last used: 2h ago │
│ ak_7kx9•••••••••                   [👁] [📋] [Setup] [Delete] │
│ ☑ eReader Browser  ☑ Kobo Sync                              │
└─────────────────────────────────────────────────────────────┘
```

**Setup Modal (for Kobo Sync):**

1. **Scope Selection:** Radio buttons
   - "All Libraries"
   - "Specific Library" → dropdown
   - "Specific List" → dropdown
2. **Generated URL:** Based on selection
3. **Setup Instructions:**
   - Connect Kobo via USB
   - Navigate to `.kobo/Kobo/Kobo eReader.conf`
   - Find `api_endpoint=https://storeapi.kobo.com`
   - Replace with generated URL
   - Eject and sync
4. **Copy URL Button**

## Downloads & Covers

### Book Downloads

Leverage existing KePub conversion from `pkg/kepub/` and `pkg/filegen/`.

```go
func (h *KoboHandler) handleDownload(c echo.Context) error {
    bookId := c.Param("bookId")

    fileId, err := parseShishoId(bookId)
    if err != nil {
        return h.proxyToKoboStore(c)
    }

    file, err := h.fileService.GetByID(fileId)
    if err != nil {
        return echo.NotFoundHandler(c)
    }

    kepubPath, err := h.downloadCache.GetOrGenerateKepub(file)
    if err != nil {
        return err
    }

    return c.File(kepubPath)
}
```

### Cover Images

```go
func (h *KoboHandler) handleCover(c echo.Context) error {
    imageId := c.Param("imageId")
    width := c.Param("w")
    height := c.Param("h")

    fileId, err := parseShishoId(imageId)
    if err != nil {
        return h.proxyToKoboStore(c)
    }

    file, err := h.fileService.GetByID(fileId)
    if err != nil || file.CoverImagePath == nil {
        return echo.NotFoundHandler(c)
    }

    resized, err := h.resizeCover(*file.CoverImagePath, width, height)
    if err != nil {
        return err
    }

    return c.Blob(http.StatusOK, "image/jpeg", resized)
}
```

Use Go stdlib (`image`, `image/jpeg`, `image/png`) and `golang.org/x/image/draw` for resizing. Cache resized covers to avoid repeated processing.

## Edge Cases & Notes

### Kobo Protocol Quirks

- Kobo sends malformed HTTP requests with unescaped `[` and `]` in query params
- All JSON responses must use PascalCase keys
- `X-Kobo-SyncToken` header must be valid base64

### File Filtering

Only EPUB and CBZ files sync (M4B audiobooks don't make sense on an e-reader):

```go
query.Where("f.file_type IN (?)", bun.In([]string{"epub", "cbz"}))
```

### Deleted Files

When a file is removed or moved out of scope, the next sync returns it with `IsRemoved: true`.

### Metadata Hash

Detect metadata changes without file changes:

```go
func metadataHash(file *File, book *Book) string {
    h := sha256.New()
    h.Write([]byte(book.Title))
    h.Write([]byte(strings.Join(authorNames, ",")))
    h.Write([]byte(file.CoverImagePath))
    return hex.EncodeToString(h.Sum(nil))[:16]
}
```

## Future: Progress Tracking

When adding progress sync:
- Extend `kobo_sync_point_books` with `progress_hash` field
- Add handlers for `GET/PUT /v1/library/{id}/state`
- Store progress in R2Locator format (converts to/from Kobo's kobo.x.y format)
