---
name: kepub
description: Use when working on KePub conversion for Kobo devices. Covers EPUB-to-KePub transformation, CBZ-to-KePub fixed-layout conversion, koboSpan wrapping, and the kepub package.
---

# KePub Format Reference

This skill documents the KePub (Kobo EPUB) format as used in Shisho for conversion and generation.

## Overview

KePub is Kobo's enhanced EPUB format with:

- Span wrapping for reading statistics and progress tracking
- Kobo-specific XHTML structure (wrapper divs)
- kobo.js injection for pagination
- Fixed-layout support for comics (CBZ conversion)
- Enhanced metadata for Kobo devices

## File Structure

KePub files are EPUBs with `.kepub.epub` extension:

```
mimetype                     # Uncompressed, "application/epub+zip"
META-INF/
  container.xml
kobo.js                      # Injected JavaScript for Kobo features
OEBPS/
  content.opf                # Enhanced OPF with Kobo extensions
  toc.ncx
  nav.xhtml                  # EPUB3 navigation (CBZ conversion)
  styles.css                 # Minimal CSS (CBZ conversion)
  *.xhtml                    # Content with koboSpan elements
```

## EPUB to KePub Conversion (`pkg/kepub/converter.go`)

### Conversion Process

1. **Identify Content Files:**
   - Parse OPF manifest using string-based parsing (avoids namespace issues)
   - Filter for `application/xhtml+xml` and `text/html` media types
   - Exclude NCX files (navigation, not content)

2. **Inject kobo.js:**
   - Added to EPUB root if not present
   - Provides pagination, progress tracking, page navigation
   - Functions: `paginate()`, `goBack()`, `goForward()`, `goPage()`, `goProgress()`

3. **Transform Content Files:**
   - Each file gets new `SpanCounter` (per-file, starting at 1)
   - Compute relative script path for kobo.js reference
   - Apply `TransformContentWithOptions()`

4. **Transform OPF:**
   - Ensure cover image has `cover-image` property

5. **Atomic Write:**
   - Write to `.tmp` file, rename on success

### Key Function

```go
func (c *Converter) ConvertEPUB(ctx context.Context, srcPath, destPath string) error
```

## Content Transformation (`pkg/kepub/content.go`)

### Span Wrapping System

**SpanCounter:**
```go
type SpanCounter struct {
    paragraph   int
    sentence    int
    incParaNext bool  // Deferred increment
}

// Generated format: "kobo.{paragraph}.{sentence}"
// Example: kobo.1.1, kobo.1.2, kobo.2.1
```

**Key Behaviors:**
- Per-file paragraph counters start at 1 (not 0)
- Sentence counter resets at paragraph boundaries
- `markParagraphBoundary()`: Deferred increment
- `incrementParagraph()`: Immediate increment (for images)

### Text Segmentation

Splits text on sentence-ending punctuation:
```
"First sentence. Second sentence."
→ ["First sentence.", " ", "Second sentence."]
```

**Rules:**
- Splits on `.!?:` followed by whitespace
- Splits on newlines (`\n`)
- Preserves whitespace as separate segments
- Handles quotes after punctuation
- Non-breaking space (U+00A0) is wrapped

### XHTML Output Structure

```html
<!-- Input -->
<p>Hello world. How are you?</p>

<!-- Output -->
<body>
  <div id="book-columns">
    <div id="book-inner">
      <p>
        <span class="koboSpan" id="kobo.1.1">Hello world.</span>
        <span class="koboSpan" id="kobo.1.2"> </span>
        <span class="koboSpan" id="kobo.1.3">How are you?</span>
      </p>
    </div>
  </div>
</body>
```

### Injected Elements

**Kobo Style Hacks:**
```html
<style id="kobostylehacks" type="text/css">
  div#book-inner { margin-top: 0; margin-bottom: 0; }
</style>
```

**Body Wrapper Structure:**
- `book-columns`: Used by kobo.js for pagination setup
- `book-inner`: Target for CSS-based column layout

**Script Reference:**
```html
<script type="text/javascript" src="../../kobo.js"></script>
```

### Elements NOT Wrapped

- `<script>`, `<style>`, `<pre>`, `<code>`, `<svg>`, `<math>`
- Whitespace-only text nodes
- Empty elements

### Block Elements (Trigger Paragraph Boundary)

`<p>`, `<ol>`, `<ul>`, `<table>`, `<h1>` through `<h6>`

## OPF Transformation (`pkg/kepub/opf.go`)

Ensures cover images have the `cover-image` property required by Kobo.

**Cover ID Detection:**
1. `<meta name="cover" content="ID"/>`
2. `<meta content="ID" name="cover"/>`
3. `<item properties="cover-image" ... id="ID"/>`

**Property Addition:**
- Finds manifest item by ID
- Adds `cover-image` to properties (idempotent)
- Handles both self-closing and regular tag formats

## CBZ to Fixed-Layout KePub (`pkg/kepub/cbz.go`)

### Conversion Process

1. **Image Collection:**
   - Filter: `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`
   - Skip hidden files (starting with `.`)
   - **Natural sorting** (page2 < page10)

2. **Parallel Image Processing:**
   - Worker pool: `min(CPU count, image count)`
   - Context cancellation support

3. **Image Processing (`processImageForKobo`):**
   - **Resize** if larger than Kobo Libra Color (1264×1680)
   - **PNG→JPEG conversion** (85% quality)
   - **Grayscale detection** for manga optimization

4. **Generate EPUB Structure:**
   - Fixed-layout OPF with rendition properties
   - NCX navigation with page navPoints
   - EPUB3 nav.xhtml
   - XHTML pages with KCC-style div+img structure

### CBZMetadata Structure

```go
type CBZMetadata struct {
    Title       string
    Subtitle    *string
    Description *string
    Authors     []CBZAuthor
    Series      []CBZSeries
    Genres      []string
    Tags        []string
    URL         *string
    Publisher   *string
    Imprint     *string
    ReleaseDate *time.Time
}

type CBZAuthor struct {
    Name     string
    SortName string  // e.g., "Doe, Jane"
    Role     string  // writer, penciller, inker, etc.
}

type CBZSeries struct {
    Name   string
    Number *float64
}
```

### Image Processing Details

**Resize Strategy:**
- Only resize if > Kobo screen (1264×1680)
- Maintain aspect ratio with bilinear interpolation
- Uses smaller ratio to fit both dimensions

**Grayscale Detection (`isGrayscaleImage`):**
- Samples every 10th pixel (performance)
- Tolerance: ±10 per RGB channel
- Threshold: <2% colored pixels = grayscale
- Minimum image size: 1000px
- Returns true for native `image.Gray` types

**Palette Quantization (`quantizeToKoboPalette`):**
- 16-level grayscale: `0x00, 0x11, 0x22, ..., 0xff`
- Formula: `level = (value + 8) / 17`
- Optimized for e-ink display

### Fixed-Layout OPF Generation

```xml
<meta property="rendition:layout">pre-paginated</meta>
<meta property="rendition:spread">landscape</meta>
```

**Author Role Mapping:**

| Input Role | EPUB Code | Meaning |
|------------|-----------|---------|
| writer/empty | aut | Author |
| penciller/artist/inker | art | Artist |
| colorist | clr | Colorist |
| letterer | ill | Illustrator |
| cover artist/cover | cov | Cover artist |
| editor | edt | Editor |
| other | aut | Fallback |

**Metadata Elements:**
- `<dc:title>` from title
- `<dc:creator>` with role and file-as
- `<meta property="belongs-to-collection">` for series
- `<dc:subject>` for each genre
- `<meta name="calibre:tags">` for tags (comma-separated)
- `<meta name="shisho:url">` and `<meta name="shisho:imprint">` for custom fields

### XHTML Page Structure (KCC-style)

```html
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
  <title>Page N</title>
  <link href="styles.css" rel="stylesheet"/>
  <meta name="viewport" content="width=800, height=1200"/>
</head>
<body style="">
  <div style="text-align:center;top:0%;">
    <img width="800" height="1200" src="images/page0001.jpg"/>
  </div>
</body>
</html>
```

- Viewport matches image dimensions
- Alternating page-spread properties (left/right)

### Generated Files

```
mimetype (uncompressed)
META-INF/container.xml
OEBPS/
  content.opf
  toc.ncx
  nav.xhtml
  styles.css
  page0001.xhtml, page0002.xhtml, ...
  images/
    page0001.jpg, page0002.jpg, ...
```

## XHTML Processing (`pkg/kepub/xhtml.go`)

### XML Declaration Preservation

Go's `html.Parse` treats as HTML5 and removes XML declarations.

**Pre-process:** Extracts `<?xml version="1.0" encoding="UTF-8"?>`
**Post-process:** Restores XML declaration and XHTML self-closing elements

### Void Element Conversion

```
HTML5: <img attr>
XHTML: <img attr/>
```

Converts: `area`, `base`, `br`, `col`, `embed`, `hr`, `img`, `input`, `link`, `meta`, `param`, `source`, `track`, `wbr`

Also handles: `<script></script>` → `<script/>`, `<a id="..."></a>` → `<a id="..."/>`

## kobo.js API Reference

Injected JavaScript provides:

```javascript
// Position/Progress
getPosition()           // Current reading position
getProgress()           // Progress percentage
getPageCount()          // Total pages
getCurrentPage()        // Current page number

// Setup
setupBookColumns()      // Configure column layout
paginate(tagId)         // Paginate with optional anchor
repaginate(tagId)       // Re-paginate
updateProgress()        // Update progress value
updateBookmark()        // Save bookmark

// Navigation
goBack()                // Previous page/chapter
goForward()             // Next page/chapter
goPage(pageNum, callPageReady)  // Jump to page
goProgress(progress)    // Jump to progress position

// Anchors
estimateFirstAnchorForPageNumber(page)
estimatePageNumberForAnchor(spanId)
```

## Key Functions

```go
// EPUB to KePub conversion
func (c *Converter) ConvertEPUB(ctx context.Context, srcPath, destPath string) error

// CBZ to KePub conversion
func ConvertCBZWithMetadata(ctx context.Context, srcPath, destPath string, metadata *CBZMetadata) error

// Content transformation
func TransformContent(r io.Reader, w io.Writer) error
func TransformContentWithOptions(r io.Reader, w io.Writer, counter *SpanCounter, scriptPath string) error

// OPF transformation
func TransformOPF(r io.Reader, w io.Writer) error
```

## Integration Points

**Generator Interface:**
- `GetKepubGenerator()` returns appropriate generator (EPUB or CBZ)
- `SupportsKepub()` checks if format can be converted

**Download Cache:**
- Fingerprint includes `format: "kepub"` vs `format: "original"`
- Separate cache files: `{id}.kepub.epub`, `{id}.kepub.meta.json`

**API Endpoints:**
- `GET /api/books/files/:id/download/kepub` - Download as KePub
- OPDS routes with `/kepub/` prefix for KePub-aware clients

## Edge Cases

**Idempotent Conversion:**
- Can convert already-converted KePub (spans not double-wrapped)

**Author Deduplication:**
- Key: `{name}|{role}`
- Prevents duplicate entries for same person in same role

**XML Special Character Escaping:**
- `&`, `<`, `>`, `'` properly escaped in metadata

**Lossless Preservation:**
- EPUB: Text preserved exactly (only wrapping added)
- CBZ: Small images (<1264×1680) copied unchanged

**Natural Page Sorting:**
- `page2` sorts before `page10`

## Related Files

- `pkg/kepub/converter.go` - EPUB to KePub conversion
- `pkg/kepub/cbz.go` - CBZ to KePub conversion
- `pkg/kepub/content.go` - XHTML content transformation
- `pkg/kepub/xhtml.go` - XHTML pre/post processing
- `pkg/kepub/opf.go` - OPF transformation
- `pkg/kepub/kobo.js` - Injected JavaScript
- `pkg/kepub/converter_test.go` - EPUB conversion tests
- `pkg/kepub/cbz_test.go` - CBZ conversion tests
- `pkg/kepub/content_test.go` - Content transformation tests
- `pkg/kepub/opf_test.go` - OPF transformation tests
- `pkg/filegen/kepub_cbz.go` - KePub CBZ generator wrapper
