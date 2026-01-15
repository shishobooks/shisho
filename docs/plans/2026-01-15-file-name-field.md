# File Name Field

## Problem

When "organize file structure" is enabled, filenames become long and noisy (e.g., `Author Name/Series Name/Book Title.epub`), making it hard to identify files at a glance. Additionally, when a book has multiple editions of the same file type, there's no way to differentiate them beyond the full filepath.

## Solution

Add a `name` field to files that:
- Displays prominently in the UI as the primary identifier
- Defaults to the title extracted from file metadata
- Can be manually overridden by users
- Shows the filename in muted text underneath

## Database Changes

Modify the files table in `pkg/migrations/20250321211048_create_initial_tables.go`:

```sql
name TEXT,
name_source TEXT
```

## Model Changes

### `pkg/models/file.go`

Add fields to the `File` struct:

```go
Name       *string `json:"name"`
NameSource *string `json:"name_source" tstype:"DataSource"`
```

## Metadata Sync Checklist

### 1. Sidecar Types (`pkg/sidecar/types.go`)

Add to `FileSidecar`:

```go
Name *string `json:"name,omitempty"`
```

Note: `name_source` is NOT stored in sidecar per backend conventions.

### 2. Sidecar Conversion (`pkg/sidecar/sidecar.go`)

Update `FileSidecarFromModel()` to include `Name`.

### 3. Download Fingerprint (`pkg/downloadcache/fingerprint.go`)

Add `Name` to `Fingerprint` struct and `ComputeFingerprint()` so cache invalidates when name changes.

### 4. File Parsers

The parsers already extract title - map it to the `Name` field in `ParsedMetadata`:

- EPUB (`pkg/epub/opf.go`): Uses `<dc:title>`
- CBZ (`pkg/cbz/cbz.go`): Uses `<Series>` + `<Number>` or `<Title>`
- M4B (`pkg/mp4/metadata.go`): Uses title atom

### 5. File Generators

Write the name back as title when generating files:

- EPUB (`pkg/filegen/epub.go`)
- CBZ (`pkg/filegen/cbz.go`)
- M4B (`pkg/filegen/m4b.go`)
- KePub (`pkg/kepub/cbz.go`)

### 6. Scanner (`pkg/worker/scan.go`)

Populate `Name` and `NameSource` from parsed metadata during scanning.

### 7. ParsedMetadata (`pkg/mediafile/mediafile.go`)

Already has `Title` field - reuse for name. No changes needed.

### 8-9. API Relations / File Retrieval

No new relations needed - `Name` is a simple field on the File model.

### 10. UI Display

Update `app/components/pages/BookDetail.tsx` and the design doc.

## UI Design Updates

Update `docs/plans/2026-01-15-file-ui-design.md` to reflect the new layout:

### Primary Row (updated)

```
▶ [EPUB]  The Great Book                                512 KB    ⬇  ✏️
          The Great Book.epub
```

**Elements (left to right):**
1. Disclosure chevron (unchanged)
2. File type badge (unchanged)
3. **Name** - Displayed prominently, font-medium, truncates if needed
4. File-specific stats (unchanged)
5. File size (unchanged)
6. Action buttons (unchanged)

**Below the primary row:**
- **Filename** - Extracted from filepath, displayed in `text-xs text-muted-foreground`
- Add `truncate` class to handle long paths
- Add `title` attribute with full filepath so users can see complete value on hover

### Supplements Section (updated)

Same pattern - name prominently displayed, filename underneath in muted text.

## File Edit Modal

The existing file edit modal should include a "Name" field that:
- Shows the current name (or placeholder if null)
- Allows users to override the auto-derived name
- When saved, sets `name_source` to "manual"

## Default Behavior

When `name` is NULL:
- UI falls back to displaying the filename (extracted from filepath)
- This maintains backwards compatibility for existing files before migration

When scanning:
- If file has no existing name OR existing name came from a lower-priority source
- Set name from parsed metadata title
- Set name_source appropriately (epub_metadata, m4b_metadata, cbz_metadata, filepath)
