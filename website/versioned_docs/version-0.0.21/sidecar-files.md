---
sidebar_position: 12
---

# Sidecar Files

Sidecar files are JSON files that store metadata alongside your book files on disk. They let you customize metadata without modifying the original files and ensure your edits survive file moves or re-imports.

## How They Work

When you edit metadata through the Shisho interface, the changes are saved both to the database and to `.metadata.json` sidecar files next to your book files. During library scans, Shisho reads these sidecars and applies them with higher priority than embedded file metadata, so your customizations are preserved.

### File Locations

Sidecar files are created alongside your book files:

**Directory-based books:**
```
[Author] Book Title/
├── book.epub
├── book.epub.metadata.json       ← file sidecar
└── Book Title.metadata.json      ← book sidecar (named after directory)
```

**Root-level books:**
```
library/
├── Book Title.m4b
├── Book Title.m4b.metadata.json  ← file sidecar
└── Book Title.metadata.json      ← book sidecar (named after file, without extension)
```

### Two Levels of Sidecars

There are two types of sidecar files, corresponding to the two levels of [metadata](./metadata) in Shisho:

- **Book sidecars** store book-level metadata: title, authors, series, genres, tags
- **File sidecars** store file-level metadata: narrators, publisher, identifiers, chapters

## Book Sidecar Format

```json
{
  "version": 1,
  "title": "The Great Gatsby",
  "sort_title": "Great Gatsby, The",
  "subtitle": "A Novel",
  "description": "A story about the American Dream.",
  "authors": [
    {
      "name": "F. Scott Fitzgerald",
      "sort_name": "Fitzgerald, F. Scott",
      "sort_order": 0
    }
  ],
  "series": [
    {
      "name": "Classic American Literature",
      "number": 5,
      "sort_order": 0
    }
  ],
  "genres": ["Fiction", "Classic"],
  "tags": ["american-literature", "1920s"]
}
```

For CBZ comics, authors can include a `role` field:

```json
{
  "authors": [
    { "name": "Alan Moore", "role": "writer" },
    { "name": "Dave Gibbons", "role": "penciller" }
  ]
}
```

Valid roles: `writer`, `penciller`, `inker`, `colorist`, `letterer`, `cover_artist`, `editor`, `translator`

## File Sidecar Format

```json
{
  "version": 1,
  "name": "Custom Display Name",
  "narrators": [
    {
      "name": "Stephen Fry",
      "sort_name": "Fry, Stephen",
      "sort_order": 0
    }
  ],
  "publisher": "Penguin Books",
  "imprint": "Penguin Classics",
  "release_date": "2004-09-30",
  "url": "https://example.com/book",
  "identifiers": [
    { "type": "isbn_13", "value": "9780743273565" },
    { "type": "asin", "value": "B000FC1GJC" }
  ],
  "chapters": [
    {
      "title": "Chapter 1",
      "start_timestamp_ms": 0,
      "children": [
        { "title": "Section 1.1", "start_timestamp_ms": 30000 }
      ]
    }
  ],
  "cover_page": 0
}
```

Chapter position fields are mutually exclusive based on file type:
- **CBZ**: `start_page` (0-indexed page number)
- **M4B**: `start_timestamp_ms` (milliseconds from start)
- **EPUB**: `href` (content document reference)

## Priority System

Sidecar metadata sits between manual edits and embedded file metadata in the priority hierarchy:

| Priority | Source |
|----------|--------|
| Highest | Manual edits (web interface) |
| | **Sidecar files** |
| | [Plugin](./plugins/overview) data |
| | Embedded file metadata |
| Lowest | Filepath |

This means:
- Sidecar values **override** embedded file metadata and filepath-derived data
- Manual edits through the interface **override** sidecar values
- When you make a manual edit, the sidecar is also updated to stay in sync

### Resync with Refresh

The **Resync with refresh** option on a book skips sidecar files and re-reads metadata directly from the embedded file data. This is useful if a sidecar has incorrect data and you want to reset to what's in the file. Manual edits are still preserved.

## When Sidecars Are Read

Sidecar files are read during library scans, after parsing the embedded file metadata. If a sidecar exists, its values are applied according to the priority system.

## When Sidecars Are Written

Sidecar files are automatically written whenever you edit metadata through the Shisho interface. This keeps the on-disk sidecars in sync with the database, so the customizations persist if you ever need to re-scan or move your library.

All fields in the sidecar are optional — only fields with values are included.
