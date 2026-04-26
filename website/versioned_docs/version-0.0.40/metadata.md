---
sidebar_position: 50
---

# Metadata Management

Shisho extracts metadata from your book files and organizes it into a structured set of resources. You can edit metadata through the web interface, and Shisho tracks the source of each field so your manual edits are never overwritten by automated scans.

> See also: [Review State](./review-state.md) for how field completeness drives the "Needs review" queue.

## Resources

### Books

A book is the central entity in Shisho. It groups one or more files together (e.g., an EPUB and an M4B of the same title) and holds shared metadata.

**Book-level fields:** title, sort title, subtitle, description, authors, series, genres, tags

### Files

Each book contains one or more files. Files hold format-specific metadata that may differ between editions.

**File-level fields:** name, narrators (M4B only), publisher, imprint, release date, URL, identifiers, chapters, language, abridged

### People

People represent both **authors** and **narrators**. The same person record is shared across both roles, so renaming an author automatically updates everywhere they appear.

### Series

A book can belong to multiple series, each with an optional series number. Series numbers support decimals (e.g., `1.5` for a side story between books 1 and 2).

### Genres and Tags

Genres and tags are simple labels attached to books. The distinction is semantic — genres are typically extracted from file metadata, while tags are more often user-defined.

### Publishers and Imprints

Publishers and imprints are attached at the **file level**, not the book level. This means different editions of the same book can have different publishers.

### Identifiers

Identifiers (ISBN, ASIN, etc.) are also file-level. Each file can have multiple identifiers of **different** types: `isbn_10`, `isbn_13`, `asin`, `uuid`, `goodreads`, `google`, and custom types registered by [plugins](./plugins/overview).

A file has at most **one identifier per type**. You can have an ISBN-13 and an ASIN on the same file, but you cannot have two ASINs. The file edit dialog enforces this in the type dropdown — types already in use are greyed out until you remove the existing entry.

When you confirm an identify match (via the Identify dialog) and the match brings in an identifier whose type already exists on the file, the incoming value **replaces** the existing one. Identifiers of types not in the match are kept untouched.

Identifier values are **canonicalized on write** so comparisons and lookups are insensitive to cosmetic formatting:

- **ISBN-10 / ISBN-13**: hyphens, spaces, and `ISBN:` prefixes are stripped. `978-0-316-76948-8` is stored as `9780316769488`.
- **ASIN**: uppercased. `b08n5wrwnw` is stored as `B08N5WRWNW`.
- **UUID**: lowercased, with any leading `urn:uuid:` prefix removed.
- **Other types**: leading/trailing whitespace is trimmed.

Searches by identifier accept any of the cosmetic variants above — you can paste a hyphenated ISBN into the search box and still find the stored canonical value.

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
- Language
- Abridged status
- File role (promote a [supplement](./supplement-files) to a main file or vice versa)

**On a person:**
- Name and sort name

**On a series:**
- Name and sort name

### Sort Names

Shisho automatically generates sort names from display names (e.g., "J.R.R. Tolkien" becomes "Tolkien, J.R.R."). If you manually set a sort name, it won't be overwritten. Clearing a manual sort name reverts to auto-generation.

### Identify Review

When you identify a book against a metadata plugin, the review screen lets you edit every field — authors, narrators, series, publisher, imprint, genres, tags, identifiers — using the same combobox inputs as the regular edit forms, with autocomplete against your existing library entities. Names that already exist in your library show as plain chips; names that don't yet exist render with a dashed-outline indicator and will be created automatically when you apply the changes. Identifiers can be added, edited, and removed inline; release date uses a calendar picker. Per-field "New" / "Changed" / "Unchanged" badges show how the incoming match compares to what you already have.

## How Metadata Is Extracted

During library scans, Shisho reads embedded metadata from each file format:

### EPUB

Extracted from the OPF package document (`content.opf`):

- **Dublin Core**: title, authors (with roles), description, publisher, release date, identifiers, genres (from subjects), language (BCP 47 tag from `<dc:language>`)
- **Calibre metadata**: series name and number, subtitle
- **Cover**: from manifest item with `properties="cover-image"` or the `cover` meta tag
- **Chapters**: from EPUB 3 nav document, falling back to NCX table of contents

### CBZ

Extracted from `ComicInfo.xml`:

- **Basic**: title, series, number, summary, publisher, imprint, URL, release date, language (`LanguageISO` field, BCP 47 tag)
- **Creators**: writer, penciller, inker, colorist, letterer, cover artist, editor, translator (each as a distinct role)
- **Categorization**: genres and tags (comma-separated)
- **Identifiers**: GTIN
- **Cover**: from the page marked `Type="FrontCover"`, falling back to the first image
- **Chapters**: auto-detected from directory structure in image filenames

### M4B

Extracted from iTunes-style MP4 atoms:

- **Standard atoms**: title, artists/authors, genre, publisher, description, year
- **Narrators**: from the `©nrt` atom, falling back to `©cmp` (composer) then `©wrt` (writer)
- **Series**: parsed from the Audible-style `com.apple.iTunes:SERIES` and `com.apple.iTunes:SERIES-PART` freeform atoms (preferred), falling back to the `©grp` grouping atom (patterns like "Series Name #1" or "Series Name, Book 1"). Album (`©alb`) is not a series source — it holds the book title.
- **Identifiers**: ASIN from freeform iTunes atoms
- **Language**: from freeform iTunes atoms
- **Abridged**: from the Tone freeform atom `com.pilabor.tone:ABRIDGED` (`true`/`false`, or `1`/`0`)
- **Technical**: duration, bitrate, codec from media stream data
- **Cover**: from the `covr` atom
- **Chapters**: from the `chpl` chapter list atom

### PDF

Extracted from the PDF info dictionary:

- **Basic**: title, description (from Subject), tags (from Keywords), release date (from CreationDate), page count, language (from catalog `Lang` property, BCP 47 tag)
- **Authors**: split from the Author field on commas, ampersands, and semicolons
- **Cover**: largest embedded image from page 1, falling back to a rendered image of the first page
- **Chapters**: from the PDF document outline (bookmark tree), flattened to a linear list of page-anchored chapters. Edited chapters are written back into downloaded PDFs as a bookmark outline so your reader's chapter navigation stays in sync with the edits.

### Supplements

[Supplement files](./supplement-files) (text files, etc.) don't have metadata extracted. Their display name is derived from the filename.

## Metadata Priority

Shisho tracks the **source** of every metadata field. When a scan encounters new data, it only updates a field if the new source has equal or higher priority than the existing source:

| Priority | Source | Description |
|----------|--------|-------------|
| Highest | **Manual** | Edits made through the web interface |
| | **Sidecar** | Values from [`.metadata.json` sidecar files](./sidecar-files) |
| | **Plugin** | Data from [plugin](./plugins/overview) enrichers and parsers |
| | **File metadata** | Embedded metadata from EPUB, CBZ, M4B, and PDF files |
| Lowest | **Filepath** | Parsed from the filename and directory structure |

This means your manual edits are never overwritten by a normal scan. If you need to override the priority system, the **Rescan** dialog offers three modes:

- **Scan for new metadata** — Respects the priority system. Won't overwrite manual edits or higher-priority sources.
- **Refresh all metadata** — Bypasses the priority system and overwrites all fields, including manual edits. Re-runs plugins.
- **Reset to file metadata** — Clears all existing metadata (including manual edits) and re-scans the file from scratch, without running plugins. Fields not present in the source file are removed. The title and authors will fall back to the filepath if the file has no embedded values. Use this when plugin enrichment has misidentified a book and you want a clean slate.

### Title Normalization for CBZ Series Numbers

For CBZ files, titles with volume notation (e.g., `Series Name #7`, `Series Name Vol. 7`) are normalized to the canonical `Series Name v007` form so books sort correctly by volume. This normalization applies only to titles that came from **File metadata** or **Filepath** sources. Titles from **Manual**, **Sidecar**, or **Plugin** sources are stored verbatim — if a plugin search result shows `Naruto v1` and you apply it, the stored title stays `Naruto v1` instead of being rewritten.

### Series Number Unit (CBZ)

CBZ books have an additional field — **series number unit** — that records whether the series number refers to a **volume** or a **chapter**. This matters for manga and comics where chapter numbering is common alongside traditional volume numbering.

**How it's set automatically:** The scanner reads the indicator embedded in the CBZ filename:

| Filename pattern | Unit |
|-----------------|------|
| `Title v01.cbz`, `Title Vol.5.cbz`, `Title volume 12.cbz` | volume |
| `Title #001.cbz`, `Title 5.cbz` (bare number) | volume (default) |
| `Title Ch.5.cbz`, `Title chapter 5.cbz`, `Title c042.cbz` | chapter |

Ambiguous indicators (`#001`, bare trailing numbers) default to **volume** to preserve historical behavior.

**Other sources:** Plugin metadata and [sidecar files](./sidecar-files) can also supply the unit via a `unit` field on the series entry. Manual edits via the book edit dialog let you change the unit using the unit dropdown next to the series number field.

**CBZ books with no unit set** render as volumes for backward compatibility — the unit field being `null` is treated the same as `volume` in the reader and in file organization.

**Other formats:** EPUB, M4B, and PDF don't use this field. Their series numbering is always implicit (the number alone is sufficient without a volume/chapter distinction).

## Content fingerprints

Shisho stores a sha256 hash of every file's contents to preserve file identity
across renames and moves. See [File Fingerprints](./file-fingerprints.md) for
details on how move detection works and how the feature degrades when the
monitor isn't running.
