# Reset to File Metadata — Full Wipe Behavior

**Date:** 2026-04-12
**Status:** Approved
**Builds on:** [Unified Rescan Dialog](2026-03-29-unified-rescan-dialog-design.md)

## Problem

The "Reset to file metadata" rescan mode (`ForceRefresh=true, SkipPlugins=true`) does not clear DB fields when the source file lacks those values. `shouldUpdateScalar` and `shouldUpdateRelationship` both short-circuit on empty new values, so plugin-added or manually-set metadata persists even after a reset. Users expect reset to be a complete wipe — "what you'd get from a fresh scan of this file with plugins off."

## Solution

**Approach: Clear-then-rescan.** Before running `scanFileCore`, wipe all metadata columns and relationships on the target book/file (preserving identity and intrinsic properties), then let the normal scan flow repopulate from file-embedded metadata with filepath fallbacks. The scanner doesn't need per-field reset logic — it runs against a blank slate.

## Detailed Design

### New ScanOptions field

```go
type ScanOptions struct {
    // ... existing fields ...
    Reset bool // Wipe all metadata before scanning (used by "reset" mode)
}
```

Mode resolution: `"reset"` maps to `ForceRefresh=true, SkipPlugins=true, Reset=true`.

### Filepath fallback helper

Extract the logic from `scanFileCreateNew` (lines ~2100-2217) that derives title/authors/narrators/series from filepath into a reusable helper:

```go
func applyFilepathFallbacks(
    metadata *mediafile.ParsedMetadata,
    filePath, bookPath, fileType string,
    isRootLevelFile bool,
)
```

Populates empty fields on `metadata`:
- `Title` via `deriveInitialTitle(filePath, isRootLevelFile, metadata)`
- `Authors` via `extractAuthorsFromFilepath(bookPath, isRootLevelFile)`
- `Narrators` via `extractNarratorsFromFilepath(filePath, bookPath, isRootLevelFile)`
- `Series` + `SeriesNumber` via `ExtractSeriesFromTitle(title, fileType)` when no metadata series exists

Sources set to `DataSourceFilepath`. Called by both `scanFileCreateNew` (refactor, no behavior change) and the reset path in `scanFileByID`.

### Reset helper: `resetBookFileState`

Called from `scanFileByID` (and indirectly from `scanBook`) when `Reset=true`, before `scanFileCore`.

#### Book columns cleared (set to zero/NULL + clear `*_source` columns)

| Field | Reset to |
|-------|----------|
| Title, TitleSource | filepath-derived title (NOT NULL) |
| SortTitle, SortTitleSource | regenerated from filepath-derived title |
| Subtitle, SubtitleSource | NULL |
| Description, DescriptionSource | NULL |
| AuthorSource | `DataSourceFilepath` |
| GenreSource | NULL |
| TagSource | NULL |

#### Book relations deleted

| Relation | Table |
|----------|-------|
| Authors | `authors` WHERE book_id |
| BookSeries | `book_series` WHERE book_id |
| BookGenres | `book_genres` WHERE book_id |
| BookTags | `book_tags` WHERE book_id |

#### File columns cleared (set to NULL + clear `*_source` columns)

| Field | Reset to |
|-------|----------|
| Name, NameSource | NULL |
| URL, URLSource | NULL |
| ReleaseDate, ReleaseDateSource | NULL |
| PublisherID, PublisherSource | NULL |
| ImprintID, ImprintSource | NULL |
| Language, LanguageSource | NULL |
| Abridged, AbridgedSource | NULL |
| ChapterSource | NULL |
| CoverImageFilename, CoverMimeType, CoverSource, CoverPage | NULL + delete on-disk cover file |

#### File columns preserved (intrinsic/identity)

ID, CreatedAt, UpdatedAt, LibraryID, BookID, Filepath, FileType, FileRole, FilesizeBytes, FileModifiedAt, PageCount, AudiobookDurationSeconds, AudiobookBitrateBps, AudiobookCodec

#### File relations deleted

| Relation | Table |
|----------|-------|
| Narrators | `narrators` WHERE file_id |
| Identifiers | `file_identifiers` WHERE file_id |
| Chapters | `chapters` WHERE file_id |

#### FTS

Book FTS entry re-indexed after scan completes (already handled by `scanFileCore` downstream).

### Scan flow when Reset=true

```
scanFileByID (or scanBook per file):
  parseFileMetadata(path)           — parse file, no plugins
  applyFilepathFallbacks(metadata)  — fill gaps from filepath
  resetBookFileState(book, file)    — wipe DB + delete on-disk cover
  extractAndSaveCover(...)          — re-extract cover from file
  scanFileCore(forceRefresh=true)   — repopulate from metadata
```

### Multi-file books (scanBook)

When `scanBook` is called with `Reset=true`, the book-level wipe runs **once** before iterating files. Each file's file-level wipe runs per-file. This prevents sibling files from clobbering each other's contributions during the per-file scan.

### Frontend changes

Update the reset mode description in `RescanDialog.tsx`:

**Before:**
> Skip plugins and use only metadata embedded in the source file(s). Use when plugin enrichment is matching incorrectly.

**After:**
> Clear all metadata and re-scan as if this were a brand new file, without plugins. Manual edits and enricher data will be removed.

### Documentation changes

Update `website/docs/metadata.md` rescan section to explain that reset clears all fields not present in the file, including manual edits.

## What's NOT Changing

- `shouldUpdateScalar` / `shouldUpdateRelationship` — no changes to these helpers.
- `scan` and `refresh` modes — behavior unchanged.
- Batch library scans — unaffected (they don't use resync endpoints).
- Sidecar files on disk — not deleted (they're the user's external data). Sidecars are already skipped during `forceRefresh=true` scans.
- `IdentifierSource` on files — cleared along with identifiers relation.

## Tests

- Reset wipes subtitle, description, genres, tags, publisher, imprint, language, abridged, release_date when file has none.
- Reset clears manual edits (source=manual survives only until reset).
- Reset falls back to filepath-derived title when file has no embedded title.
- Reset falls back to filepath-derived authors when file has no embedded authors.
- Reset preserves filepath, library_id, file_role, book_id, primary_file_id, intrinsic file props.
- Reset re-extracts cover from file (deletes prior cover first).
- Multi-file book reset: book-level wipe runs once, both files contribute metadata.
- `applyFilepathFallbacks` refactor: `scanFileCreateNew` behavior unchanged (regression guard).
