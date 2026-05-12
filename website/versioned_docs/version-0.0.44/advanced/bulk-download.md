---
sidebar_position: 2
---

# Bulk Download

Shisho supports downloading multiple books at once as a zip file. This is useful when you want to transfer a batch of books to another device or backup a selection of your library.

## How It Works

1. **Enter select mode** in the library gallery by long-pressing a book or using the selection toggle.
2. **Select the books** you want to download — you can select across pages and use Shift+click for range selection.
3. **Click the Download button** in the selection toolbar at the bottom of the screen. The button shows the estimated total file size.
4. Shisho creates a background job to prepare your download. You'll see a progress toast showing how many files have been processed.
5. **Navigate freely** while the download is being prepared — the progress toast persists across pages and will notify you when the download is ready.
6. Once complete, click **Download Zip** to save the file.

## What's Included

- Each selected book contributes its **primary file** (the file designated for downloads and device sync).
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
