# File Identifiers Design

## Overview

Add support for file identifiers (ISBN, ASIN, etc.) to track unique identifiers for ebook and audiobook files. Identifiers are tied to files (not books) because each file represents a different edition with potentially different identifiers.

## Identifier Types

```go
const (
    IdentifierTypeISBN10    = "isbn_10"
    IdentifierTypeISBN13    = "isbn_13"
    IdentifierTypeASIN      = "asin"
    IdentifierTypeUUID      = "uuid"
    IdentifierTypeGoodreads = "goodreads"
    IdentifierTypeGoogle    = "google"
    IdentifierTypeOther     = "other"
)
```

## Schema

Modify existing migration (`pkg/migrations/20250321211048_create_initial_tables.go`):

```sql
CREATE TABLE file_identifiers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,
    source TEXT NOT NULL
);
CREATE INDEX ix_file_identifiers_file_id ON file_identifiers (file_id);
CREATE INDEX ix_file_identifiers_value ON file_identifiers (value);
CREATE UNIQUE INDEX ux_file_identifiers_file_type ON file_identifiers (file_id, type);
```

Key points:
- Rename table from `book_identifiers` to `file_identifiers`
- `book_id` → `file_id` with CASCADE delete
- Add `source` column for data source tracking
- Add unique constraint on `(file_id, type)` - one identifier of each type per file
- Add index on `value` for search functionality

## Parsing

### Detection Logic

Shared detection logic for all identifier sources:

1. **Check explicit scheme first** (for EPUB `dc:identifier`):
   - `scheme="ISBN"` → run ISBN detection on value
   - `scheme="ASIN"` → ASIN
   - `scheme="GOODREADS"` → Goodreads
   - `scheme="GOOGLE"` → Google
   - Unknown scheme → skip

2. **Pattern matching on value**:
   - Matches ISBN-10 format (10 digits/chars, valid checksum) → ISBN-10
   - Matches ISBN-13 format (13 digits, valid checksum) → ISBN-13
   - Matches UUID format → UUID
   - Matches ASIN format (10 alphanumeric, starts with B0) → ASIN

3. **Fallback behavior**:
   - For EPUB identifiers → **skip** (only store known types)
   - For CBZ GTIN → **store as Other**

### Source Locations

| Format | Source | Identifiers | Detection |
|--------|--------|-------------|-----------|
| EPUB | `dc:identifier` elements | ISBN-10, ISBN-13, ASIN, UUID, Goodreads, Google | `scheme` attribute or pattern matching |
| M4B | Freeform atoms | ASIN | Key `com.apple.iTunes:ASIN` |
| CBZ | ComicInfo.xml | ISBN-10, ISBN-13, ASIN, UUID, Other | `<GTIN>` element with pattern matching |

## File Generation

When writing metadata back to files, identifiers are mapped to format-appropriate elements:

| Identifier Type | EPUB | CBZ | M4B |
|-----------------|------|-----|-----|
| ISBN-10 | `<dc:identifier scheme="ISBN">` | `<GTIN>` | — |
| ISBN-13 | `<dc:identifier scheme="ISBN">` | `<GTIN>` | — |
| ASIN | `<dc:identifier scheme="ASIN">` | `<GTIN>` | `----:com.apple.iTunes:ASIN` |
| UUID | `<dc:identifier scheme="UUID">` | — | — |
| Goodreads | `<dc:identifier scheme="GOODREADS">` | — | — |
| Google | `<dc:identifier scheme="GOOGLE">` | — | — |
| Other | — | `<GTIN>` | — |

**CBZ generation priority**: When multiple identifiers could go to `<GTIN>`, use: ISBN-13 > ISBN-10 > Other > ASIN

## Search Integration

Unified search includes identifier matching:
- User can paste an ISBN/ASIN into the search box
- Search query JOINs on `file_identifiers.value` column
- Exact match on identifier value finds the book

## Implementation Checklist

Following the Metadata Sync Checklist from backend skill:

| # | File | Changes |
|---|------|---------|
| 1 | `pkg/migrations/20250321211048_create_initial_tables.go` | Rename table, change book_id to file_id, add source column, add indexes |
| 2 | `pkg/models/book-identifier.go` → `file-identifier.go` | Rename to `FileIdentifier`, add type constants, add `Source` field |
| 3 | `pkg/models/file.go` | Add `Identifiers []*FileIdentifier` relation |
| 4 | `pkg/sidecar/types.go` | Add `Identifiers []IdentifierMetadata` to `FileSidecar` |
| 5 | `pkg/sidecar/sidecar.go` | Update `FileSidecarFromModel()` to include identifiers |
| 6 | `pkg/downloadcache/fingerprint.go` | Add identifiers to `Fingerprint` struct and `ComputeFingerprint()` |
| 7 | `pkg/mediafile/mediafile.go` | Add `ParsedIdentifier` struct and `Identifiers` field to `ParsedMetadata` |
| 8 | `pkg/epub/opf.go` | Parse `dc:identifier` elements with detection logic |
| 9 | `pkg/cbz/cbz.go` | Parse `<GTIN>` element with detection logic |
| 10 | `pkg/mp4/metadata.go` | Surface `com.apple.iTunes:ASIN` from freeform atoms |
| 11 | `pkg/filegen/epub.go` | Write `dc:identifier` elements with scheme |
| 12 | `pkg/filegen/cbz.go` | Write `<GTIN>` element |
| 13 | `pkg/filegen/m4b.go` | Write `com.apple.iTunes:ASIN` freeform atom |
| 14 | `pkg/worker/scan.go` | Save identifiers to database with source priority |
| 15 | `pkg/books/service.go` | Add `.Relation("Files.Identifiers")` to query methods |
| 16 | Search handler | JOIN on `file_identifiers.value` for unified search |
| 17 | Frontend components | Display/edit identifiers in file details |

## Data Source Priority

Identifiers follow the same source priority system as other metadata:
- 0: Manual (highest)
- 1: Sidecar
- 2: EPUB/CBZ Metadata
- 3: M4B Metadata

User-provided identifiers are preserved unless explicitly changed.

## UI Requirements

### Display
- Show identifiers in file details view
- Format type nicely (e.g., "ISBN-13", "ASIN")
- Make values copyable
- Show source indicator

### Edit
- Add new identifier (select type from dropdown, enter value)
- Edit existing identifier value
- Delete identifier

### Validation
- ISBN-10/13 checksum validation
- ASIN format (10 chars, alphanumeric)
- UUID format
