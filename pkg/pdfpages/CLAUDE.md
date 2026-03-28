# PDF Page Cache

This package renders and caches PDF pages as JPEG images for the PDF viewer.

## Cache Path Format

```
{cacheDir}/pdf/{fileID}/page_{N}.jpg
```

Where `N` is the 0-indexed page number.

## Thread Safety

Concurrent calls to `renderPage` are serialized by the pdfium pool's `MaxTotal: 1` configuration (set in `pkg/pdf/cover.go`). When multiple goroutines call `renderPage` simultaneously, they queue at `GetInstance` — so no explicit mutex is needed in this package.

## Configuration

DPI and JPEG quality are configurable via server config:

- `config.PDFRenderDPI` — controls render resolution (higher = sharper, slower)
- `config.PDFRenderQuality` — controls JPEG compression quality (1–100)

## Key Functions

```go
// NewCache creates a cache with the given base directory and render settings.
func NewCache(dir string, dpi int, quality int) *Cache

// GetPage returns the path to a cached page image, rendering if necessary.
// pageNum is 0-indexed.
func (c *Cache) GetPage(pdfPath string, fileID int, pageNum int) (cachedPath string, mimeType string, err error)

// Invalidate removes all cached pages for a file.
func (c *Cache) Invalidate(fileID int) error
```

## Relationship to cbzpages

This package mirrors the same pattern as `pkg/cbzpages`: a `Cache` struct with `NewCache`, `GetPage`, and `Invalidate`. The difference is that CBZ pages are extracted directly from the ZIP archive (no rendering needed), while PDF pages must be rendered via go-pdfium WASM.

## Related Files

- `pkg/pdfpages/cache.go` — Cache implementation
- `pkg/pdf/cover.go` — pdfium pool initialization (`MaxTotal: 1`)
- `pkg/cbzpages/cache.go` — CBZ page cache (same pattern)
