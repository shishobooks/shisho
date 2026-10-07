# PDF Page Cache

Renders PDF pages to JPEG for the reader and caches them at `{cacheDir}/pdf/{fileID}/{RenderKey(dpi, quality)}/page_{N}.jpg` (N 0-indexed). It mirrors `pkg/cbzpages` (`NewCache`, `GetPage`, `Invalidate`, `ErrPageOutOfRange`), except PDF pages are rendered through the shared PDFium pool in `pkg/pdf` instead of read from a ZIP. Render settings come from `pdf_render_dpi` and `pdf_render_quality`.

## The render key reaches the browser

Keying on `{dpi}-{quality}` means a restart with new settings renders fresh pages; old renders stay until `Invalidate` or `Clear`. `GET /auth/status` reports the key as `pdf_render_key` and the frontend adds it to page URLs as `&r=`, so the `immutable` browser cache never shows a page from old settings. The page handler serves a page whose `r` differs from `Cache.RenderKey()` (a tab opened before the restart) as `private, no-store`, so the new render is never cached under the old key. `ErrPageOutOfRange` becomes `NotFound("Page")`, since the stored page count can be missing.

## Thread Safety

- No mutex here: the pool's `MaxTotal: 1` serializes `renderPage`. It waits `pdf.InteractivePdfiumTimeout`, since the reader is interactive (timeouts are explained under "Shared PDFium pool" in `pkg/pdf/AGENTS.md`).
- `GetPage` checks for the file without taking an instance, so `renderPage` must write with `fileutils.WriteFileAtomic`; a concurrent request never serves a partial page.
- `Clear` and `Invalidate` are plain `os.RemoveAll`. An in-flight render can fail once with `ENOENT`, and a render that opened the old file before a swap can write one stale page back after invalidation. The window is one page render, so it is accepted rather than locked; callers should not assume either call is invisible to concurrent readers.

## Invalidation

The cache does not notice a file replaced on disk. The scan calls `Invalidate` on this cache and `cbzpages` (`invalidatePageCaches` in `pkg/worker/scan_unified.go`) when a file's size or mtime changed and on refresh or reset, supplements included; a failure is logged and the scan continues. The worker and `server.New` share the same cache instances from `cmd/api/main.go`. The frontend keys page URLs on `file.updated_at`, which the same scan bumps.
