# PDF Cover Page Selection

## Summary

Extend cover page selection (currently CBZ-only) to work for PDF files. Users can select which page of a PDF to use as the cover, just like CBZ. The selected page is rendered out as a cover image the same way page 0 is during the initial scan.

## Discriminator

The single discriminator for the cover page workflow is `cover_page != null`. If a file has `cover_page` set, it gets the page picker. If not, it gets the upload workflow. This is future-proof — a hypothetical page-based format that doesn't set `cover_page` during scan won't get the cover page workflow.

## Changes

### Backend: `pkg/books/handlers_cover_page.go`

- Remove the `file.FileType != FileTypeCBZ` validation check
- Replace with `file.PageCount == nil` to validate bounds (cover_page already implies page support, but we need page count for bounds checking)
- Route to the correct page cache internally based on file type:
  - CBZ: `h.pageCache.GetPage()`
  - PDF: `h.pdfPageCache.GetPage()`
- Update error messages to be format-agnostic (e.g., "Cover page selection requires a file with pages" instead of "only available for CBZ files")

### Frontend: `app/components/library/FileEditDialog.tsx`

Replace all `file.file_type === FileTypeCBZ` checks in the cover section with `file.cover_page != null`:

- **Cover thumbnail**: Show pending page preview vs current cover
- **Page number badge**: Show "Page N" overlay
- **"Select page" button**: Show for files with cover_page
- **Unsaved changes indicator**: Detect pending cover page changes
- **Page picker dialog**: Render when cover_page is set and file has page_count

The upload button is already hidden via the existing `file.cover_page == null` check — no change needed.

### Component Rename

Rename CBZ-specific component names to generic names:

| Old | New |
|-----|-----|
| `CBZPagePicker.tsx` | `PagePicker.tsx` |
| `CBZPagePreview.tsx` | `PagePreview.tsx` |
| `CBZPageThumbnail.tsx` | `PageThumbnail.tsx` |

Update all imports across the codebase.

### Tests: `pkg/books/handlers_cover_page_test.go`

- Add a PDF test case that sets a cover page and verifies the cover image is extracted
- Update the "rejects non-CBZ files" test to use EPUB (a format without pages) and update the error message assertion

## What stays the same

- `GET /files/:id/page/:pageNum` already handles both CBZ and PDF
- Upload rejection for files with `cover_page` set (line ~1250 in handlers.go)
- Sidecar writing works generically
- Scan worker already sets `CoverPage = 0` for PDFs during initial scan
- `cover_page` field in database model already exists and is used for PDFs
