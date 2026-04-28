---
sidebar_position: 140
---

# Supplement Files

Supplement files are additional files associated with a book that aren't the primary readable media — things like companion PDFs, liner notes, artwork, or reference material. Shisho discovers these automatically during library scans and links them to the parent book.

## What Counts as a Supplement

Any file in a book's directory that isn't a main file type (EPUB, CBZ, M4B) is treated as a supplement. Common examples:

- PDF companion guides or liner notes
- Text files with notes or errata
- Image files (artwork, maps)
- Any other non-media files

## How Supplements Are Discovered

Discovery works differently depending on whether the book is in a directory or at the library root.

### Directory-Based Books

All non-main files in the book's directory (and its subdirectories) are linked as supplements:

```
[Author] Book Title/
├── book.epub              ← main file
├── companion-guide.pdf    ← supplement
├── notes.txt              ← supplement
└── extras/
    ├── map.jpg            ← supplement
    └── appendix.pdf       ← supplement
```

### Root-Level Books

For books that aren't in their own directory, only files with a **matching basename** are linked:

```
library/
├── My Book.m4b            ← main file
├── My Book.pdf            ← supplement (same basename)
├── My Book - Notes.txt    ← NOT linked (different basename)
└── Other File.pdf         ← NOT linked (different basename)
```

## Excluded Files

Some files are automatically excluded from supplement discovery:

- **Main file types**: `.epub`, `.cbz`, `.cbr`, `.m4b`
- **Shisho internal files**: cover images (`*.cover.*`) and [sidecar files](./sidecar-files) (`*.metadata.json`)
- **Hidden and system files**: configurable via `supplement_exclude_patterns`

### Exclude Patterns

The `supplement_exclude_patterns` [configuration](./configuration) option controls which files are skipped. The default patterns are:

```yaml
supplement_exclude_patterns:
  - ".*"
  - ".DS_Store"
  - "Thumbs.db"
  - "desktop.ini"
```

The `.*` pattern matches all hidden files (files starting with a dot). You can add additional patterns using glob syntax (e.g., `*.tmp`, `backup-*`).

## PDF Auto-Classification

Companion PDFs that share a directory with a main book file are sometimes named generically (`Supplement.pdf`, `Bonus Material.pdf`, etc.). To avoid manually demoting these every scan, Shisho automatically classifies a PDF as a supplement when:

1. Its basename matches an entry in `pdf_supplement_filenames` (case-insensitive, exact match — substrings do not match), AND
2. A sibling main file (`.epub`, `.cbz`, `.m4b`, or a [plugin-registered](./plugins/overview) file extension) exists in the same directory tree.

A PDF alone in its directory always imports as a main file regardless of name, so books are never silently dropped.

```
[Author] My Book/
├── My Book.epub          ← main file
└── Supplement.pdf        ← classified as supplement (matches default list)
```

The check runs only at file creation. Existing main-file PDFs whose names happen to match the list are not retroactively reclassified. To change which names trigger classification, see the [`pdf_supplement_filenames` setting](./configuration#supplement-discovery).

## Working with Supplements

Supplements appear on the book detail page alongside the main files. You can:

- **Download** any supplement file
- **Rename** the display name
- **Promote** a supplement to a main file if it should be treated as a primary format
- **Demote** a main file to a supplement if it shouldn't be a primary format

:::tip
Promoting a supplement to a main file will trigger [metadata extraction](./metadata#how-metadata-is-extracted) for that file. Demoting a main file to a supplement clears its format-specific metadata (cover, chapters, audiobook data, etc.).
:::
