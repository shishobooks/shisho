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

## Aliases

Aliases are alternative names for resources that resolve to the canonical resource during any name-based lookup. When metadata arrives from different sources — embedded file data, plugins, sidecars, user edits — the same logical resource is often represented by different name variants. For example, "Nonfiction" vs "Non-fiction", "J.K. Rowling" vs "Joanne Rowling", or "Sci-Fi" vs "Science Fiction". Without aliases, each variant creates a separate resource that must be manually merged, and the duplicate reappears on the next scan.

Aliases solve this by letting you declare that certain names map to an existing resource. When a name matches an alias, Shisho returns the existing resource instead of creating a new one.

### Supported Resources

All six resource types support aliases:

- **People** (authors and narrators)
- **Series**
- **Genres**
- **Tags**
- **Publishers**
- **Imprints**

### How Aliases Work

When Shisho encounters a resource name during a scan, [plugin](./plugins/overview) enrichment, or [sidecar](./sidecar-files) import, it resolves the name in this order:

1. **Primary name** (case-insensitive) — if a resource with this name exists, use it
2. **Aliases** (case-insensitive) — if the name matches an alias, use the alias's canonical resource
3. **Create new** — if no match is found, create a new resource

This resolution happens transparently in all contexts — library scans, plugin metadata, sidecar files, and manual edits via autocomplete. No changes to plugins or sidecar files are needed.

### Managing Aliases

**Edit dialog.** Open the edit dialog for any resource (person, series, genre, tag, publisher, or imprint). Below the name field, a chip input lets you add and remove aliases. Type a name and press Enter to add it; click the × on a chip to remove it.

**Automatic creation on merge.** When you merge two resources, the source resource's name automatically becomes an alias of the target. Any existing aliases on the source transfer to the target as well, so no previously-working name mappings are lost.

**Automatic creation on rename.** When you rename a resource, the old name automatically becomes an alias. Future references to the old name still resolve to the renamed resource.

### Uniqueness Rules

- Aliases are **case-insensitive** — "non-fiction" and "Non-Fiction" are treated as the same alias
- An alias cannot duplicate an existing primary name or another alias within the same resource type and library
- Shisho validates uniqueness when you add an alias and rejects conflicts

### Aliases in Search

- **Autocomplete:** When editing a book and typing in a resource field, the search matches against both primary names and aliases. Results show only the canonical name.
- **Full-text search:** Author, narrator, and series aliases are included in book search results, so searching by an alias name surfaces the correct books.
- **Resource search:** Searching on genre, tag, series, or person list pages also matches against aliases.

### Aliases in the UI

- **List pages:** Genres, tags, publishers, imprints, and people show aliases as a muted subtitle below the primary name. Series grid cards don't show aliases due to layout constraints.
- **Detail pages:** All six resource types show aliases below the name in the header section.

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
- [Aliases](#aliases)

**On a series:**
- Name and sort name
- [Aliases](#aliases)

**On a genre, tag, publisher, or imprint:**
- Name
- [Aliases](#aliases)

### Sort Names

Shisho automatically generates sort names from display names (e.g., "J.R.R. Tolkien" becomes "Tolkien, J.R.R."). If you manually set a sort name, it won't be overwritten. Clearing a manual sort name reverts to auto-generation.

### Identify Review

When you identify a book against a metadata plugin, the review screen splits the proposed metadata into two sections — **Book** (title, subtitle, authors, series, genres, tags, description) and **File** (cover, name, narrators, publisher, imprint, language, release date, URL, identifiers, abridged). Each row carries a checkbox: only the checked fields are written when you click Apply.

**Smart defaults at open time** decide which boxes start checked, so most identifies are one click:

- **File-level fields** default ON whenever there's something to apply. Each file owns its own copy, so applying the plugin's value can't trample shared metadata.
- **Book-level _new_ fields** (the book has no value, plugin proposes one) default ON.
- **Book-level _changed_ fields** (book and plugin disagree) default ON only when you're identifying the book's primary file. On a non-primary file (a "second-identify" against a different edition), they default OFF — the canonical book metadata stays put unless you opt in.
- **Unchanged fields** (book and plugin already match) default OFF.

A Book / File section banner sits sticky above each section with its own select-all checkbox and a live "X of Y selected" count. The global **Apply all** at the top of the body toggles every checkbox in the dialog.

The **Name** row corresponds to `file.Name` — the file-level title used for downloads and on-disk organization. Its proposed value defaults to the plugin's title, but you can edit it (e.g. "Harry Potter and the Sorcerer's Stone (Full-Cast Edition)") and the user-edited value is preserved. A "Copy from book title" button under the Name input quickly resyncs to whatever you've typed into Title above.

When you've flipped many boxes and want to start over, **Restore suggestions** in the footer reverts every checkbox and edited value back to the smart defaults without leaving the dialog.

Within each row, names that already exist in your library show as plain chips; new names that don't yet exist will be created automatically when you apply. Identifiers can be added, edited, and removed inline. Per-field "New" / "Changed" badges show how the incoming match compares to what you already have, and "Currently:" shows the existing value beside the input for easy reference.

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
