# Supplement Files

Supplement files are additional files associated with a book, such as companion guides, liner notes, maps, artwork, and errata. Shisho discovers them during [library scans](./libraries.md).

## Main Files and Supplements

Shisho's native main-file formats are EPUB, CBZ, M4B, and PDF. CBR is not native unless a [plugin](./plugins/overview.md) adds support. Files that are not recognized as main files can be linked to a nearby book as supplements.

A supplement's display name is derived from its filename. Shisho does not extract embedded book or file metadata from supplements.

## Directory-Based Discovery

For a book stored in its own directory, Shisho recursively links eligible non-main files anywhere below that book directory:

```text
[Author] Book Title/
├── book.epub
├── companion-guide.txt
├── notes.txt
└── extras/
    ├── map.jpg
    └── appendix.txt
```

All four files other than `book.epub` are candidates for supplement discovery.

:::warning
Directory grouping is intentionally broad. If one directory contains unrelated books or general-purpose files, those files may all be attached to the same book. Keep each directory limited to one logical book and its extras.
:::

## Root-Level Discovery

For a main file stored directly at a configured library root, Shisho checks files in that same root. A candidate matches when its basename is equal to, or starts with, the main file's basename.

```text
library/
├── My Book.m4b
├── My Book.pdf
├── My Book - Notes.txt
├── My Booklet.jpg
└── Other Book.txt
```

`My Book - Notes.txt` and `My Booklet.jpg` match the `My Book.m4b` prefix and become supplements. `Other Book.txt` does not match. `My Book.pdf` remains a native main file rather than becoming a supplement because main-file extensions are excluded before prefix matching.

Prefix matching is literal and can be broader than expected. For example, `My Booklet.jpg` matches `My Book`. Use distinctive main filenames or put the book in its own directory if similarly named files should remain separate.

## Exclusions

Discovery skips:

- Native main-file extensions and files already tracked by Shisho as main files.
- Shisho cover files and [Sidecar Metadata Files](./sidecar-files.md).
- Files matching `supplement_exclude_patterns`.
- Directories when evaluating root-level candidates.

The default exclusion patterns cover hidden files and common operating-system files. Configure exceptions and additional globs in [Configuration](./configuration.md#supplement-discovery) instead of relying on repeated manual demotion.

## PDF Auto-Demotion

A newly created PDF is automatically imported as a supplement only when all of these conditions are true:

1. It is part of a directory-based book, not a file at a configured library root.
2. Its basename is an exact, case-insensitive match for a configured `pdf_supplement_filenames` value.
3. The same book directory already has a Shisho book or contains a non-PDF main file, such as EPUB, CBZ, M4B, or a plugin-registered format.

For example, `Supplement.pdf` can be auto-demoted beside `Book.epub` when `supplement` is configured. `My Supplement.pdf` is not an exact match for `supplement`.

The rule runs only when the PDF is first created in Shisho. Existing PDFs are not reclassified by later scans. A root-level PDF remains a main PDF even when its basename matches the configured list.

## Managing Supplements

Changing file roles requires **Books Write** permission and access to the book's library. See [Users and Permissions](./users-and-permissions.md).

:::warning
Demoting a main file clears its extracted format metadata, including its cover and chapters, but does not delete the media file from disk. Verify that the metadata can be recovered from the source or a backup before demoting. Leave the file as main if you only need to download it normally.
:::

You can:

- Download a supplement.
- Rename its display name.
- **Promote** a supported supplement to a main file. Promotion triggers metadata extraction for that format.
- **Demote** a main file to a supplement. Demotion clears its format-specific metadata.

Supplements are excluded from Kobo Sync and normal bulk-download selection. They are also excluded from surfaces that distribute main reading files, even if the supplement itself uses EPUB, CBZ, M4B, or PDF.

## Troubleshooting

### A File Was Not Discovered

**Symptom:** An expected supplement does not appear on the book.

**Likely cause:** It is outside the book directory, its root-level basename does not match, an exclusion pattern matched it, or Shisho already treats it as a main file.

**Verify:** Compare its location and basename with the discovery rules above, then review `supplement_exclude_patterns` and its current file role.

**Fix:** Move or rename the file to match the intended book, adjust the exclusion configuration if appropriate, and rescan.

### An Unrelated File Was Attached

**Symptom:** A book shows a supplement that belongs elsewhere.

**Likely cause:** The book directory contains unrelated files or a root-level filename shares a broad prefix.

**Verify:** Check the candidate's directory and compare its basename with the main file's basename.

**Fix:** Separate books into distinct directories or rename root-level files to avoid the shared prefix, then rescan.

### A PDF Has the Wrong Role

**Symptom:** A companion PDF stayed main, or a new PDF became a supplement unexpectedly.

**Likely cause:** Auto-demotion depends on creation time, directory placement, an exact configured basename, and an appropriate sibling.

**Verify:** Check whether the PDF was newly created in a directory-based book, then compare its exact basename with `pdf_supplement_filenames` and inspect the other recognized files in that directory tree.

**Fix:** Promote or demote the file manually for the current book. Adjust `pdf_supplement_filenames` only if the automatic rule should change for future files. See [Libraries, Scanning, and File Organization](./libraries.md), [Configuration](./configuration.md), and [Troubleshooting](./troubleshooting.md).
