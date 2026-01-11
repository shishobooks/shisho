# KePub Format Reference

This skill documents the KePub (Kobo EPUB) format as used in Shisho for conversion and generation.

## Overview

KePub is Kobo's enhanced EPUB format with:

- Span wrapping for reading statistics
- Kobo-specific XHTML structure
- Fixed-layout support for comics
- Enhanced metadata for Kobo devices

## File Structure

KePub files are EPUBs with `.kepub.epub` extension:

```
mimetype
META-INF/
  container.xml
OEBPS/
  content.opf              # Enhanced OPF with Kobo extensions
  toc.ncx
  *.xhtml                  # Content with kobo spans
  styles/
    kobo.css               # Kobo-specific styles
```

## Kobo Extensions

### XHTML Span Wrapping

Kobo wraps text in spans for reading progress:

```html
<!-- Original EPUB -->
<p>This is a paragraph.</p>

<!-- KePub -->
<p>
  <span class="koboSpan" id="kobo.1.1">This is a paragraph.</span>
</p>
```

### Kobo CSS

```css
/* Required Kobo styles */
.koboSpan {
  /* Kobo reading progress tracking */
}
```

### Fixed-Layout for Comics

For CBZ-to-KePub conversion, uses fixed-layout:

```xml
<!-- In OPF metadata -->
<meta property="rendition:layout">pre-paginated</meta>
<meta property="rendition:spread">none</meta>
<meta property="rendition:orientation">auto</meta>
```

XHTML page structure:

```html
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <meta charset="utf-8" />
    <title>Page 1</title>
    <style>
      html,
      body {
        margin: 0;
        padding: 0;
      }
      img {
        max-width: 100%;
        max-height: 100%;
      }
    </style>
  </head>
  <body>
    <img src="images/page001.jpg" alt="Page 1" />
  </body>
</html>
```

## Shisho Implementation

### EPUB to KePub Conversion (`pkg/kepub/converter.go`)

Transforms standard EPUB to KePub:

1. Add span wrapping to content files
2. Add Kobo-specific styles
3. Update manifest with new files
4. Rename to `.kepub.epub`

### CBZ to KePub Conversion (`pkg/kepub/cbz.go`)

Converts CBZ comics to fixed-layout KePub:

1. Extract page images from CBZ
2. Generate fixed-layout XHTML pages
3. Create OPF with comic metadata
4. Build NCX navigation
5. Package as KePub

### CBZMetadata Structure

```go
type CBZMetadata struct {
    Title        string
    Authors      []AuthorInfo
    Series       string
    SeriesNumber *float64
    Genres       []string    // Written as dc:subject
    Tags         []string    // Written as calibre:tags
    PageCount    int
    CoverPage    int
}
```

### OPF Generation

```go
func generateFixedLayoutOPF(meta CBZMetadata, pages []pageInfo) []byte {
    // Generate OPF with:
    // - dc:title, dc:creator
    // - dc:subject for each genre
    // - calibre:series, calibre:series_index meta
    // - calibre:tags meta
    // - Fixed-layout rendition properties
    // - Manifest with page items
    // - Linear spine
}
```

### Key Functions

```go
// Convert EPUB to KePub
func ConvertToKePub(srcPath, destPath string) error

// Convert CBZ to KePub
func ConvertCBZToKePub(ctx context.Context, srcPath, destPath string, meta CBZMetadata) error

// Transform existing OPF (preserves metadata)
func TransformOPF(opfContent []byte) ([]byte, error)
```

## Metadata Handling

### From CBZ to KePub

| CBZ Field | KePub Element                        |
| --------- | ------------------------------------ |
| Title     | `<dc:title>`                         |
| Series    | `<meta name="calibre:series">`       |
| Number    | `<meta name="calibre:series_index">` |
| Writer    | `<dc:creator opf:role="aut">`        |
| Genre     | `<dc:subject>` (one per genre)       |
| Tags      | `<meta name="calibre:tags">`         |

### EPUB to KePub

Metadata is preserved via OPF transformation. No changes to metadata elements.

## Cover Handling

For CBZ-to-KePub:

1. Cover page identified by CoverPage index or page type
2. First page used as cover if not specified
3. Cover referenced in OPF manifest with cover-image properties

## Navigation

NCX file generated with:

- One navPoint per page
- Play order matching spine order
- Page titles as "Page N"

## Related Files

- `pkg/kepub/converter.go` - EPUB to KePub conversion
- `pkg/kepub/cbz.go` - CBZ to KePub conversion
- `pkg/kepub/opf.go` - OPF transformation
- `pkg/kepub/opf_test.go` - OPF transformation tests
- `pkg/kepub/cbz_test.go` - CBZ conversion tests
- `pkg/kepub/content.go` - XHTML content handling
- `pkg/kepub/content_test.go` - Content transformation tests
