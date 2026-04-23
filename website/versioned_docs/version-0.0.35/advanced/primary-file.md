---
sidebar_position: 1
---

# Primary File

When a book has multiple files — for example, an EPUB and a PDF, or multiple EPUB editions — Shisho designates one as the **primary file**. The primary file is the one used for syncing to devices (via Kobo sync) and for downloads from the eReader browser.

## Why It Matters

Without a primary file, a book with both an EPUB and a PDF would sync both to your e-reader, creating duplicate entries. The primary file system ensures only one file per book is sent to devices while keeping all editions accessible in the web UI.

## Automatic Selection

Shisho handles primary file selection automatically in most cases:

- **First file added** — When a book is created with its first file, that file becomes the primary.
- **Additional files** — Adding more files to a book does not change the existing primary.
- **Primary deleted** — If the primary file is deleted, Shisho promotes another file automatically. It prefers main files over supplements, and picks the oldest file first.
- **Files moved between books** — When files are moved to a different book, the primary is updated if needed for both the source and destination books.

## Manual Selection

You can manually change the primary file from the book detail page:

1. Open the book in the web UI.
2. Click the **...** menu on the file you want to set as primary.
3. Select **Set as primary**.

The primary file is indicated with a star badge next to its name. This badge only appears when the book has more than one file — if a book has a single file, no badge is shown since there is no ambiguity.

## Where the Primary File Is Used

| Feature | Behavior |
|---------|----------|
| **Kobo sync** | Only the primary file is synced to Kobo e-readers |
| **eReader browser** | The primary file is used for downloads and determining the book's file type |
