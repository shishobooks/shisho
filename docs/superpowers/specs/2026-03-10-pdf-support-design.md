# First-Class PDF Support

## Overview

Add PDF as a fourth built-in file type alongside EPUB, CBZ, and M4B. PDFs will be scanned as main files with full metadata extraction, cover generation, and metadata embedding on download.

## Parser (`pkg/pdf/`)

New package following the pattern of `pkg/epub/`, `pkg/cbz/`, `pkg/mp4/`.

**Public API:**

```go
func Parse(path string) (*mediafile.ParsedMetadata, error)
```

**Metadata extraction (via pdfcpu):**

- Title, Author, Subject (mapped to description), Keywords (mapped to tags), CreationDate (mapped to release date) from PDF info dictionary
- Page count from page tree
- Multiple authors: split Author field on `,` / `&` / `;`
- Keywords: split on `,` / `;`
- Identifiers: PDF has no standardized identifier field — not extracted

**Cover extraction (two-tier):**

1. **Primary — pdfcpu:** Extract embedded images from page 1, pick the largest by pixel area. Handles publisher ebook PDFs with full-page cover images.
2. **Fallback — go-pdfium WASM:** Render page 1 to JPEG at ~150 DPI. Handles text-only covers, vector artwork, and any PDF where no suitable embedded image exists.

**Data source:** `models.DataSourcePDFMetadata` (new constant).

**Chapters:** Not extracted in this iteration. Can be added later via PDF document outlines/bookmarks.

**Future enhancement:** XMP metadata parsing for richer fields (multiple creators with roles, identifiers, series info).

## File Generator (`pkg/filegen/pdf.go`)

New `PDFGenerator` implementing the `Generator` interface.

**Metadata written back via pdfcpu info dict manipulation:**

| PDF Field | Source |
|-----------|--------|
| Title | `book.Title` |
| Author | Book authors joined with `, ` |
| Subject | `book.Description` |
| Keywords | Book tags joined with `, ` |
| CreationDate | `file.ReleaseDate` |
| Producer | Preserved from original |
| Creator | Preserved from original |

**Behavior:**

- Copies source PDF to temp destination path, modifies metadata in the copy, serves the copy. Original file on disk is never touched.
- Same atomic write pattern as CBZ/EPUB generators.
- Accepts `context.Context` and checks for cancellation before expensive operations.

**KePub:** Not supported for PDF (returns `ErrKepubNotSupported`, same as M4B).

## Scanner Integration

**File discovery (`pkg/worker/scan.go`):**

- Add `.pdf` to `extensionsToScan` with MIME type `application/pdf`
- PDFs in library directories discovered as main files during scans

**Metadata parsing (`pkg/worker/scan_unified.go`):**

- Add `case models.FileTypePDF:` to `parseFileMetadata` switch, calling `pdf.Parse(path)`

**Existing supplement PDFs:**

- Not upgraded on rescan. The scanner matches files by filepath to existing DB records. Files already in the DB as supplements are re-scanned via `scanFileByID`, which preserves their `FileRole`. Only newly discovered PDFs (no existing DB record) are created as main files.

**Supplement discovery behavior change:**

- Once PDF is added to `extensionsToScan`, the `isMainFileExtension` function will return `true` for `.pdf`. This means `discoverSupplements` will skip PDFs — new PDFs in a book directory will be discovered as main files, not supplements. This is intentional: PDFs are now first-class and should create main file records.

**Cover and page count:**

- Cover uses the same `extractAndSaveCover` flow. Cover bytes come from `ParsedMetadata.CoverData`.
- Page count stored in `file.PageCount` (reuses the existing field, currently CBZ-only).

**Cover recovery (`recoverMissingCover`):**

- Add `case models.FileTypePDF:` to re-extract cover from the source PDF if the cover file is missing on disk.

## Model & Constant Changes

**`pkg/models/file.go`:**

- Add `FileTypePDF = "pdf"` constant
- Update `tygo:emit` comment to include PDF in `FileType` union:
  ```go
  //tygo:emit export type FileType = typeof FileTypeCBZ | typeof FileTypeEPUB | typeof FileTypeM4B | typeof FileTypePDF;
  ```

**Data source (`pkg/models/data-source.go`):**

- Add `DataSourcePDFMetadata = "pdf_metadata"` constant
- Add to `tygo:emit` comment for the `DataSource` TypeScript union
- Add to `dataSourcePriority` map at priority 3 (same as epub_metadata, cbz_metadata, m4b_metadata)

**No database migration needed.** The `file_type` column is a string. PDFs already exist as supplements with `file_type = "pdf"`.

**No new model fields needed.** PDF reuses `PageCount` (shared with CBZ). `CoverPage` is not used (cover always from page 1).

## Cover Aspect Ratio Selection

Three `selectCoverFile` functions use a `switch f.FileType` to categorize files as "book" or "audiobook" aspect ratio. PDF must be added to the book branch alongside EPUB and CBZ:

- `pkg/books/handlers.go` — `selectCoverFile` for API cover endpoint
- `pkg/opds/service.go` — `selectCoverFile` for OPDS feed covers
- `pkg/ereader/handlers.go` — `selectCoverFile` for eReader browser covers

## OPDS Integration

Three changes in `pkg/opds/`:

- Add `MimeTypePDF = "application/pdf"` constant alongside existing MIME type constants
- Add PDF case to `FileTypeMimeType()` returning `MimeTypePDF`
- Add `"pdf"` to `validateFileTypes` whitelist so OPDS clients can filter by `?types=pdf`

## eReader Browser Integration

- PDF files will be offered for download like other main file types. The download flow already handles arbitrary file types.
- PDF does not support KePub conversion (already handled by the `SupportsKepub` function).
- Add PDF to file type badge display if there is a file type switch in the eReader templates.

## Kobo Sync

PDFs are **excluded** from Kobo sync. The existing filter in `pkg/kobo/service.go` only includes EPUB and CBZ, which are formats Kobo devices can read natively. Kobo devices cannot read PDFs via the sync protocol. This exclusion is intentional and requires no code changes.

## Frontend

- `FileType` TypeScript union updated automatically via tygo
- File type badges/labels render `file.file_type` as a string — PDF appears naturally
- Cover display unchanged (same `cover_image_filename` system)
- Page count already displayed for CBZ; works for PDF automatically
- No in-browser PDF reader in this iteration. Users download and read in their preferred reader.

## Dependencies

| Dependency | Purpose | License | Size Impact |
|-----------|---------|---------|-------------|
| `github.com/pdfcpu/pdfcpu` | Info dict parsing, image extraction, metadata writing | Apache-2.0 | ~2-3 MB binary |
| `github.com/klippa-app/go-pdfium` | WASM-based PDF page rendering for cover fallback | MIT/BSD/Apache-2.0 | ~15-25 MB binary |

**Build compatibility:**

- Both pure Go (`CGO_ENABLED=0` compatible)
- go-pdfium WASM has compiler support for amd64 and arm64
- No new Alpine packages in Docker
- No CGo required

**Initialization:**

- go-pdfium WASM runtime initialized lazily on first PDF cover extraction, not at server startup

## Documentation Updates

- `website/docs/supported-formats.md` — promote PDF to full entry alongside EPUB/CBZ/M4B with metadata field table
- `website/docs/metadata.md` — add PDF section describing extracted/written fields
- `pkg/pdf/CLAUDE.md` — new file documenting PDF conventions
- `pkg/CLAUDE.md` — update "File Types" section to include PDF

## Testing

- `pkg/pdf/pdf_test.go` — metadata extraction, both cover tiers, edge cases (missing info dict, no images, encrypted PDFs)
- `pkg/filegen/pdf_test.go` — metadata written back correctly, original file unchanged
- Scanner tests — verify new PDFs discovered as main files, existing supplement PDFs not upgraded
- OPDS tests — verify PDF MIME type and type filter
- Cover aspect ratio tests — verify PDF covers selected in book branch
- Test fixtures in `pkg/pdf/testdata/` — small crafted PDFs for various scenarios

## Sidecar & Fingerprint

- No new sidecar fields needed (PDF uses same metadata fields as other formats)
- No fingerprint changes needed (existing fields cover PDF's metadata)
- `FormatDownloadFilename()` already uses `file.FileType` directly as extension — no changes needed
