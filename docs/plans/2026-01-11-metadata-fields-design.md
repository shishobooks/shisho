# Metadata Fields Design

Add new metadata columns for books and files.

## Data Model

### Books Table Changes
- `description TEXT` (nullable)
- `description_source TEXT`

### Files Table Changes
- `url TEXT` (nullable)
- `url_source TEXT`
- `release_date DATE` (nullable, stored as `time.Time` in Go)
- `release_date_source TEXT`
- `publisher_id INTEGER` (nullable, FK to publishers)
- `publisher_source TEXT`
- `imprint_id INTEGER` (nullable, FK to imprints)
- `imprint_source TEXT`

### New Tables

**publishers:**
- `id INTEGER PRIMARY KEY`
- `created_at TIMESTAMPTZ`
- `updated_at TIMESTAMPTZ`
- `library_id INTEGER` (FK to libraries)
- `name TEXT`
- Case-insensitive unique constraint per library

**imprints:**
- `id INTEGER PRIMARY KEY`
- `created_at TIMESTAMPTZ`
- `updated_at TIMESTAMPTZ`
- `library_id INTEGER` (FK to libraries)
- `name TEXT`
- Case-insensitive unique constraint per library

### Relationships
- File → Publisher: one-to-many (a file has one publisher, publisher has many files)
- File → Imprint: one-to-many (a file has one imprint, imprint has many files)

## File Parser Extraction

### EPUB (OPF metadata)

| Field | Read From | Write To |
|-------|-----------|----------|
| Description | `<dc:description>` | `<dc:description>` |
| Publisher | `<dc:publisher>` | `<dc:publisher>` |
| ReleaseDate | `<dc:date>` | `<dc:date>` |
| Imprint | `<meta property="ibooks:imprint">` or `<meta name="imprint">` | Both meta formats |
| URL | `<dc:relation>` or `<dc:source>` (URL-like) | `<dc:relation>` and `<dc:source>` |

### CBZ (ComicInfo.xml)

| Field | Read From | Write To |
|-------|-----------|----------|
| Description | `<Summary>` | `<Summary>` |
| Publisher | `<Publisher>` | `<Publisher>` |
| Imprint | `<Imprint>` | `<Imprint>` |
| ReleaseDate | `<Year>/<Month>/<Day>` | `<Year>`, `<Month>`, `<Day>` |
| URL | `<Web>` | `<Web>` |

### M4B (MP4 atoms)

| Field | Read From | Write To |
|-------|-----------|----------|
| Description | `desc` atom | `desc` atom |
| Publisher | `©pub` atom (`0xA9, 'p', 'u', 'b'`) | `©pub` atom |
| Imprint | `com.shisho:imprint` freeform | `com.shisho:imprint` freeform |
| ReleaseDate | `©day` or `rldt` atoms | `©day` and `rldt` atoms |
| URL | `com.shisho:url` freeform | `com.shisho:url` freeform |

## Implementation Checklist

### Database & Models
1. `pkg/migrations/20250321211048_create_initial_tables.go` - Add columns and new tables
2. `pkg/models/book.go` - Add `Description`, `DescriptionSource` fields
3. `pkg/models/file.go` - Add URL, ReleaseDate, Publisher, Imprint fields
4. `pkg/models/publisher.go` (new) - `Publisher` struct
5. `pkg/models/imprint.go` (new) - `Imprint` struct

### Sidecar Persistence
6. `pkg/sidecar/types.go` - Add fields to `BookSidecar` and file sidecar

### Download Cache
7. `pkg/downloadcache/fingerprint.go` - Add fields to `Fingerprint` struct

### File Parsers (extract metadata)
8. `pkg/epub/opf.go` - Extract description, publisher, release date, imprint, URL
9. `pkg/cbz/cbz.go` - Extract summary, publisher, imprint, release date, URL
10. `pkg/mp4/metadata.go` - Add publisher field
11. `pkg/mp4/atoms.go` - Add `AtomPublisher` and `AtomReleaseDate` constants
12. `pkg/mp4/reader.go` - Parse `©pub` and `rldt` atoms

### File Generators (write metadata back)
13. `pkg/filegen/epub.go` - Write all fields
14. `pkg/filegen/cbz.go` - Write all fields
15. `pkg/filegen/m4b.go` - Write all fields
16. `pkg/kepub/cbz.go` - Write fields for CBZ-to-KePub conversion

### Scanner
17. `pkg/worker/scan.go` - Handle new fields during scanning
18. `pkg/mediafile/mediafile.go` - Add fields to `ParsedMetadata`
