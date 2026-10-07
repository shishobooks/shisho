# PDF Format

`pkg/pdf` reads metadata (pdfcpu, `pdf.go`), the cover (`cover.go`), and the outline (`outline.go`, PDFium); `pkg/filegen/pdf.go` writes downloads. The info-dict-to-field mapping lives in `pdf.go`. PDFs are page-based: page numbers are 0-indexed everywhere except pdfcpu's bookmark API.

## Parsing

- **Language comes from the catalog `Lang`, not the info dict.** pdfcpu exposes no field for it; read it from `xrt.RootDict` after `api.ReadAndValidate`.
- `Author` is one string split on `,`, `&`, and `;`; PDF authors have no role. `Keywords` become tags, `Subject` the description, `CreationDate` the release date (pdfcpu `types.DateTime` in relaxed mode plus fallback formats).
- **`CoverPage` is always 0, even when cover extraction fails.** The file type (`models.IsPageBasedFileType`) is the guard that blocks external covers; `CoverPage` is only page-selection data. Never set it conditionally.
- **Cover extraction and outline extraction are best-effort**: a failure logs a warning and `Parse` returns metadata without that piece. The cover is the largest embedded image on page 1 (pdfcpu `ExtractImagesRaw`), else page 0 rendered by PDFium at 150 DPI, JPEG quality 85.
- **Outlines are read with PDFium (`GetBookmarks`), never pdfcpu.** pdfcpu refuses outlines containing `/Dest [null ...]`, which some tooling writes on most bookmarks. The tree is flattened (`flattenBookmarks`); a bookmark with no `DestInfo` or a `PageIndex` of -1 is skipped, but its children are still walked because they can point at real pages.

## pdfcpu config is disabled on purpose

pdfcpu's `NewDefaultConfiguration()` reads or creates `$HOME/.config/pdfcpu/config.yml`, which is not safe across processes: `go test` runs `pkg/pdf` and `pkg/filegen` binaries concurrently, one can read the file mid-write, and pdfcpu panics with `config problem: EOF`. The package `init()` sets `model.ConfigPath = "disable"` so the configuration is in-memory only. Nothing needs the on-disk config: `ValidationMode` is set at each call site, and Shisho never fills forms or validates signatures. `EnsurePdfcpuInit()` is a remaining `sync.Once` for the in-process case.

## Writing downloads (`pkg/filegen/pdf.go`)

- **Info properties go through `writeInfoProperties`** (`api.ReadValidateAndOptimize`, `pdfcpu.PropertiesAdd`, `api.Write`). Since pdfcpu 0.14, `api.AddPropertiesFile` rejects `Keywords`, `Producer`, `CreationDate`, `ModDate`, and `Trapped`, and `api.AddKeywordsFile` merges with the source keywords, joins with `"; "`, and strips XMP keywords; use neither. The release date is not written because pdfcpu always overwrites `CreationDate` with the current time.
- **Chapters are written as bookmarks** with `api.AddBookmarksFile` (`replace=true`), pages converted to 1-indexed, through a sibling `.bookmarks.tmp` file renamed over the destination (pdfcpu needs distinct input and output paths). Empty `file.Chapters` skips the write so source bookmarks survive. A failure logs a warning with `category=pdf_bookmark_write` and returns the properties-only file, so a chapter quirk never blocks a download.
- **pdfcpu rejects out-of-order bookmark trees**, so `convertModelChaptersToPDFBookmarks` drops chapters with a nil, negative, or out-of-range page and children that start before their parent (with their whole subtree, no re-parenting), and sorts siblings by page, then `SortOrder`. Do not trust database order: plugins, sidecars, and API callers can store any order.

## Shared PDFium pool

One lazily initialized go-pdfium WASM pool with `MaxTotal: 1` (`cover.go`) serves cover and outline extraction, Scan page-cover rendering (`RenderPageJPEG`), and the reader page cache (`pkg/pdfpages`). Get an instance with `PdfiumInstance(timeout)` and `defer instance.Close()`. One instance is deliberate: each holds its own PDFium memory, many installs run on small NAS hardware, so raise it only after measuring per-instance memory and seeing real contention in the warn logs.

**Pick the timeout for where the caller runs; never change a shared default.**

| Timeout | Value | Callers |
|---------|-------|---------|
| `InteractivePdfiumTimeout` | 30s | `pkg/pdfpages` (reader pages), the cover-page picker (`books.ExtractCoverPageToFile`), Identify `cover_page` (`books.PluginPageExtractor`) |
| `scanPdfiumTimeout` | 5m | everything inside `pkg/pdf`: `Parse` (cover and outline), `RenderPageJPEG` |

Scans parse files in parallel, so large PDFs can hold the instance past 30 seconds; a short wait used to store a File with no cover or chapters, and unchanged Files are skipped by later Scans, so the loss stuck. Consequences of the long wait: handlers that run a Scan in the request (`resyncFile`, `resyncBook`, and `deleteFile` when it promotes a supplement) can take minutes and keep going after the client disconnects, and `Parse` waits separately for cover and outline, so one PDF can wait twice the Scan timeout. An interactive caller of `pkg/pdf` needs a variant that takes a timeout.

## Tests

- Fixtures are raw PDF bytes built in `TestMain`, because pdfcpu's write path overwrites `CreationDate`, `ModDate`, and `Producer`. `with-image.pdf` embeds a DCTDecode JPEG on page 1 for the embedded-cover path.
- Tests that hold the PDFium instance or change package state (`scanPdfiumTimeout`, the logger) must not call `t.Parallel()`: the pool, the variable, and the logger output are process-global (`pool_test.go`).
