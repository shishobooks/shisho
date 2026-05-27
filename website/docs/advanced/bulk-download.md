---
sidebar_position: 2
---

# Bulk Download

Shisho supports downloading multiple books at once as a zip file. This is useful when you want to transfer a batch of books to another device or backup a selection of your library.

## How It Works

1. **Enter select mode** in the library gallery by long-pressing a book or using the selection toggle.
2. **Select the books** you want to download — you can select across pages and use Shift+click for range selection.
3. **Click the Download button** in the selection toolbar at the bottom of the screen.
4. **Choose which file types to include** (e.g., EPUB, M4B, CBZ, PDF). All available types are selected by default. The file count and total size update as you toggle types.
5. **Click Download** to start. Shisho creates a background job to prepare your zip. You'll see a progress toast showing how many files have been processed.
6. **Navigate freely** while the download is being prepared — the progress toast persists across pages and will notify you when the download is ready.
7. Once complete, click **Download Zip** to save the file.

## File Type Selection

When you click the Download button, a popover shows checkboxes for each file type (EPUB, M4B, CBZ, PDF). Types that don't exist among the selected books' main files are disabled.

- **All available types are selected by default** — uncheck any types you don't need.
- **Multiple files of the same type** from a single book are all included (e.g., if a book has two EPUBs, both are downloaded).
- **Supplement files are excluded** — only main files are included in the download, regardless of type selection.
- The total file count and size shown in the popover reflect your current type selection.

## What's Included

- Each selected book contributes all **main files** matching your selected file types. Supplement files are always excluded.
- Files include **embedded metadata** — the same enriched version you get from individual downloads, with title, authors, cover art, and other metadata written into the file.
- The zip uses **store mode** (no compression) since ebook formats like EPUB and CBZ are already compressed internally. This makes the download faster and the file size estimate accurate.

## Caching

Bulk downloads are cached to speed up repeated requests:

- **Individual files** generated during a bulk download are cached in the download cache. Future single-file downloads of the same books will be served instantly from this cache.
- **The zip file itself** is cached based on a fingerprint of the selected files and their metadata. If you request the same set of books again with no metadata changes, the cached zip is served immediately.

Cache is managed automatically and respects the configured download cache size limit.

## Permissions

Bulk download requires:
- **Authentication** — you must be logged in.
- **books:read** permission — granted to all default roles (Admin, Editor, Viewer).
- **Library access** — you must have access to the libraries containing the selected books.
