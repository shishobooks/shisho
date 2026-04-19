# Plugin-Settable Cover Page

## Summary

Allow plugins to set `coverPage` for page-based files (CBZ, PDF). Currently plugins can return `coverData`/`coverUrl` for non-page-based files, but for CBZ/PDF these are silently ignored because covers come from page content, not arbitrary images. Plugins — particularly metadata enrichers — often know that a comic's cover isn't the first page; this change lets them tell Shisho which page to use. `coverPage` falls under the existing `cover` capability; no new capability is added.

## Background

- `ParsedMetadata.CoverPage *int` already exists in Go (`pkg/mediafile/mediafile.go:50`).
- `coverPage?: number` already exists in the plugin SDK (`packages/plugin-sdk/metadata.d.ts:66`).
- The `cover` manifest field is already documented as controlling `coverData`, `coverMimeType`, `coverPage`, and `coverUrl` (`pkg/plugins/CLAUDE.md`).
- `models.File.CoverPage *int` already exists with a column in the schema.
- `IsPageBasedFileType` returns true only for CBZ and PDF (`pkg/models/file.go:86`).
- The manual UI handler `updateFileCoverPage` (`pkg/books/handlers_cover_page.go`) is the reference implementation for applying a cover page: bounds-check, extract the page via the appropriate page cache, write the rendered image as the cover file, update file fields, write sidecar.

What's missing is wiring: parsing `coverPage` out of JS results, applying it during `persistMetadata`, and sharing the page-extraction logic between the UI handler and the plugin path.

## Design Decisions

1. **Scope: all three plugin paths** — `fileParser`, `metadataEnricher`, and the manual identify apply all support `coverPage`. Keeps the three parsers symmetric. In practice `metadataEnricher` is the primary use case; `fileParser` is limited (reserved extensions can't be claimed) but including it costs nothing.
2. **Precedence: file-type-gated (strict)** — For CBZ/PDF, `coverPage` is used and `coverData`/`coverUrl` are ignored. For EPUB/M4B, `coverData`/`coverUrl` is used and `coverPage` is ignored. This matches the existing invariant in `persistMetadata` that page-based formats never accept external cover images. No fallback between the two branches.
3. **Invalid `coverPage` handling: skip with warning** — Out-of-range, negative, or `PageCount`-unknown cases log a warning and leave the existing cover alone. No clamping, no fallback to `coverData`. A plugin returning page 5 for a 3-page file is a bug worth surfacing.
4. **Cover extraction mirrors the manual handler** — When `coverPage` is applied, extract the page via the appropriate page cache and write it as the cover image file, just like the UI flow. Otherwise the cover wouldn't actually appear.
5. **`CoverSource` is `plugin:scope/id`** — Follows existing data source convention. The manual UI handler continues to write `manual`.

## Changes

### Shared helper

New exported function, implemented in the `books` package (which already owns the caches and the existing logic):

```go
// ExtractCoverPageToFile renders `page` from `file`, writes it as the cover
// image alongside the book, and returns (filename, mimeType). Deletes any
// existing cover image with the same base name first.
func ExtractCoverPageToFile(
    file *models.File,
    bookFilepath string,
    page int,
    cbzCache *cbzpages.Cache,
    pdfCache *pdfpages.Cache,
) (filename, mimeType string, err error)
```

The body is the existing `handlers_cover_page.go:65-107` logic, unchanged behaviorally. `updateFileCoverPage` becomes a thin wrapper: bounds-check, call the helper, update fields, write sidecar.

### JS → Go parsing

Each parser gets a small `coverPage` block next to existing cover fields. Only populate `md.CoverPage` when the value is a finite non-negative integer; ignore `undefined`, `null`, `NaN`, and negative values at parse time. The real bounds check happens at apply time when `PageCount` is known.

- `pkg/plugins/hooks.go` `parseParsedMetadata` (fileParser) — near line 495.
- `pkg/plugins/hooks.go` `parseSearchResponse` (metadataEnricher) — near line 398.
- `pkg/plugins/handler.go` `convertFieldsToMetadata` (manual identify apply) — near line 1953, key is `cover_page`, value is `float64`.

### Apply (`persistMetadata`)

Replace the current block at `pkg/plugins/handler.go:1870-1896`. Instead of unconditionally skipping cover data when `targetFile.CoverPage != nil`, branch on file type:

```
if targetFile is CBZ/PDF:
    if md.CoverPage != nil:
        if invalid (negative, >= PageCount, PageCount unknown):
            log warning, skip
        else:
            call pageExtractor.ExtractCoverPage(file, book.Filepath, *md.CoverPage)
            set CoverPage, CoverImageFilename, CoverMimeType, CoverSource (plugin:scope/id)
            append the four columns to fileColumns
    // coverData / coverUrl are ignored for page-based files (unchanged)
else:
    // existing coverData write path, unchanged
```

### Dependency injection

Add a new interface to `enrichDeps` (`pkg/plugins/handler.go:39`):

```go
type pageExtractor interface {
    ExtractCoverPage(file *models.File, bookFilepath string, page int) (filename, mimeType string, err error)
}
```

Implementation lives in the `books` package as a small wrapper that captures the two caches and delegates to `ExtractCoverPageToFile`. Wired in `pkg/plugins/routes.go` at both construction sites (lines 28 and 70).

## Testing

All new tests use `t.Parallel()`.

**Parser unit tests** (`pkg/plugins/hooks_test.go`): one new case per parser asserting `coverPage: 3` produces `md.CoverPage == &3`, and one case per parser asserting invalid values (negative, `NaN`, missing) produce `nil`.

**Persist integration tests** (new `pkg/worker/scan_cover_page_test.go` or extension of `scan_enricher_test.go`):

- CBZ with 5 pages, enricher returns `coverPage: 2` → `file.CoverPage == 2`, cover file exists on disk, `CoverSource == "plugin:<scope>/<id>"`.
- PDF equivalent.
- Out-of-bounds (`coverPage: 99`) → `CoverPage` unchanged, warning logged, no cover file written.
- `coverPage` for an EPUB → silently ignored, existing cover untouched.
- CBZ with both `coverPage` and `coverData` → `coverPage` wins, no `coverData` write.
- EPUB with both → `coverData` wins, `coverPage` ignored.

**Manual handler regression** (`pkg/books/handlers_cover_page_test.go`): existing tests continue to pass against the refactored helper — `CoverSource` remains `manual`, cover file is still extracted.

## Docs

- **`website/docs/plugins/development.md`**: under the `cover` field group, document that `coverPage` only applies to CBZ/PDF files, takes precedence over `coverData`/`coverUrl` for those formats, is silently ignored otherwise, and out-of-range values are skipped with a warning.
- **`pkg/plugins/CLAUDE.md`**: one-line note under the fileParser metadata example clarifying CBZ/PDF-only behavior for `coverPage`.
- No versioned docs updates (they're snapshots).
- No `configuration.md`, sidecar, or `metadata.md` updates — no new config, no schema change, no new user-visible metadata field.

## Non-goals

- No new plugin capability (still under `cover`).
- No schema migration (`files.cover_page` already exists).
- No frontend changes (the existing `FileEditDialog` already handles files with `cover_page` set).
- No changes to how covers are served for page-based files.
- No new SDK changes (`coverPage` already in `metadata.d.ts`).
