# EPUB Format Reference

This file documents the EPUB format as used in Shisho for parsing and generation.

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
| `<dc:creator>` | Authors. EPUB 2: `opf:role="aut"` and `opf:file-as` attributes. EPUB 3: `<meta refines="#id" property="role">` and `property="file-as"` |
| `<dc:subject>` | **Genres** (one element per genre) |
| `<dc:identifier>` | Unique identifier (ISBN, UUID, etc.). Type from `opf:scheme` (EPUB 2) or `<meta refines="#id" property="identifier-type">` (EPUB 3) |
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
<meta name="imprint" content="Imprint Name"/>     <!-- Overrides dc:publisher when present -->
<meta name="shisho:url" content="https://..."/>   <!-- Written by the generator; overrides dc:relation/dc:source -->
```

#### Meta Elements (EPUB3 style)

```xml
<meta property="belongs-to-collection" id="series-1">Series Name</meta>
<meta property="collection-type" refines="#series-1">series</meta>
<meta property="group-position" refines="#series-1">3</meta>
<meta property="ibooks:imprint">Imprint Name</meta>  <!-- Overrides dc:publisher when present -->
```

## Shisho Implementation

### Parsing (`pkg/epub/opf.go`)

**All Metadata Fields Extracted:**

| Field | Source | Notes |
|-------|--------|-------|
| Title | `<dc:title>` | Prefers element with id="title-main" or `title-type="main"` property |
| Authors | `<dc:creator>` with role "aut" | Role from the `role` attribute (any namespace), else the creator's `role` refinement. Any creator counts if only one exists |
| Series Name | `<meta name="calibre:series">` | From content attribute |
| Series Number | `<meta name="calibre:series_index">` | Parsed as float (supports decimals like 1.5) |
| Genres | `<dc:subject>` | All subject elements |
| Tags | `<meta name="calibre:tags">` | Comma-separated in content attribute |
| Description | `<dc:description>` | Full text content |
| Publisher | `<dc:publisher>` | Overridden by `ibooks:imprint` or `<meta name="imprint">` when present (more specific) |
| URL | `<meta name="shisho:url">`, then `<dc:relation>`, then `<dc:source>` | `shisho:url` is what the generator writes, so it wins. The `dc:` elements are heuristics for files from elsewhere: first value starting with http:// or https:// |
| Release Date | `<dc:date>` | Tries 4 date formats in order |
| Language | `<dc:language>` | BCP 47 tag, normalized via `NormalizeLanguage` (handles ISO 639-2/T like "eng" → "en") |
| Identifiers | `<dc:identifier>` | Type from `scheme` (any namespace), else the `identifier-type` refinement, else detected from the value. Unknown types are skipped |
| Cover Image | Via manifest | `<meta name="cover" content="ID"/>`, then the image item with `properties="cover-image"` |

**Data Source:** All extracted metadata tagged with `models.DataSourceEPUBMetadata` (priority 2)

### Generation (`pkg/filegen/epub.go`)

When generating EPUBs, Shisho writes metadata in **dual format** for maximum compatibility:

| Field | OPF Elements |
|-------|-------------|
| Title | `<dc:title>[0].Text` |
| Subtitle | Second `<dc:title>` with id="subtitle" |
| Authors | `<dc:creator>` with role "aut" and file-as from the person's sort name (sorted by SortOrder), written per version (see OPF attribute rules) |
| Genres | Individual `<dc:subject>` elements (if book has genres) |
| Tags | `<meta name="calibre:tags" content="Tag1, Tag2"/>` |
| Series | **Both** Calibre (`calibre:series`) **AND** EPUB3 (`belongs-to-collection`) formats |
| Publisher | `<dc:publisher>` (from file.Publisher.Name) |
| Release Date | `<dc:date>` (format: "2006-01-02") |
| URL | `<meta name="shisho:url" content="..."/>` |
| Description | `<dc:description>` |
| Language | `<dc:language>` (from file.Language) |
| Cover | Replaces the cover image file and updates its manifest MIME type, or adds one (see cover rules) |

**Round-trip fidelity:** `modifyOPF` unmarshals the whole OPF into `opfPackage`, edits it, and marshals it back, so anything the structs do not carry is silently dropped from every generated EPUB (and the KePub, OPDS, eReader, Kobo, and Share Link downloads built on it). Rules:

- The package, spine, manifest item, itemref, and metadata child structs (title, creator, identifier, meta) each carry an ``Attrs opfAttrs `xml:",any,attr"` `` catch-all. That is what keeps manifest `properties` (`nav`, `cover-image`), itemref `linear` and `properties`, spine `page-progression-direction`, and package `prefix`/`xml:lang`. Add the field to any new OPF struct rather than modeling attributes one at a time.
- `opfAttrs.UnmarshalXMLAttr` drops `xmlns` and `xmlns:*`. encoding/xml hands namespace declarations to `,any,attr` fields but cannot re-emit them: it prints a duplicate `xmlns` attribute or an invented `_xmlns:` prefix. The struct tags declare the namespaces themselves.
- Replacing `dc:identifier` elements keeps the one whose `id` matches `package@unique-identifier` (the publication's identity). Dropping it leaves the package pointing at a missing id, which is invalid. Its value follows the file: a file identifier with the same normalized value is not written twice (`urn:isbn:978...` equals `978...`), and a file identifier of the same kind (ISBN-10 and ISBN-13 count as one kind) replaces a stale value under the unique id. If the file has no identifier of that kind, the source value stays so the package keeps an identity.
- A retitled `dc:title` drops its unmodeled attributes (`xml:lang`, `dir`, `opf:file-as`) because they described the old text. When the file has a language, the package's `xml:lang`, if present, is set to it.
- `pkg/filegen/epub_opf_fidelity_test.go` generates from an EPUB 3 fixture using all of the above. Extend it when adding OPF handling.

**EPUB version:** `isEPUB3` reads `package@version`. `3.x` and later is EPUB 3; a missing or `2.x` version is EPUB 2. The cover and attribute rules below branch on it.

**Cover rules (`findCoverImage`, `addCoverImage`):**

- The cover is the manifest item named by the last `<meta name="cover" content="ID"/>`, else the item whose `properties` tokens include `cover-image`, else an item whose id is `cover`, `cover-image`, or `coverimage` (case-insensitive). This is the parser's order. The writer also requires an `image/*` media type at every step, so a meta pointing at an XHTML cover page is never overwritten with image bytes.
- Hrefs resolve relative to the OPF file (`path.Join` of the OPF directory and the percent-decoded href), so `../Images/cover.jpg` finds its entry.
- When Shisho has a cover and the package has none, it adds the image as a new zip entry (`cover.<ext>` next to the OPF, suffixed if taken) and a manifest item with an unused id based on `cover`. EPUB 3 marks it with `properties="cover-image"`; EPUB 2 adds `<meta name="cover">`. An existing `<meta name="cover">` that pointed at nothing usable is repointed rather than duplicated.

**OPF attribute rules (`writeRefinements`):**

- `opfCreator.Role`/`FileAs` and `opfID.Scheme` parse `role`, `file-as`, and `scheme` in any namespace (encoding/xml matches an unnamespaced attr tag against every namespace). They are parse-only: `writeRefinements` moves them out and clears them before marshal, because as plain fields they would print without the `opf:` prefix. Any new code path that marshals `opfPackage` must call it.
- EPUB 2 writes them as `opf:role`, `opf:file-as`, and `opf:scheme` in the OPF namespace (`opfAttrs.setOPF`), removing any other spelling from the `Attrs` catch-all so nothing prints twice.
- EPUB 3 does not allow them on `dc:` elements. The writer gives the element an id if it lacks one and writes `<meta refines="#id" property="role" scheme="marc:relators">`, `property="file-as"`, or `property="identifier-type"`. An existing refinement with the same value is kept as is (with its own attributes); a role or file-as refinement Shisho changed takes the new value and drops its stale attributes. An existing `identifier-type` refinement always wins, since sources often use another vocabulary (ONIX codes) for the same type.
- A creator's role is its attribute or its `role` refinement. A refinement counts only with no scheme or `scheme="marc:relators"`; one in another vocabulary (ONIX `A01`) is treated as no role, so a same-named book author reuses that element instead of being written twice. Non-author creators and all `dc:contributor` elements are kept. A source author whose name matches a book author keeps its element (id, attributes, other refinements such as `alternate-script`); only role and file-as are set. A dropped author or identifier takes the metas refining its id with it, so no refinement dangles.
- An unchanged `dc:title` keeps its attributes. In EPUB 2 that includes `opf:file-as`; in EPUB 3 an `opf:file-as` on the title moves into a `file-as` refinement (an existing one wins).
- `pkg/filegen/epub_opf_version_test.go` covers both versions and validates the generated OPF with `xmllint --noout` (skipped if not installed) and epubcheck (only if installed).

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

// Apply book/file metadata to the parsed OPF and marshal it
func modifyOPF(pkg *opfPackage, book *models.Book, file *models.File, coverInfo *coverImageInfo, newCoverMimeType string) ([]byte, error)

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

1. `<meta name="cover" content="cover-image-id"/>` in metadata, naming a manifest item (EPUB 2 style, also common in EPUB 3)
2. A manifest image item with `properties="cover-image"` (EPUB 3)

Parsing and generation use the same order. Generation swaps the image either way, and adds a cover marked the version's way when the package has none (see the cover rules under Generation).

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
- Only "aut" role extracted during parsing (from the attribute or an EPUB 3 `role` refinement)
- During generation, all authors are written with role "aut": `opf:role` in EPUB 2, a `role` refinement in EPUB 3

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

## Chapter/Navigation Parsing

EPUB files contain navigation documents that define the table of contents. Shisho extracts chapters from these documents.

### Navigation Document Types

**EPUB 3: Navigation Document** (`nav.xhtml`)
- Uses HTML5 `<nav epub:type="toc">` element
- Supports nested chapters via nested `<ol>` lists
- Preferred source for chapter extraction

**EPUB 2: NCX** (`toc.ncx`)
- Uses `<navMap>` with `<navPoint>` elements
- Supports nesting via child `<navPoint>` elements
- Fallback when EPUB 3 nav not found

### Parsing Strategy

**Priority:**
1. Try EPUB 3 nav document first (manifest item with `properties="nav"`)
2. Fall back to NCX (referenced in spine `toc` attribute)
3. If neither found, no chapters extracted

### Key Functions (`pkg/epub/nav.go`)

```go
// Parse EPUB 3 navigation document
func parseNavDocument(r io.Reader) ([]mediafile.ParsedChapter, error)

// Parse EPUB 2 NCX file
func parseNCX(r io.Reader) ([]mediafile.ParsedChapter, error)

// Find nav document href from manifest
func findNavDocumentHref(manifest []ManifestItem, basePath string) string

// Find NCX href from spine toc attribute
func findNCXHref(pkg *OPFPackage, basePath string) string
```

### Chapter Data Structure

```go
type ParsedChapter struct {
    Title    string
    Href     *string          // Content document href (e.g., "chapter1.xhtml")
    Children []ParsedChapter  // Nested chapters (EPUB supports arbitrary nesting)
}
```

### EPUB 3 Nav Document Structure

```xml
<nav epub:type="toc">
  <ol>
    <li><a href="chapter1.xhtml">Chapter 1</a></li>
    <li>
      <a href="part2.xhtml">Part 2</a>
      <ol>
        <li><a href="chapter2.xhtml">Chapter 2.1</a></li>
        <li><a href="chapter3.xhtml">Chapter 2.2</a></li>
      </ol>
    </li>
  </ol>
</nav>
```

### EPUB 2 NCX Structure

```xml
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
  </navPoint>
  <navPoint id="part2" playOrder="2">
    <navLabel><text>Part 2</text></navLabel>
    <content src="part2.xhtml"/>
    <navPoint id="ch2" playOrder="3">
      <navLabel><text>Chapter 2.1</text></navLabel>
      <content src="chapter2.xhtml"/>
    </navPoint>
  </navPoint>
</navMap>
```

### Integration

- Chapters extracted during `Parse()` and included in `ParsedMetadata.Chapters`
- Worker syncs chapters to database via `chapterService.ReplaceChapters()`
- Nested structure preserved in database via `parent_id` foreign key
- API: `GET /books/files/:id/chapters` returns nested chapter tree
- API: `PUT /books/files/:id/chapters` allows manual chapter editing

## Related Files

- `pkg/epub/opf.go` - OPF parsing and types
- `pkg/epub/nav.go` - Navigation/chapter parsing
- `pkg/epub/nav_test.go` - Navigation parsing tests
- `pkg/epub/epub.go` - EPUB file handling
- `pkg/filegen/epub.go` - EPUB generation
- `pkg/filegen/epub_test.go` - EPUB generation tests
- `pkg/filegen/epub_opf_fidelity_test.go` - OPF round-trip fidelity tests (EPUB 3 attributes, unique identifier)
- `pkg/filegen/epub_opf_version_test.go` - Version-dependent generation tests (cover lookup and adding, `opf:` attributes vs refinements)
- `pkg/sidecar/types.go` - Sidecar data structures
- `pkg/worker/scan.go` - Scanner integration
- `internal/testgen/epub.go` - Test file generation
