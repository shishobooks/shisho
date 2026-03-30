# Enricher Cover Resolution Gate

## Problem

Three related issues with how enricher (plugin) covers are handled during the scan pipeline:

1. **Enricher covers are computed but never saved.** In `scanFileCreateNew`, the file's embedded cover is saved to disk (line 2120) *before* enrichers run (line 2222). The enriched metadata flows into `scanFileCore`, but there is no code to persist enricher cover data. Similarly, during rescans (`scanFileByID`), enrichers run but cover data is not applied.

2. **No resolution gate.** When enrichers provide covers, there is no check that the enricher cover is actually better (higher resolution) than the current cover. A low-resolution enricher image could replace a high-quality embedded cover.

3. **Page-based format protection uses `CoverPage != nil` instead of file type.** The current guard at `scan_unified.go:2770` and `books/handlers.go:1250` blocks external covers only when `CoverPage` is set. For PDFs where cover extraction fails, `CoverPage` is nil, which would allow an external cover despite the format being page-based. Both backend and frontend have this issue.

## Design

### 1. File Type Helper

Add a centralized helper to determine if a file type is page-based (derives cover from page content).

**Backend** (`pkg/models/file.go`):
```go
func IsPageBasedFileType(fileType string) bool {
    return fileType == FileTypeCBZ || fileType == FileTypePDF
}
```

**Frontend** (`app/libraries/utils.ts` or similar):
```ts
export const isPageBasedFileType = (fileType: string): boolean =>
  fileType === "cbz" || fileType === "pdf";
```

### 2. Change Guards from `CoverPage != nil` to File Type

Replace all guard-style `CoverPage != nil` checks with the file type helper.

**Backend changes:**

| Location | Current | New |
|----------|---------|-----|
| `pkg/books/handlers.go:1250` | `file.CoverPage != nil` | `models.IsPageBasedFileType(file.FileType)` |
| `pkg/worker/scan_unified.go:2770` | `metadata.CoverPage != nil` | `models.IsPageBasedFileType(fileType)` — needs `fileType` passed into `runMetadataEnrichers` |

**Frontend changes:**

| Location | Current | New |
|----------|---------|-----|
| `IdentifyReviewForm.tsx:525` | `file?.cover_page == null` | `!isPageBasedFileType(file?.file_type)` |
| `FileEditDialog.tsx` (multiple lines) | `file.cover_page == null` / `file.cover_page != null` | `!isPageBasedFileType(file.file_type)` / `isPageBasedFileType(file.file_type)` |

Lines in `FileEditDialog.tsx` that use `cover_page` as **data** (value display, page picker current page, comparison for pending changes) should NOT change.

**No changes needed** for places that use `CoverPage` as data:
- `scan_unified.go:2136` — assigns value to new file record
- `scan_unified.go:2855` — merges value in enrichment
- `scan_unified.go:1795` — sidecar restoration (already checks file type)
- `filegen/cbz.go:450`, `filegen/kepub_cbz.go:104`, `kepub/cbz.go:470` — generation
- `downloadcache/fingerprint.go:246` — fingerprint

### 3. PDF Parser: Always Set CoverPage

In `pkg/pdf/pdf.go`, always set `CoverPage = 0` for PDFs regardless of whether cover extraction succeeds. This ensures:
- The frontend page picker always has a valid initial value
- The data is consistent: PDFs are page-based and page 0 is always the cover page

```go
// Before (line 88-98):
var coverPage *int
if cd, cm, err := extractCover(path); err == nil {
    coverData = cd
    coverMime = cm
    page0 := 0
    coverPage = &page0
}

// After:
page0 := 0
coverPage := &page0  // PDF covers always derive from page 0
if cd, cm, err := extractCover(path); err == nil {
    coverData = cd
    coverMime = cm
}
```

Update the comment in the PDF parser to explain this, and add a note to `pkg/pdf/CLAUDE.md`.

### 4. Image Resolution Helper

Add to `pkg/fileutils/operations.go`:

```go
// ImageResolution returns the total pixel count (width * height) of an image
// by reading only the image header (no full decode). Returns 0 if the image
// cannot be decoded.
func ImageResolution(data []byte) int {
    cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
    if err != nil {
        return 0
    }
    return cfg.Width * cfg.Height
}

// ImageFileResolution returns the total pixel count of an image file on disk.
// Returns 0 if the file cannot be read or decoded.
func ImageFileResolution(path string) int {
    f, err := os.Open(path)
    if err != nil {
        return 0
    }
    defer f.Close()
    cfg, _, err := image.DecodeConfig(f)
    if err != nil {
        return 0
    }
    return cfg.Width * cfg.Height
}
```

Uses `image.DecodeConfig` which reads only the header — no full image decode needed.

### 5. Post-Enrichment Cover Upgrade

Add a new function `upgradeEnricherCover` that handles comparing and applying enricher covers. This is called after `runMetadataEnrichers` in both scan paths.

**Logic:**

```
func upgradeEnricherCover(metadata, file, bookPath, ...) {
    // 1. Skip if no enricher cover data
    if metadata.CoverData is empty:
        return

    // 2. Skip if cover field source is NOT a plugin
    if metadata.FieldDataSources["cover"] is not a plugin source:
        return

    // 3. Skip for page-based file types (CBZ, PDF)
    if IsPageBasedFileType(file.FileType):
        return

    // 4. Determine the "current" cover to compare against
    existingCoverPath = find cover file on disk for this file
    if existingCoverPath exists:
        currentResolution = ImageFileResolution(existingCoverPath)
    else:
        // No cover on disk — enricher cover always wins
        currentResolution = 0

    // 5. Resolution gate
    enricherResolution = ImageResolution(metadata.CoverData)
    if enricherResolution == 0:
        // Can't decode enricher cover — skip
        return
    if enricherResolution <= currentResolution:
        log("enricher cover not larger than current, skipping")
        return

    // 6. Save enricher cover (overwrite existing)
    save normalized cover to disk
    update file record (CoverImageFilename, CoverMimeType, CoverSource)
}
```

**Integration points:**

In `scanFileCreateNew` (after line 2222):
```go
metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID, opts.JobLog)

// NEW: Apply enricher cover if it's higher resolution than the current cover
w.upgradeEnricherCover(ctx, metadata, file, bookPath, isRootLevelFile, opts.JobLog)
```

In `scanFileByID` (after line 456):
```go
metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID, opts.JobLog)

// NEW: Apply enricher cover if it's higher resolution than the current cover
w.upgradeEnricherCover(ctx, metadata, file, book.Filepath, false, opts.JobLog)
```

**Plugin field settings are already respected:** The `filterMetadataFields` function (line 2952) zeros out all cover data when the "cover" field is disabled for a plugin. This happens before `upgradeEnricherCover` runs, so there would be no cover data to upgrade with.

### 6. Edge Cases

| Scenario | Behavior |
|----------|----------|
| No current cover (no cover on disk, file has no embedded cover) | Enricher cover accepted (currentResolution = 0) |
| Enricher cover can't be decoded | Skipped (enricherResolution = 0) |
| Current cover can't be decoded | Enricher cover accepted (currentResolution = 0, benefit of doubt) |
| CoverPage is set (CBZ/PDF) | Enricher cover blocked by file type check (step 3) AND by existing line 2770 guard |
| Cover field disabled for plugin | Cover data already zeroed by filterMetadataFields — nothing to upgrade |
| Multiple enrichers provide covers | First-wins merge in runMetadataEnrichers — only the first enricher's cover is considered |
| Enricher provides coverUrl but no coverData | DownloadCoverFromURL populates coverData before the upgrade step |

### 7. Website Documentation

Update `website/docs/plugins/development.md` in the **Cover Images** subsection (around line 414) to document:

1. **Resolution gate**: Enricher covers are only applied during automatic scans if they have a higher total resolution (width x height) than the file's current cover. If the file already has a cover of equal or greater resolution, the enricher cover is skipped.

2. **Page-based formats**: CBZ and PDF files derive covers from their page content. Plugin covers are never applied to these formats, even if the plugin declares the `cover` field.

3. **Field settings**: The `cover` field must be enabled in the plugin's field settings for a library for cover enrichment to take effect.

Also add a note to the metadata enricher section (around line 476) that automatic enrichment respects both the confidence threshold and the cover resolution gate.

### 8. Test Updates

Update `pkg/worker/scan_enricher_test.go`:
- Lines 469, 513: Change `CoverPage != nil` checks to use the file type helper
- Add test cases for the resolution gate behavior
- Add test cases verifying page-based formats block enricher covers regardless of CoverPage

Add tests for `ImageResolution` and `ImageFileResolution` in `pkg/fileutils/`.

## Files Changed

| File | Change |
|------|--------|
| `pkg/models/file.go` | Add `IsPageBasedFileType()` helper |
| `pkg/fileutils/operations.go` | Add `ImageResolution()`, `ImageFileResolution()` |
| `pkg/pdf/pdf.go` | Always set `CoverPage = 0` |
| `pkg/pdf/CLAUDE.md` | Note about CoverPage always being set |
| `pkg/worker/scan_unified.go` | Add `upgradeEnricherCover()`, change CoverPage guard to file type, pass fileType to `runMetadataEnrichers`, integrate upgrade in both scan paths |
| `pkg/worker/scan_enricher_test.go` | Update CoverPage guard tests, add resolution gate tests |
| `pkg/books/handlers.go` | Change cover upload guard to file type |
| `app/components/library/IdentifyReviewForm.tsx` | Change `coverEditable` to file type check |
| `app/components/library/FileEditDialog.tsx` | Change guard-style CoverPage checks to file type |
| `app/libraries/utils.ts` (or similar) | Add `isPageBasedFileType()` frontend helper |
| `website/docs/plugins/development.md` | Document resolution gate, page-based format protection, field settings |
| `pkg/fileutils/operations_test.go` | Tests for resolution helpers |
