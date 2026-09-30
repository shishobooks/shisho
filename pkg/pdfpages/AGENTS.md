# PDF Page Cache

This package renders and caches PDF pages as JPEG images for the PDF viewer.

## Cache Path Format

```
{cacheDir}/pdf/{fileID}/{dpi}-{quality}/page_{N}.jpg
```

Where `N` is the 0-indexed page number and `{dpi}-{quality}` is `RenderKey(dpi, quality)`. Keying on the render settings means a restart with a new `pdf_render_dpi` or `pdf_render_quality` renders fresh pages instead of serving old ones. Pages rendered at earlier settings stay under their own key until `Invalidate` (which removes the whole `{fileID}` directory) or `Clear` runs. The same key reaches the browser: `GET /auth/status` reports it as `pdf_render_key`, and the frontend adds it to PDF page URLs as `&r=`, so the `immutable` browser cache never serves a page from the old settings either. The page handler serves a PDF page whose `r` differs from `Cache.RenderKey()` (a tab loaded before the restart) as `private, no-store`, so the new render is never cached under the old key.

## Thread Safety

Concurrent calls to `renderPage` are serialized by the pdfium pool's `MaxTotal: 1` configuration (set in `pkg/pdf/cover.go`). When multiple goroutines call `renderPage` simultaneously, they queue at `GetInstance`, so no explicit mutex is needed in this package. `GetPage` checks for the page file without taking an instance, so `renderPage` writes the JPEG with `fileutils.WriteFileAtomic`; a concurrent request never serves a partial page. `renderPage` waits `pdf.InteractivePdfiumTimeout` (30s) for the instance, since the reader is interactive; Scans wait longer (see "Shared Pdfium Pool" in `pkg/pdf/AGENTS.md`).

**`Clear()` vs in-flight `GetPage()`:** `Clear()` removes the entire `{cacheDir}/pdf/` tree via `os.RemoveAll`. If an admin triggers a clear while a `GetPage` call is mid-`renderPage`, the `os.MkdirAll(pageDir)` → temp write → rename sequence in `renderPage` can race the removal and fail with `ENOENT`. The in-flight request returns an error; the next attempt recreates the directory and succeeds. This is acceptable for an admin-initiated operation but callers should not assume `Clear()` is transparent to concurrent readers.

## Configuration

DPI and JPEG quality are configurable via server config:

- `config.PDFRenderDPI`: controls render resolution (higher = sharper, slower)
- `config.PDFRenderQuality`: controls JPEG compression quality (1 to 100)

## Key Functions

```go
// NewCache creates a cache with the given base directory and render settings.
func NewCache(dir string, dpi int, quality int) *Cache

// GetPage returns the path to a cached page image, rendering if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error)

// RenderKey identifies render settings as "{dpi}-{quality}".
func RenderKey(dpi int, quality int) string

// Invalidate removes all cached pages for a file, at every render setting.
func (c *Cache) Invalidate(fileID int) error

// SizeBytes returns the total bytes and file count under the cache root ({dir}/pdf).
// A missing root is treated as empty.
func (c *Cache) SizeBytes() (int64, int, error)

// Clear removes the cache root directory entirely. Safe when missing.
func (c *Cache) Clear() error
```

## Invalidation

The cache is keyed by file ID, render settings, and page, so it does not notice when a file is replaced on disk. The scan calls `Invalidate` (through `invalidatePageCaches` in `pkg/worker/scan_unified.go`) for both this cache and `cbzpages` when a file's size or mtime changed, or on a refresh or reset. This covers supplements too, since they can be opened in the reader. A failed invalidation is logged as a warning and the scan continues. The worker gets these instances from `cmd/api/main.go`, the same ones `server.New` receives. The frontend keys page URLs on `file.updated_at`, which the same scan bumps.

`Invalidate` is a plain `os.RemoveAll`, so it has the same race with an in-flight `GetPage` as `Clear()` above: that request can fail once with `ENOENT`, and a render that opened the old file before the swap can write one old page back after the invalidation. The window is a single page render, so it is accepted rather than locked.

## Relationship to cbzpages

This package mirrors the same pattern as `pkg/cbzpages`: a `Cache` struct with `NewCache`, `GetPage`, and `Invalidate`. The difference is that CBZ pages are extracted directly from the ZIP archive (no rendering needed), while PDF pages must be rendered via go-pdfium WASM.

## Related Files

- `pkg/pdfpages/cache.go`: Cache implementation
- `pkg/pdf/cover.go`: pdfium pool initialization (`MaxTotal: 1`)
- `pkg/cbzpages/cache.go`: CBZ page cache (same pattern)
