---
sidebar_position: 50
---

# Cache Management

Shisho maintains three on-disk caches to speed up common operations. All three live under the directory set by the [`cache_dir`](./configuration.md#cache) config option (default `/config/cache`).

| Cache | What it stores | Notes |
|-------|----------------|-------|
| **Downloads** | Generated format conversions (e.g. kepub), files produced by plugins, and bulk-download zips. | Size is capped by [`download_cache_max_size_gb`](./configuration.md#cache) and evicted LRU-style automatically. |
| **CBZ Pages** | Page images extracted from CBZ files for the in-app reader. | Avoids re-extracting pages every time a CBZ is opened. |
| **PDF Pages** | JPEGs rendered from PDF pages for the in-app reader. | Avoids re-rendering pages; can grow large on image-heavy PDFs. |

## Viewing cache usage

Admins can view the current size and file count of each cache at **Settings → Cache** (`/settings/cache`). The page requires the `config:read` permission.

## Clearing caches

Each cache has its own **Clear** button. Clearing is safe — content is regenerated on next access — but can temporarily slow down affected operations (downloads, the PDF reader, etc.) while the cache rebuilds.

Clearing requires the `config:write` permission (admin-only by default). A confirmation dialog shows the number of files and total size that will be deleted before the action is performed.

## When to clear

- **Downloads**: reclaim disk space after removing a plugin whose generated files should not be reused.
- **CBZ Pages / PDF Pages**: force the reader to re-extract or re-render after changing a config option that affects output (e.g. `pdf_render_dpi` or `pdf_render_quality`).

See also: [Configuration](./configuration.md), [Users and Permissions](./users-and-permissions.md).
