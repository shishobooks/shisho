# Download Generation System

This document describes the system for generating modified book files on download, embedding updated metadata from the application database into the downloaded file.

## Overview

### Problem

When users modify book metadata (title, authors, series, cover) in the application, these changes are stored in the database and sidecar files, but the original book files remain unchanged. This is intentional - original files should never be modified to support hard-linking from other locations.

### Solution

When a user downloads a file, instead of serving the original file directly, the system:

1. Checks if a valid cached version exists
2. If not, generates a new file with updated metadata embedded
3. Serves the generated file from the cache

The original files remain untouched, and users always get files with their current metadata.

## Architecture

### Components

```
pkg/
├── config/                      # Configuration
│   └── config.go               # Cache directory and size settings
├── downloadcache/              # Cache management
│   ├── cache.go                # Core cache logic
│   ├── cleanup.go              # LRU cleanup (background)
│   ├── filename.go             # Download filename formatting
│   ├── fingerprint.go          # Cache invalidation via fingerprinting
│   └── metadata.go             # Cache metadata file operations
├── filegen/                    # File generation
│   ├── generator.go            # Generator interface
│   ├── epub.go                 # EPUB generation
│   ├── m4b.go                  # M4B generation
│   └── cbz.go                  # CBZ generation (stub)
├── books/                      # API endpoints
│   ├── handlers.go             # Download handlers
│   └── routes.go               # Download routes
└── opds/                       # OPDS integration
    └── handlers.go             # Modified download handler
```

### API Endpoints

| Method | Endpoint                                 | Description                                         |
| ------ | ---------------------------------------- | --------------------------------------------------- |
| `GET`  | `/api/books/files/:id/download`          | Download generated file with embedded metadata      |
| `GET`  | `/api/books/files/:id/download/original` | Download original file directly (bypass generation) |
| `GET`  | `/opds/download/:id`                     | OPDS download (uses generation logic)               |

## Configuration

### New Config Fields

```yaml
# Path to cache directory for generated downloads
# Env: DOWNLOAD_CACHE_DIR
# Default: /config/cache/downloads
download_cache_dir: /config/cache/downloads

# Maximum cache size in gigabytes
# Env: DOWNLOAD_CACHE_MAX_SIZE_GB
# Default: 5
download_cache_max_size_gb: 5
```

### Startup Validation

On startup, the application:

1. Creates the cache directory if it doesn't exist
2. Verifies write permissions by creating and deleting a temp file
3. Fails with a fatal error if the directory is not writable

## Cache Invalidation

### Fingerprint-Based Invalidation

Instead of using `UpdatedAt` timestamps (which change on any modification), the system uses a **fingerprint** of only the fields that affect file generation.

#### Fingerprint Components

For EPUB files:

- Book title
- Book subtitle
- Authors (name, sort_order for each)
- Series (name, number, sort_order for each)
- Cover image (path, mime type, file modification time)

For M4B files:

- Book title
- Book subtitle
- Authors (name, sort_order for each)
- Narrators (name, sort_order for each)
- Series (name, number, sort_order for each)
- Cover image (path, mime type, file modification time)

For CBZ files:

- Book title
- Authors (name, role, sort_order for each)
- Series (name, number, sort_order for each)
- Cover page index (0-indexed page number)

#### Validation Flow

1. Compute current fingerprint from Book + File models
2. Hash the fingerprint (SHA256)
3. Compare with stored hash in cache metadata
4. If mismatch or no cache exists → regenerate

### Cache Metadata

Each cached file `{file_id}.{ext}` has a corresponding `{file_id}.meta.json`:

```json
{
  "file_id": 123,
  "fingerprint_hash": "abc123...",
  "generated_at": "2024-01-01T00:00:00Z",
  "last_accessed_at": "2024-01-02T00:00:00Z",
  "size_bytes": 1048576
}
```

## File Generation

### EPUB Generation

The EPUB generator:

1. Opens source EPUB as a zip archive
2. Creates new zip at cache location
3. For each file in source:
   - `.opf` file → parse XML, modify metadata, write modified version
   - Cover image → replace if cover has changed
   - All other files → copy unchanged via `io.Copy`
4. Uses atomic write (temp file + rename) for safety

#### OPF Modifications

The following elements are modified while preserving the rest of the OPF structure:

- `<dc:title>` → book title
- `<dc:creator role="aut">` → all authors, sorted by sort_order
- Calibre meta tags → all series information
- Subtitle in appropriate metadata element

### M4B Generation

The M4B generator:

1. Parses source M4B using go-mp4 library
2. Preserves existing metadata (description, genre, chapters, duration, bitrate)
3. Updates metadata from book/file models
4. Writes to cache location using atomic pattern (temp file + rename)

#### Metadata Mapping

| Shisho Field | M4B Atom                         | Notes                              |
| ------------ | -------------------------------- | ---------------------------------- |
| Title        | `©nam`                           | Standard iTunes title atom         |
| Subtitle     | `----:com.apple.iTunes:SUBTITLE` | Freeform atom                      |
| Authors      | `©ART`                           | Comma-separated artist names       |
| Narrators    | `©nrt` + `©cmp`                  | Written to both for compatibility  |
| Series       | `©alb`                           | Formatted as "Series Name #N"      |
| Cover        | `covr`                           | Image data atom                    |

#### Series Number Formatting

- Integer numbers: `Series Name #1`
- Decimal numbers: `Series Name #1.5`
- No number: `Series Name`

### CBZ Generation

The CBZ generator:

1. Opens source CBZ as a zip archive
2. Creates new zip at cache location
3. Parses existing ComicInfo.xml (if present) to preserve untracked fields
4. For each file in source:
   - `ComicInfo.xml` → modify tracked fields, write modified version
   - All other files (page images) → copy unchanged
5. If no ComicInfo.xml existed, creates one
6. Uses atomic write (temp file + rename) for safety

#### ComicInfo.xml Modifications

Only the following fields are modified (all others are preserved from the original):

| Shisho Field | ComicInfo Element | Notes                              |
| ------------ | ----------------- | ---------------------------------- |
| Title        | `<Title>`         | Book title                         |
| Series       | `<Series>`        | Primary series name                |
| Number       | `<Number>`        | Series number (integer or decimal) |
| Authors      | Creator fields    | Mapped by role (see below)         |
| Cover page   | `<Pages>`         | FrontCover type on specified page  |

#### Author Role Mapping

CBZ files support distinct creator roles from ComicInfo.xml v2.1. Authors are written to the appropriate element based on their role:

| Author Role   | ComicInfo Element |
| ------------- | ----------------- |
| `writer`      | `<Writer>`        |
| `penciller`   | `<Penciller>`     |
| `inker`       | `<Inker>`         |
| `colorist`    | `<Colorist>`      |
| `letterer`    | `<Letterer>`      |
| `cover_artist`| `<CoverArtist>`   |
| `editor`      | `<Editor>`        |
| `translator`  | `<Translator>`    |
| (no role)     | `<Writer>`        |

Multiple authors with the same role are comma-separated (e.g., `Writer One, Writer Two`).

#### Cover Page Handling

Unlike EPUB/M4B files which have embedded cover images, CBZ files use a page index to identify the cover:

- The `File.CoverPage` field stores the 0-indexed page number
- During generation, the `<Pages>` section is updated to set `Type="FrontCover"` on the specified page
- The actual page image is not modified
- Cover page index is extracted during scanning from ComicInfo.xml if a FrontCover type exists, otherwise defaults to page 0

#### Series Number Formatting

- Integer numbers: `1`, `2`, `10`
- Decimal numbers: `1.5`, `2.25`

## Download Filename Format

Generated files are served with a formatted filename:

```
[{Author}] {Series} #{SeriesNumber} - {Title}.{ext}
```

For audiobooks with narrators:

```
[{Author}] {Series} #{SeriesNumber} - {Title} {Narrator}.{ext}
```

### Rules

- **Author**: First author by `sort_order`, using `Name` field
- **Series**: First series by `sort_order`, using `Name` field
- **Series Number**: Integer if whole number (`1`), decimal otherwise (`1.5`)
- **Narrator**: First narrator by `sort_order`, using `Name` field (audiobooks only)
- **No series**: `[{Author}] {Title}.{ext}`
- **No author**: `{Title}.{ext}`
- **Volume in title**: If title contains a volume indicator (e.g., `v1`, `Vol. 2`, `V3`), series is omitted from filename and volume numbers are padded to 3 digits for lexicographic sorting (e.g., `v1` → `v001`)
- **Sanitization**: Invalid characters (`/ \ : * ? " < > |`) are removed

### Examples

- `[Brandon Sanderson] The Stormlight Archive #1 - The Way of Kings.epub`
- `[George Orwell] 1984.epub` (no series)
- `[Andy Weir] Standalone #1 - Project Hail Mary {Ray Porter}.m4b` (audiobook with narrator)
- `[Author Name] My Manga v001.cbz` (volume in title, series omitted, padded to 3 digits)

## LRU Cache Cleanup

### Trigger

Cleanup runs as a background goroutine after each file generation.

### Algorithm

1. List all `.meta.json` files in cache directory
2. Parse each, collect: `{file_id, size_bytes, last_accessed_at}`
3. Sum total size
4. If total > max size:
   - Sort by `last_accessed_at` ascending (oldest first)
   - Delete files until total < (max size \* 0.8)
   - Delete both cached file and `.meta.json`

### Concurrency Safety

Uses file-based locking to prevent concurrent cleanup operations.

## Error Handling

### Generation Errors

If file generation fails:

1. API returns JSON error response with descriptive message
2. Frontend shows error toast/dialog with:
   - Error message explaining the failure
   - "Download Original File" button as fallback
3. User can download the original unmodified file via `/download/original`

### OPDS Errors

For OPDS clients (which can't show UI):

- On generation error, falls back to serving the original file
- Error is logged for administrator awareness

## Performance Considerations

### Fast Path (Cache Hit)

1. Load File + Book from DB (already required)
2. Compute fingerprint hash (~1ms)
3. Read cache metadata file (~1ms)
4. Compare hashes → match → serve cached file

### Slow Path (Cache Miss)

1. Generate new file (target: <2 seconds for typical EPUB)
   - Stream-based zip processing (no full file in memory)
   - `io.Copy` for unchanged files (no decompression)
2. Write cache metadata
3. Spawn background cleanup goroutine (non-blocking)
4. Serve generated file

## Future Enhancements

- **Cover resizing**: Optionally resize large covers to reduce file size
- **Batch downloads**: Download multiple files as a zip archive
- **Progress indication**: Show generation progress for large files
- **CBZ cover page selection**: UI to select which page is the cover (currently extracted from ComicInfo.xml or defaults to first page)
