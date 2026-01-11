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

- `<dc:title>` - Book title
- `<dc:creator>` - Authors (with `opf:role="aut"` and `opf:file-as` attributes)
- `<dc:subject>` - **Genres** (one element per genre)
- `<dc:identifier>` - Unique identifier (ISBN, UUID, etc.)
- `<dc:language>` - Language code (e.g., "en")
- `<dc:description>` - Book description

#### Meta Elements (EPUB2 style)

```xml
<meta name="cover" content="cover-image"/>
<meta name="calibre:series" content="Series Name"/>
<meta name="calibre:series_index" content="3"/>
<meta name="calibre:tags" content="Tag1, Tag2"/>  <!-- Tags, comma-separated -->
```

#### Meta Elements (EPUB3 style)

```xml
<meta property="belongs-to-collection" id="series-1">Series Name</meta>
<meta property="collection-type" refines="#series-1">series</meta>
<meta property="group-position" refines="#series-1">3</meta>
```

## Shisho Implementation

### Parsing (`pkg/epub/opf.go`)

Genres are extracted from:

1. `<dc:subject>` elements (primary source)

Tags are extracted from:

1. `<meta name="calibre:tags">` (comma-separated)
2. `<meta name="calibre:user_categories">` (if present)

### Generation (`pkg/filegen/epub.go`)

When generating EPUBs, Shisho writes:

- Genres as individual `<dc:subject>` elements
- Tags as `<meta name="calibre:tags" content="Tag1, Tag2"/>`

### Key Functions

```go
// Parse metadata from OPF
func ParseOPF(r io.Reader) (*OPFPackage, error)

// Extract ParsedMetadata from EPUB
func Parse(path string) (*mediafile.ParsedMetadata, error)

// Modify OPF content with new metadata
func (g *EPUBGenerator) modifyOPF(pkg *opfPackage, book *models.Book, file *models.File) error
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

Covers are identified by:

1. `<meta name="cover" content="cover-image-id"/>` in metadata
2. `<item id="cover-image-id" href="cover.jpg" media-type="image/jpeg"/>` in manifest
3. Properties attribute: `<item properties="cover-image" .../>`

## Related Files

- `pkg/epub/opf.go` - OPF parsing and types
- `pkg/epub/epub.go` - EPUB file handling
- `pkg/filegen/epub.go` - EPUB generation
- `pkg/filegen/epub_test.go` - EPUB generation tests
