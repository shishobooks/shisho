---
name: epub
description: You MUST use this before working on anything EPUB-related (e.g. parsing, generation, metadata, etc.). Covers OPF structure, Dublin Core elements, Calibre meta tags, and the epub/filegen packages.
---

# EPUB Format Reference

This skill documents the EPUB format as used in Shisho for parsing and generation.

## File Structure

EPUB files are ZIP archives with a specific structure:

```
mimetype                  # Must be first, uncompressed: "application/epub+zip"
META-INF/
  container.xml           # Points to the OPF file location
OEBPS/ (or similar)
  content.opf             # Package document with metadata
  toc.ncx                 # Navigation (EPUB2) or nav.xhtml (EPUB3)
  *.xhtml                 # Content files
  *.css                   # Stylesheets
  images/                 # Cover and content images
```

## OPF Package Document

The OPF (Open Packaging Format) file contains metadata and manifest.

### XML Namespaces

```xml
xmlns="http://www.idpf.org/2007/opf"           <!-- OPF namespace -->
xmlns:dc="http://purl.org/dc/elements/1.1/"    <!-- Dublin Core -->
xmlns:opf="http://www.idpf.org/2007/opf"       <!-- OPF attributes -->
```

### Metadata Elements

#### Dublin Core Elements

| Element | Usage |
|---------|-------|
| `<dc:title>` | Book title (multiple allowed; prefers id="title-main" or `title-type="main"` property) |
| `<dc:creator>` | Authors (with `opf:role="aut"` and `opf:file-as` attributes) |
| `<dc:subject>` | **Genres** (one element per genre) |
| `<dc:identifier>` | Unique identifier (ISBN, UUID, etc.) |
| `<dc:language>` | Language code (e.g., "en") |
| `<dc:description>` | Book description |
| `<dc:publisher>` | Publisher name |
| `<dc:date>` | Release date (formats: "2006-01-02", "2006-01-02T15:04:05Z", "2006-01-02T15:04:05-07:00", "2006") |
| `<dc:relation>` | URLs (matched by http:// or https:// prefix) |
| `<dc:source>` | URLs (fallback if `<dc:relation>` not present) |

#### Meta Elements (EPUB2 style - Calibre)

```xml
<meta name="cover" content="cover-image"/>
<meta name="calibre:series" content="Series Name"/>
<meta name="calibre:series_index" content="3"/>
<meta name="calibre:tags" content="Tag1, Tag2"/>  <!-- Tags, comma-separated -->
<meta name="imprint" content="Imprint Name"/>     <!-- Fallback for imprint -->
```

#### Meta Elements (EPUB3 style)

```xml
<meta property="belongs-to-collection" id="series-1">Series Name</meta>
<meta property="collection-type" refines="#series-1">series</meta>
<meta property="group-position" refines="#series-1">3</meta>
<meta property="ibooks:imprint">Imprint Name</meta>  <!-- Preferred imprint source -->
```

## Shisho Implementation

### Parsing (`pkg/epub/opf.go`)

**All Metadata Fields Extracted:**

| Field | Source | Notes |
|-------|--------|-------|
| Title | `<dc:title>` | Prefers element with id="title-main" or `title-type="main"` property |
| Authors | `<dc:creator role="aut">` | All creators with role="aut", or any creator if only one exists |
| Series Name | `<meta name="calibre:series">` | From content attribute |
| Series Number | `<meta name="calibre:series_index">` | Parsed as float (supports decimals like 1.5) |
| Genres | `<dc:subject>` | All subject elements |
| Tags | `<meta name="calibre:tags">` | Comma-separated in content attribute |
| Description | `<dc:description>` | Full text content |
| Publisher | `<dc:publisher>` | Single element |
| Imprint | `<meta property="ibooks:imprint">` | Falls back to `<meta name="imprint">` |
| URL | `<dc:relation>` or `<dc:source>` | First URL starting with http:// or https:// |
| Release Date | `<dc:date>` | Tries 4 date formats in order |
| Cover Image | Via manifest + meta reference | Found by `<meta name="cover" content="ID"/>` |

**Data Source:** All extracted metadata tagged with `models.DataSourceEPUBMetadata` (priority 2)

### Generation (`pkg/filegen/epub.go`)

When generating EPUBs, Shisho writes metadata in **dual format** for maximum compatibility:

| Field | OPF Elements |
|-------|-------------|
| Title | `<dc:title>[0].Text` |
| Subtitle | Second `<dc:title>` with id="subtitle" |
| Authors | `<dc:creator role="aut">` (sorted by SortOrder) |
| Genres | Individual `<dc:subject>` elements (if book has genres) |
| Tags | `<meta name="calibre:tags" content="Tag1, Tag2"/>` |
| Series | **Both** Calibre (`calibre:series`) **AND** EPUB3 (`belongs-to-collection`) formats |
| Publisher | `<dc:publisher>` (from file.Publisher.Name) |
| Release Date | `<dc:date>` (format: "2006-01-02") |
| URL | `<meta name="shisho:url" content="..."/>` |
| Imprint | `<meta name="shisho:imprint" content="..."/>` |
| Description | `<dc:description>` |
| Cover | Replaces image file and updates manifest MIME type |

**Series Dual Format Example:**
```xml
<!-- Calibre format (for Calibre, older readers) -->
<meta name="calibre:series" content="The Stormlight Archive"/>
<meta name="calibre:series_index" content="1"/>

<!-- EPUB3 format (for modern readers, Kobo) -->
<meta property="belongs-to-collection" id="series-1">The Stormlight Archive</meta>
<meta refines="#series-1" property="collection-type">series</meta>
<meta refines="#series-1" property="group-position">1</meta>
```

### Key Functions

```go
// Parse metadata from OPF file
func ParseOPF(r io.Reader) (*OPFPackage, error)

// Extract ParsedMetadata from EPUB file
func Parse(path string) (*mediafile.ParsedMetadata, error)

// Modify OPF content with new metadata
func (g *EPUBGenerator) modifyOPF(pkg *opfPackage, book *models.Book, file *models.File) error

// Generate EPUB with updated metadata (atomic write)
func (g *EPUBGenerator) Generate(ctx, srcPath, destPath string, book *models.Book, file *models.File) error
```

## Container.xml

Located at `META-INF/container.xml`, points to the OPF file:

```xml
<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
```

## Cover Image

Covers are identified by (in order of preference):

1. `<meta name="cover" content="cover-image-id"/>` in metadata
2. `<item id="cover-image-id" href="cover.jpg" media-type="image/jpeg"/>` in manifest
3. Properties attribute: `<item properties="cover-image" .../>`

**Cover Path Resolution:**
- Root-level books: Cover stored in parent directory of file
- Directory-based books: Cover stored in book directory
- File naming: `{filename}.cover.{ext}`

## Scanner Integration

**Metadata Priority System:**
```
Priority 0 (highest): Manual edits
Priority 1: Sidecar (.metadata.json)
Priority 2: EPUB Metadata
Priority 3: Filepath (fallback)
```

**Fallback Title Extraction:**
If EPUB metadata has no title, extracts from filename.

## Edge Cases

**Title Handling:**
- Multiple titles: Prefers one with `title-type="main"` property
- Falls back to filepath if all EPUB titles are empty

**Author Role:**
- Only "aut" role extracted during parsing
- During generation, all authors written with `role="aut"`

**Series Numbers:**
- Supports decimals (1.5) and integers
- Formatted as "1" for whole numbers, "1.5" for decimals

**Genres vs Tags:**
- Genres: `<dc:subject>` (one element per genre)
- Tags: Calibre meta tag (comma-separated)
- Completely separate storage mechanisms

**Preservation:**
- Non-author creators preserved during generation
- Original genres preserved if book has none assigned

## Related Files

- `pkg/epub/opf.go` - OPF parsing and types
- `pkg/epub/epub.go` - EPUB file handling
- `pkg/filegen/epub.go` - EPUB generation
- `pkg/filegen/epub_test.go` - EPUB generation tests
- `pkg/sidecar/types.go` - Sidecar data structures
- `pkg/worker/scan.go` - Scanner integration
- `internal/testgen/epub.go` - Test file generation
