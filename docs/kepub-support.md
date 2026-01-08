# KePub Download Support

This document describes the KePub format conversion support for EPUB and CBZ files, providing improved reading experience on Kobo e-readers.

## Overview

### What is KePub?

KePub is Kobo's enhanced EPUB format that provides:

- **Reading statistics**: Track reading progress, time spent, and pages read
- **Better page numbering**: More accurate location tracking
- **Improved page turning**: Smoother navigation experience
- **Kobo-specific features**: Integration with Kobo's reading ecosystem

KePub files are standard EPUB files with:
1. A `.kepub.epub` extension
2. Kobo-specific span wrappers around text content
3. Wrapper divs in the body for pagination
4. Cover image property in the OPF manifest

### Conversion Approach

The conversion is **lossless** - original content is preserved exactly:
- EPUB: Text is wrapped in spans, but content remains identical
- CBZ: Images are copied byte-for-byte, converted to fixed-layout EPUB

## Library Settings

Each library has a `download_format_preference` setting with three options:

| Value      | Behavior                                      |
| ---------- | --------------------------------------------- |
| `original` | Always download in original format (default)  |
| `kepub`    | Auto-convert EPUB/CBZ to KePub on download    |
| `ask`      | Show format selection popover on download     |

### Affected File Types

Only EPUB and CBZ files can be converted to KePub:

| File Type | KePub Conversion |
| --------- | ---------------- |
| EPUB      | Supported        |
| CBZ       | Supported        |
| M4B       | Not applicable   |

## API Endpoints

### Web API

| Method | Endpoint                                      | Description                    |
| ------ | --------------------------------------------- | ------------------------------ |
| `GET`  | `/api/books/files/:id/download/kepub`         | Download as KePub format       |
| `HEAD` | `/api/books/files/:id/download/kepub`         | Trigger generation, check size |

### OPDS

Parallel routes with `/kepub/` prefix serve feeds with KePub download links:

| Endpoint                                    | Description                      |
| ------------------------------------------- | -------------------------------- |
| `/opds/v1/kepub/:types/catalog`             | Main catalog with KePub links    |
| `/opds/v1/kepub/:types/libraries/:id`       | Library catalog with KePub links |
| `/opds/v1/kepub/:types/libraries/:id/all`   | All books with KePub links       |
| `/opds/download/:id/kepub`                  | Download file as KePub           |

OPDS clients can choose to use the `/kepub/` prefixed routes to get KePub download links in their feeds.

## Conversion Process

### EPUB to KePub

1. **Span Wrapping**: Each sentence/text node is wrapped in a `<span>` with a unique ID:
   ```html
   <span class="koboSpan" id="kobo.1.1">Original text content.</span>
   ```
   - `kobo.X.Y` format: X = paragraph number, Y = sentence number
   - Enables Kobo's reading statistics and location tracking

2. **Body Wrapper Divs**: Body content is wrapped with:
   ```html
   <div id="book-columns">
     <div id="book-inner">
       <!-- original body content -->
     </div>
   </div>
   ```
   - Provides targets for Kobo's pagination CSS

3. **OPF Updates**: Cover image gets `properties="cover-image"` in manifest

4. **Skip Elements**: Script, style, pre, code, SVG, and math elements are not transformed

### CBZ to KePub

CBZ files are converted to fixed-layout EPUB with KePub enhancements:

1. **Fixed Layout EPUB**: Creates EPUB 3 with `rendition:layout` set to `pre-paginated`

2. **Image Pages**: Each CBZ image becomes a full-page element:
   - SVG wrapper with original image dimensions
   - Viewport metadata matches original image size
   - Images are copied byte-for-byte (lossless)

3. **KePub Spans**: Minimal spans added to enable Kobo features

4. **Metadata**: Embeds book metadata from the database:
   - Title from book model
   - Authors as `dc:creator` elements (deduplicated by name + role)
   - Series as `belongs-to-collection` with `group-position` for number
   - Cover image property on first image
   - NCX navigation file with book title
   - Fixed-layout CSS

5. **Author Role Mapping**: CBZ creator roles are mapped to standard codes:
   - `writer` → `aut` (author)
   - `penciller`, `artist`, `inker` → `art` (artist)
   - `colorist` → `clr`
   - `letterer` → `ill` (illustrator)
   - `cover artist`, `cover` → `cov`
   - `editor` → `edt`

6. **Page Order**: Natural sorting of filenames (e.g., page2 before page10)

## Caching

KePub files are cached separately from original generated files:

### Cache Files

```
{cache_dir}/
├── 123.epub           # Generated EPUB with metadata
├── 123.meta.json      # Metadata for original
├── 123.kepub.epub     # Generated KePub
└── 123.kepub.meta.json  # Metadata for KePub
```

### Fingerprint

The cache fingerprint includes a `format` field to differentiate:
- `format: "original"` for standard downloads
- `format: "kepub"` for KePub downloads

This ensures the correct cached file is served based on the requested format.

## Download Filename

KePub downloads use the `.kepub.epub` extension:

```
[Author] Series #1 - Title.kepub.epub
```

## Frontend Integration

### Library Settings

Both Create Library and Library Settings pages include a "Download Format Preference" dropdown with options:
- Original format
- KePub (Kobo-optimized)
- Ask on download

### Book Detail Page

When downloading a file:

1. **Original preference**: Downloads with standard metadata embedding
2. **KePub preference**: Downloads as KePub for EPUB/CBZ files
3. **Ask preference**: Shows a popover with format options

The `DownloadFormatPopover` component displays when:
- Library preference is set to "ask"
- File type is EPUB or CBZ

## Architecture

### Package Structure

```
pkg/
├── kepub/                    # Core conversion logic
│   ├── converter.go          # Main Converter struct
│   ├── content.go            # HTML transformation (spans, wrappers)
│   ├── opf.go                # OPF transformation (cover property)
│   └── cbz.go                # CBZ to fixed-layout EPUB conversion
├── filegen/                  # Generator interface
│   ├── generator.go          # GetKepubGenerator, SupportsKepub
│   ├── kepub_epub.go         # KePub EPUB generator
│   └── kepub_cbz.go          # KePub CBZ generator
├── downloadcache/            # Cache management
│   ├── cache.go              # GetOrGenerateKepub method
│   ├── metadata.go           # KePub-specific metadata functions
│   └── fingerprint.go        # Format field in fingerprint
├── books/                    # API endpoints
│   ├── handlers.go           # downloadKepubFile handler
│   └── routes.go             # KePub download routes
└── opds/                     # OPDS integration
    ├── routes.go             # KePub route group
    ├── handlers.go           # KePub catalog handlers
    └── service.go            # bookToEntryWithKepub method
```

### Frontend Components

```
app/
├── components/
│   ├── library/
│   │   └── DownloadFormatPopover.tsx  # Format selection popover
│   └── pages/
│       ├── CreateLibrary.tsx          # Download preference field
│       ├── LibrarySettings.tsx        # Download preference field
│       └── BookDetail.tsx             # Format-aware download logic
└── types/generated/
    └── models.ts                      # DownloadFormat* constants
```

## Error Handling

### Unsupported File Types

Requesting KePub for M4B files returns a 400 error:
```json
{
  "message": "kepub conversion is not supported for m4b files"
}
```

### Conversion Errors

If conversion fails:
1. Error is logged
2. JSON error response returned
3. Frontend shows error dialog with "Download Original" fallback

## Performance

### Conversion Time

- **EPUB to KePub**: Typically <1 second for most books
- **CBZ to KePub**: Depends on image count, typically 1-5 seconds

### Caching

Once converted, subsequent downloads are served from cache with only fingerprint comparison overhead (~2ms).
