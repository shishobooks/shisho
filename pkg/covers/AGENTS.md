# Cover Serving and Thumbnails

`ServeBookCover` selects the cover using the library preference. `ServeFileCover` serves a specific file. Callers must authorize the book, file, library, or Share Link before calling either helper. No thumbnail parameter bypasses authorization.

API callers pass the one `ThumbnailCache` built in `server.New`. The same instance is injected into books, series, public Share Links, and cache management. Do not build independent instances for the same cache directory: request deduplication, resize concurrency, the LRU index, and cache clearing belong to the instance. OPDS, eReader, and Kobo retain their original cover behavior.

## Thumbnail Requests

- No `size` means the original cover. `size` is the maximum dimension in pixels, limited to the tiers declared in `types.go`.
- `aspect=book` applies a centered 2:3 crop; `aspect=square` applies a centered square crop. Without `aspect`, the original proportions are preserved. Cropping matches the web UI's `object-cover` and prevents a wide source from becoming an undersized thumbnail in a tall frame. Never upscale the source.
- `r` is the generated `ThumbnailRenderKey`. Requests with an absent or stale render key use `private, no-store`, so current bytes are not cached under a previous renderer's URL. Bump this key when changing resize or encoding settings.
- Go owns `CoverThumbnailQuery`, tiers, aspects, and render key in `types.go`. Tygo emits them to `app/types/generated/covers.ts`.
- Errors must not carry success image or cache headers. Generation and cache reads happen before setting headers. Thumbnail ETags identify the source version, size, and aspect. `http.ServeContent` handles conditional requests and ranges on the complete in-memory PNG.

## Lazy Cache

`ThumbnailCache` writes only requested variants under `CACHE_DIR/cover-thumbnails`. PNG preserves the resampled detail and transparency. Catmull-Rom averages fine detail during reduction rather than dropping source pixels. Raster decoding is bounded to 32 million source pixels; unsupported or excessive images fall back to the original with `private, no-store`.

Source identity includes path, file ID, nanosecond mtime, byte size, and render version. Metadata-only `updated_at` changes do not regenerate thumbnails. A successful replacement removes cached older versions of that file. Unvisited deleted-file entries remain eligible for eviction.

Two resizes can run at once. Duplicate cold requests for a variant wait for one generation; canceled waiters return without canceling other requests. Check cancellation between decode, resize, encode, and publication. Cache reads do not wait behind resizing. Files are published with `fileutils.WriteFileAtomic` so partially written PNGs cannot be served.

The disk index is loaded once and reused. LRU order changes on every hit; approximate recency is persisted at most once per minute for restarts. The cache is bounded to `DefaultThumbnailCacheMaxBytes`, currently 256 MiB. Returned bytes remain valid across eviction or an admin clear. Clearing increments an epoch so in-flight work cannot repopulate the cache after the clear completes. A later request can generate a fresh variant.
