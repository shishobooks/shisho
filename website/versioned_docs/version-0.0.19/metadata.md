---
sidebar_position: 5
---

# Metadata Management

Shisho extracts metadata from your book files and organizes it into a structured set of resources. You can edit metadata through the web interface, and Shisho tracks the source of each field so your manual edits are never overwritten by automated scans.

## Resources

### Books

A book is the central entity in Shisho. It groups one or more files together (e.g., an EPUB and an M4B of the same title) and holds shared metadata.

**Book-level fields:** title, sort title, subtitle, description, authors, series, genres, tags

### Files

Each book contains one or more files. Files hold format-specific metadata that may differ between editions.

**File-level fields:** name, narrators (M4B only), publisher, imprint, release date, URL, identifiers, chapters

### People

People represent both **authors** and **narrators**. The same person record is shared across both roles, so renaming an author automatically updates everywhere they appear.

### Series

A book can belong to multiple series, each with an optional series number. Series numbers support decimals (e.g., `1.5` for a side story between books 1 and 2).

### Genres and Tags

Genres and tags are simple labels attached to books. The distinction is semantic — genres are typically extracted from file metadata, while tags are more often user-defined.

### Publishers and Imprints

Publishers and imprints are attached at the **file level**, not the book level. This means different editions of the same book can have different publishers.

### Identifiers

Identifiers (ISBN, ASIN, etc.) are also file-level. Each file can have multiple identifiers of different types: `isbn_10`, `isbn_13`, `asin`, `uuid`, `goodreads`, `google`, and custom types registered by [plugins](./plugins/overview).

## Relationships

| Relationship | Type | Notes |
|-------------|------|-------|
| Book &harr; Authors | Many-to-many | A book can have multiple authors; an author can appear on multiple books |
| Book &harr; Series | Many-to-many | A book can be in multiple series, each with its own number |
| Book &harr; Genres | Many-to-many | |
| Book &harr; Tags | Many-to-many | |
| Book &rarr; Files | One-to-many | A book has one or more files |
| File &harr; Narrators | Many-to-many | M4B audiobook files only |
| File &rarr; Publisher | Many-to-one | A file has at most one publisher |
| File &rarr; Imprint | Many-to-one | A file has at most one imprint |
| File &rarr; Identifiers | One-to-many | A file can have multiple identifiers |
| File &rarr; Chapters | One-to-many | Chapters support nested hierarchy |

## Editing Metadata

### What You Can Edit

**On a book:**
- Title, sort title, subtitle, description
- Authors (with roles for comics — writer, penciller, inker, etc.)
- Series membership and series numbers
- Genres and tags

**On a file:**
- Display name
- Narrators (M4B only)
- Publisher and imprint
- Release date
- URL
- Identifiers
- File role (promote a [supplement](./supplement-files) to a main file or vice versa)

**On a person:**
- Name and sort name

**On a series:**
- Name and sort name

### Sort Names

Shisho automatically generates sort names from display names (e.g., "J.R.R. Tolkien" becomes "Tolkien, J.R.R."). If you manually set a sort name, it won't be overwritten. Clearing a manual sort name reverts to auto-generation.

## How Metadata Is Extracted

During library scans, Shisho reads embedded metadata from each file format:

### EPUB

Extracted from the OPF package document (`content.opf`):

- **Dublin Core**: title, authors (with roles), description, publisher, release date, identifiers, genres (from subjects), language
- **Calibre metadata**: series name and number, subtitle
- **Cover**: from manifest item with `properties="cover-image"` or the `cover` meta tag
- **Chapters**: from EPUB 3 nav document, falling back to NCX table of contents

### CBZ

Extracted from `ComicInfo.xml`:

- **Basic**: title, series, number, summary, publisher, imprint, URL, release date
- **Creators**: writer, penciller, inker, colorist, letterer, cover artist, editor, translator (each as a distinct role)
- **Categorization**: genres and tags (comma-separated)
- **Identifiers**: GTIN
- **Cover**: from the page marked `Type="FrontCover"`, falling back to the first image
- **Chapters**: auto-detected from directory structure in image filenames

### M4B

Extracted from iTunes-style MP4 atoms:

- **Standard atoms**: title, artists/authors, genre, publisher, description, year
- **Narrators**: from the `©nrt` atom, falling back to `©cmp` (composer) then `©wrt` (writer)
- **Series**: parsed from the album name (patterns like "Series Name, Book 1")
- **Identifiers**: ASIN from freeform iTunes atoms
- **Technical**: duration, bitrate, codec from media stream data
- **Cover**: from the `covr` atom
- **Chapters**: from the `chpl` chapter list atom

### Supplements

[Supplement files](./supplement-files) (PDFs, text files, etc.) don't have metadata extracted. Their display name is derived from the filename.

## Metadata Priority

Shisho tracks the **source** of every metadata field. When a scan encounters new data, it only updates a field if the new source has equal or higher priority than the existing source:

| Priority | Source | Description |
|----------|--------|-------------|
| Highest | **Manual** | Edits made through the web interface |
| | **Sidecar** | Values from [`.metadata.json` sidecar files](./sidecar-files) |
| | **Plugin** | Data from [plugin](./plugins/overview) enrichers and parsers |
| | **File metadata** | Embedded metadata from EPUB, CBZ, and M4B files |
| Lowest | **Filepath** | Parsed from the filename and directory structure |

This means your manual edits are never overwritten by scans. If you want to reset a field to the value from the file, use the **Resync with refresh** option, which re-reads embedded file metadata and ignores the priority system (but still preserves manual edits for fields where the embedded value is empty).
