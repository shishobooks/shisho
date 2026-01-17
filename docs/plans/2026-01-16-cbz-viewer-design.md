# CBZ Viewer Design

This document describes the design for a web-based CBZ viewer that allows users to read comic book files directly in the browser.

## Overview

The CBZ viewer enables page-by-page reading of CBZ files with:
- Image preloading for instant page turns
- Keyboard and button navigation
- Persistent user preferences
- Deep-linked URLs for bookmarking
- Chapter-aware progress tracking

## API Endpoints

### Page Serving

```
GET /books/files/:id/page/:pageNum
```

- Accepts 0-indexed page number
- Extracts image from CBZ on first request
- Caches extracted images to `${CACHE_DIR}/cbz/{file_id}/page_{num}.{ext}`
- Returns cached version on subsequent requests
- Includes `Cache-Control` headers for browser caching
- Returns 404 if page number exceeds `PageCount`

### User Settings

New package: `pkg/settings/`

```
GET  /settings/viewer    - Get current user's viewer settings
PUT  /settings/viewer    - Update viewer settings
```

Request/response payload:
```json
{
  "preload_count": 3,
  "fit_mode": "fit-height"
}
```

Valid `fit_mode` values: `"fit-height"` (default), `"original"`

## Database Schema

### New Table: `user_settings`

```sql
CREATE TABLE user_settings (
    id INTEGER PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id INTEGER NOT NULL UNIQUE,
    viewer_preload_count INTEGER NOT NULL DEFAULT 3,
    viewer_fit_mode TEXT NOT NULL DEFAULT 'fit-height',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)
```

### Go Model

```go
type UserSettings struct {
    bun.BaseModel `bun:"table:user_settings"`

    ID                 int       `bun:"id,pk,autoincrement" json:"id"`
    CreatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
    UpdatedAt          time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
    UserID             int       `bun:",notnull,unique" json:"user_id"`
    ViewerPreloadCount int       `bun:",notnull,default:3" json:"viewer_preload_count"`
    ViewerFitMode      string    `bun:",notnull,default:'fit-height'" json:"viewer_fit_mode"`
}
```

Design notes:
- One row per user, created on first settings update (or lazily on GET with defaults)
- Future viewer settings (reading direction, theme, etc.) can be added as columns
- `ON DELETE CASCADE` ensures cleanup when user is deleted

## Frontend

### Route

```
/libraries/:libraryId/books/:bookId/files/:fileId/read?page=0
```

The `page` query param enables deep linking - refreshing preserves position.

### Component Hierarchy

```
CBZReader.tsx (page component)
├── ReaderHeader.tsx
│   ├── Back button (→ book detail page)
│   ├── Chapter dropdown (if chapters exist)
│   └── Settings gear button (opens SettingsPanel)
├── PageDisplay.tsx
│   ├── Current page image
│   └── Preloaded images (hidden, for cache warming)
├── ReaderControls.tsx
│   ├── Previous/Next buttons
│   └── Page indicator ("Page 1 of 236")
├── ProgressBar.tsx
│   ├── Clickable progress track
│   ├── Chapter markers (vertical lines)
│   └── Current chapter name display
└── SettingsPanel.tsx (slide-out or popover)
    ├── Preload count slider (1-10)
    └── Fit mode toggle (fit-height / original)
```

## Preloading Strategy

When on page N with preload count P (default 3):
- Preload pages: N-P through N+P (clamped to valid range)
- Use hidden `<img>` elements to trigger browser cache
- On page change, update preload set and remove stale entries

Example: on page 5 with preloadCount=3
- Preloaded: [2, 3, 4, 5, 6, 7, 8]
- User navigates to page 6
- New preload: [3, 4, 5, 6, 7, 8, 9]
- Remove: 2, Add: 9

## Navigation

### Keyboard

| Key | Action |
|-----|--------|
| Arrow Right | Next page |
| Arrow Left | Previous page |
| D | Next page |
| A | Previous page |

### Behavior

- At last page, "next" returns to book detail page
- At first page, "previous" does nothing

### URL Sync

- Use `useSearchParams` to read/write `page` param
- On page change, update URL: `setSearchParams({ page: newPage.toString() })`
- On mount, read initial page from URL (default to 0)

## Progress UI

### Visual Structure

```
┌─────────────────────────────────────────────────────────────┐
│ ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │
│         │              │                    │               │
│      Ch.1           Ch.2                 Ch.3            Ch.4
└─────────────────────────────────────────────────────────────┘
         "Chapter 2: The Journey Begins"        Page 45 of 236
```

### Implementation

- Progress fill width: `(currentPage / (totalPages - 1)) * 100%`
- Chapter markers positioned at: `(chapter.start_page / (totalPages - 1)) * 100%`
- Clicking on the bar jumps to that page (calculate from click position)

### Current Chapter Detection

Chapters are sorted by `sort_order`. The current chapter is the last one in sorted order where `start_page <= currentPage`:

```tsx
const currentChapter = chapters
  .filter(ch => ch.start_page != null && ch.start_page <= currentPage)
  .at(-1)
```

### Styling

- Progress bar: thin (4-6px), subtle background, accent color fill
- Chapter markers: small vertical lines (8-10px tall), slightly transparent
- Hover on marker shows chapter name tooltip
- Current chapter name below progress bar, left-aligned

## Cache Directory Structure

```
${CACHE_DIR}/
└── cbz/
    └── {file_id}/
        ├── page_0.jpg
        ├── page_1.jpg
        ├── page_2.png
        └── ...
```

Pages are extracted on-demand and cached indefinitely. Cache can be cleared manually or via future cache management feature.
