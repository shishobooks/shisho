# PDF Viewer Design

## Overview

Add an in-app PDF viewer to Shisho that renders PDF pages as images on the server (via pdfium WASM) and displays them using a shared reader component extracted from the existing CBZ reader. This provides a consistent reading experience across formats without adding client-side PDF rendering dependencies.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Rendering approach | Server-side (pdfium → JPEG) | Consistent with CBZ, reuses existing pdfium, no new JS deps |
| Frontend architecture | Shared PageReader component | ~90% code reuse, single reading UX across formats |
| DPI | Configurable, default 200 | Balances quality and file size; admin-tunable |
| JPEG quality | Configurable, default 85 | Good visual quality at reasonable size; admin-tunable |
| Chapters | Extract PDF outline during scan | Bookmark tree maps naturally to existing Chapter model |
| Image format | JPEG only | Smallest files, good enough for rendered book pages |

## Architecture

### Backend

#### New Package: `pkg/pdfpages/`

Mirrors `pkg/cbzpages/`. Renders individual PDF pages via pdfium WASM and caches them to disk.

**Interface:**
```go
func (c *Cache) GetPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error)
```

**Behavior:**
- Cache path: `{cacheDir}/pdf/{fileID}/page_{pageNum}.jpg`
- On cache miss: open PDF with pdfium, render page at configured DPI, encode as JPEG at configured quality, write to cache
- On cache hit: return cached file path immediately
- Page numbers are 0-indexed (consistent with CBZ)
- Validates page bounds against PDF page count
- Returns `image/jpeg` MIME type

**Thread safety:**
- pdfium WASM requires single-threaded access
- Use a mutex around pdfium render calls
- Cache lookup (file-exists check) does not need locking

**Error handling:**
- Invalid page number → error (handler returns 400)
- Corrupt/unreadable PDF → error (handler returns 500)
- pdfium init failure → error with context

**Cache invalidation:**
Same strategy as CBZ — cached pages live indefinitely. When a source file changes and is re-scanned, it gets a new file ID, so old cache entries are naturally orphaned. Cache cleanup follows whatever strategy `cbzpages` uses.

#### Handler Extension: `pkg/books/handlers.go`

The existing `getPage` handler rejects non-CBZ files. Extend the routing logic:

```
switch file.FileType:
  case "cbz" → cbzpages.Cache.GetPage(...)
  case "pdf" → pdfpages.Cache.GetPage(...)
  default    → 400 "unsupported file type"
```

Everything else stays the same: library access check, cache headers (`Cache-Control: public, max-age=31536000, immutable`), content-type from the cache's returned MIME type.

#### PDF Outline Extraction

Integrated into the existing file scanning pipeline. When a PDF is processed, after metadata extraction:

1. Call a new `pdf.ExtractOutline(path)` function
2. Returns `[]pdf.OutlineEntry` (lightweight struct with `Title string` and `StartPage int`, 0-indexed) — avoids coupling `pkg/pdf` to `pkg/models`
3. Scanner converts `[]pdf.OutlineEntry` to chapter records the same way CBZ chapter detection works (see `pkg/cbz/chapters.go` for the pattern)
4. PDFs without bookmarks produce no chapters (graceful no-op)

#### Config Fields: `pkg/config/config.go`

```go
PDFRenderDPI     int `yaml:"pdf_render_dpi"`     // default: 200, range: 72-600
PDFRenderQuality int `yaml:"pdf_render_quality"`  // default: 85, range: 1-100
```

Defaults applied when zero/missing. Validated at config load time.

Must also update:
- `shisho.example.yaml` — add both fields with comments
- `website/docs/configuration.md` — document both fields

### Frontend

#### New Shared Component: `app/components/pages/PageReader.tsx`

Extracted from `CBZReader.tsx`. Contains all format-agnostic reading UI:

**Props:**
```typescript
interface PageReaderProps {
  fileId: number;
  bookId: number;
  libraryId: string;
  totalPages: number;
  getPageUrl: (pageNum: number) => string;  // 0-indexed
}
```

**Owns:**
- Current page state, synced to `?page=N` URL param
- Keyboard navigation (Arrow keys, A/D)
- Click-zone navigation (left/right thirds of screen)
- Page preloading via `<link rel="prefetch">` using `getPageUrl`
- Viewer settings fetch/update (preload count, fit mode) via existing `useViewerSettings` hook
- Chapter selector dropdown via existing `useFileChapters(fileId)` hook
- Progress bar with chapter markers and click-to-seek
- Header bar (back button, page counter, chapter selector, settings popover)
- End-of-book redirect to book detail page

#### Refactored: `app/components/pages/CBZReader.tsx`

Becomes a thin wrapper that receives the file record from `FileReader` as a prop:
- Derives `page_count` from the file record
- Passes `getPageUrl = (n) => \`/api/books/files/${fileId}/page/${n}\`` to PageReader

#### New: `app/components/pages/PDFReader.tsx`

Structurally identical to CBZReader wrapper:
- Receives file record as prop from `FileReader`
- Same `getPageUrl` pattern (same backend endpoint)
- Exists as a separate component so format-specific features can be added later without bloating a shared wrapper

#### New: `app/components/pages/FileReader.tsx`

Dispatcher component at the route level:
- Extracts `fileId`, `bookId`, `libraryId` from route params
- Fetches the file record (single fetch, shared across wrappers)
- Switches on `file_type` → renders `CBZReader` or `PDFReader`, passing the file record as a prop
- Adding future formats requires only a new case here, no router changes

#### Router: `app/router.tsx`

Single route, unchanged path:
```
/libraries/:libraryId/books/:bookId/files/:fileId/read → <FileReader />
```

Replaces the direct `<CBZReader />` reference with `<FileReader />`.

#### BookDetail: `app/components/pages/BookDetail.tsx`

Extend the "Read" button condition from CBZ-only to include PDF:
```typescript
{(file.file_type === FileTypeCBZ || file.file_type === FileTypePDF) && (
  // Read button with BookOpen icon, same link pattern
)}
```

Both places in BookDetail with this pattern (main file row and primary action area) get updated.

### File Structure

```
pkg/pdfpages/
  cache.go           ← new: PDF page rendering and caching
  cache_test.go      ← new: tests

pkg/pdf/
  pdf.go             ← modified: add ExtractOutline function
  outline_test.go    ← new: outline extraction tests

pkg/books/
  handlers.go        ← modified: extend getPage for PDF routing

pkg/config/
  config.go          ← modified: add PDFRenderDPI, PDFRenderQuality

app/components/pages/
  PageReader.tsx      ← new: shared reader component
  FileReader.tsx      ← new: file-type dispatcher
  CBZReader.tsx       ← modified: refactored to thin wrapper
  PDFReader.tsx       ← new: thin wrapper
```

## Testing

**Backend:**
- `pkg/pdfpages/cache_test.go` — Page rendering and caching with a small test PDF. Covers: correct JPEG output, cache hit on second call, page bounds validation, invalid PDF handling.
- `pkg/pdf/outline_test.go` — Outline extraction with a PDF that has bookmarks and one without. Covers: correct chapter titles and start pages, empty result for no-outline PDFs.
- Handler tests extended if existing `getPage` tests exist.

**Frontend:**
- No new component tests. The PageReader refactor is a behavioral no-op for CBZ (existing behavior preserved). PDF wrapper is structurally identical. Existing CBZ reader tests (if any) should continue passing.

## Documentation Updates

- `website/docs/configuration.md` — Add `pdf_render_dpi` and `pdf_render_quality` fields
- `website/docs/supported-formats.md` — Update PDF row to indicate in-app reading is supported
- `shisho.example.yaml` — Add both new config fields with comments

## Out of Scope

- Text selection or in-page search (server-rendered images)
- Annotation or highlight support
- Two-page spread mode (could be added later to PageReader for both formats)
- Password-protected PDF support
