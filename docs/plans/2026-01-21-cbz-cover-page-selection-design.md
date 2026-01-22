# CBZ Cover Page Selection Design

## Overview

Allow users to select any page from a CBZ file as the cover image, instead of being limited to the auto-detected cover. The selection persists to the database, sidecar, and extracts the page as the external cover file. The actual CBZ file is only modified during download/generation.

## Data Flow

1. User opens FileEditDialog for a CBZ file
2. User clicks "Select from pages" in the cover section
3. CBZPagePicker opens, user selects a page (0-indexed internally)
4. On selection, API call: `PUT /api/books/files/:id/cover-page` with `{ page: number }`
5. Backend:
   - Updates `file.cover_page` in DB
   - Extracts that page from CBZ and saves as `{filename}.cover.{ext}`
   - Updates `file.cover_mime_type` and `file.cover_source = "manual"`
   - Triggers sidecar update
6. Frontend invalidates queries, cover thumbnail updates with cache-busted URL

## API Changes

### New Endpoint

```
PUT /api/books/files/:id/cover-page
Body: { "page": number }  // 0-indexed
```

### Handler Logic

1. Validate file exists and is CBZ type
2. Validate `page` is within bounds: `0 <= page < file.page_count`
3. Extract the page image from CBZ (via existing `cbzpages.GetPage()`)
4. Save as external cover: `{filename}.cover.{ext}` (delete any existing cover first)
5. Update File model:
   - `cover_page = page`
   - `cover_mime_type` from extracted image
   - `cover_source = "manual"`
   - `cover_image_path` to the new cover file path
6. Update sidecar via existing sidecar service
7. Return updated file

## Frontend Changes

### FileEditDialog Cover Section (CBZ files)

- Show current cover thumbnail (using `/api/books/files/:id/cover?v={timestamp}`)
- Below thumbnail: "Page {cover_page + 1}" label showing which page is the cover
- "Select from pages" button that opens CBZPagePicker
- On page selection: call the new API endpoint, invalidate queries

### CBZPagePicker Modifications

- Add optional `title` prop (defaults to existing behavior)
- When used for cover selection, pass `title="Select Cover Page"`
- Highlight the currently selected cover page (pass `currentPage` prop)

### Cache Busting

- Update cover URL construction to append `?v={file.updated_at}` timestamp
- Affects FileEditDialog, book cards, and anywhere covers are displayed

### Query Invalidation

After cover change:
- Invalidate `RetrieveFile` query
- Invalidate `RetrieveBook` query (book cover may derive from file)
- Invalidate `ListBooks` if book covers are shown in lists

## Generation Updates

### CBZ Generation (`pkg/filegen/cbz.go`)

- `updateCoverPage()` already exists and sets FrontCover type in ComicInfo.xml
- Verify it's called when `file.cover_page` is set
- Ensure it uses the `cover_page` value from the File model

### KePub Generation (`pkg/kepub/cbz.go`)

- When converting CBZ to KePub, the cover image should use `file.cover_page`
- Verify the cover selection logic respects this field
- If not, update to extract the correct page as the EPUB cover

## Sidecar Integration

- Ensure `cover_page` field is written to sidecar metadata
- Ensure `cover_page` is read from sidecar during scan (manual data source)

## Implementation Tasks

1. **Backend: New endpoint** - `PUT /files/:id/cover-page` handler
2. **Backend: Verify generation** - CBZ and KePub respect `cover_page`
3. **Frontend: FileEditDialog** - Add cover page selector UI for CBZ files
4. **Frontend: CBZPagePicker** - Add `title` prop for context-appropriate labeling
5. **Frontend: Cache busting** - Add timestamp to cover image URLs
6. **Sidecar integration** - Ensure `cover_page` is written to/read from sidecar
