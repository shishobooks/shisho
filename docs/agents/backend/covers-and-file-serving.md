# Covers and File Serving

Covers: storage, path resolution, writes, and conditional GET. File serving: `httputil.ServeFile`, `Cache-Control` policy, downloads, and fallback.

## Cover storage

A file's cover is `{filename}.cover.{ext}` next to the file. `file.CoverImageFilename` stores **the filename only** (`MyBook.cbz.cover.jpg`), never a path; handlers join it with the file's directory at runtime, so a stored path doubles into `/path/to/path/to/cover.jpg`. When renaming, store the base name:

```go
newCoverPath := filepath.Base(fileutils.ComputeNewCoverPath(*file.CoverImageFilename, newPath))
file.CoverImageFilename = &newCoverPath
```

**Resolve cover paths from the file, never the book.** `book.Filepath` can be a synthetic organized-folder path that does not exist on disk (root-level books in libraries with `OrganizeFileStructure` off; see `scanFileCreateNew`).

- **Read side** (serving, fingerprinting, generation): `covers.FileCoverPath(file)`, never an inline join. It falls back to `<file>.cover<ext>` when `CoverImageFilename` is empty, so a caller that must skip coverless files checks that first.
- **Book-cover serving** in the books, series, OPDS, and eReader handlers goes through `covers.ServeBookCover` (selection, missing-cover 404, ETag). The series handler passes its first book's files and `"Series cover"`; every other caller passes `"Cover"`. Selection is `covers.SelectFile`, which must keep falling back to any available cover when the preferred aspect ratio has none, so an audiobook-only book still shows a cover on an eReader.
- **Web thumbnails**: one `covers.ThumbnailCache`, built in `server.New`, is shared by book, file, series, and Share Link covers and by Settings > Cache. API handlers pass it to `covers.ServeBookCover` or `covers.ServeFileCover` after authorization; device callers (OPDS, eReader, Kobo) pass none and serve originals. Request parameters, bounds, publication, and invalidation are in `pkg/covers/AGENTS.md`.
- **Write side** (scanner, pre-organize): `fileutils.ResolveCoverDirForWrite(bookFilepath, fileFilepath)` when the book path may not exist yet; it falls back to the file's directory.
- Book sidecars are anchored the same way: `sidecar.WriteBookSidecarFromModel(book)` falls back to `book.Files[0].Filepath` when `book.Filepath` is not a real directory, and `sidecar.ReadBookSidecarFromModel(book, fileHint)` takes the current file so it resolves before `Files` is loaded.

## Cover writes never destroy a working cover

1. Write the replacement with `fileutils.WriteFileAtomic` (temp plus rename, 0644).
2. Collect older covers at other extensions with `fileutils.OtherCoverExtensions`.
3. Update the row, then remove the stale covers with `books.RemoveStaleCovers` only after that write succeeds.

Extractors (`books.ExtractCoverPageToFile`, the scanner's `extractCBZPageCover` and `extractPDFPageCover`) return the stale list instead of deleting; whoever owns the `UpdateFile` owns the removal. For bytes from outside the file (enricher, download, upload), the validity gate is `fileutils.NormalizeImage`'s error, which decodes every pixel. `fileutils.ImageResolution` reads only the header and accepts a truncated body, so it is for comparing resolutions. Embedded covers from the file's own parser are still written best effort when they do not decode.

## Conditional GET

- `/files/:id/cover` relies on `http.ServeContent`'s `Last-Modified`, which is enough because the URL pins the file.
- `/books/:id/cover`, its OPDS and eReader mirrors, and `/series/:id/cover` serve a *selected* file, and the selection can change without the new file's mtime changing (aspect ratio flip on a hybrid book, a file removed, a series' first book changing). `ServeBookCover` therefore sends `ETag: "<file_id>-<mtime_unix>"` through `httputil.WithETag`, which omits `Last-Modified` so `If-Modified-Since` cannot shortcut to a stale 304.

## `httputil.ServeFile`

Every route that serves a file from disk calls `httputil.ServeFile(c, path, errcodes.NotFound(resource), opts...)` (forbidigo rejects `c.File`, `c.Attachment`, `c.Inline`, and `http.ServeFile`, which turn every open failure into a 404). It opens and stats first: missing file or directory returns the `notFound` error, anything else (`EACCES`, `EIO`) is a 500. Only then does it set headers.

**Set no `Cache-Control`, `Content-Type`, `Content-Disposition`, or `ETag` before the open.** Echo's error handler keeps preset headers, so a failure would go out cacheable for a year, typed `image/jpeg`, or as an attachment. A route that builds its own body (the Kobo cover resize) sets cache headers only after the image decodes.

Options (`WithCacheControl`, `WithContentType`, `WithAttachment`, `WithETag`) are in `pkg/httputil`. Book files pass an explicit content type (`models.FileTypeMimeType`, `books.KepubContentType`) because the Alpine image's mime table has no entry for `.epub`, `.cbz`, or `.m4b`.

Before serving bytes, check existence with `books.RequireFileOnDisk(c, file, resource)`, not an inline `os.Stat`; it returns `NotFound(resource)` and warn-logs the ID and path (not on HEAD). The page handler checks before the page cache lookup so a cached page is not served for a missing source. The generated-download and KePub handlers pass `"Source file"`; everything else passes `"File"`.

Ranges are `http.ServeContent`'s job, including the audio stream: never parse `Range` by hand. Malformed bytes ranges are 416, suffix ranges 206, a non-`bytes` unit is ignored with a 200 (RFC 9110 section 14.2).

Each serving route has a test that `chmod 000`s the file and asserts a 500 with a JSON body and no success headers (`pkg/books/handlers_file_faults_test.go` is the model). A new serving route adds one.

## Cache-Control policy

Every file-serving response carries `Cache-Control`; without it, reverse proxies heuristically cache anything with `Last-Modified` and keep serving stale content after Shisho's caches clear.

| Endpoints | Header | Why |
|---|---|---|
| API covers (`/api/*/cover`) | `private, max-age=31536000, immutable` | URL changes through `?v=cover_cache_key` |
| External covers (OPDS, eReader, Kobo) | `private, no-cache` | Clients have no `?v=`; revalidate by `Last-Modified`/`ETag` |
| Downloads and streams | `private, no-store` | Generated files change without the URL changing |
| Page endpoint (`getPage`) | `private, max-age=31536000, immutable` | Keyed on `?v=file.updated_at`, plus `&r=` (`pdfpages.RenderKey`) for PDF |
| Plugin icon | `private, no-cache` | No version in the URL; changes on plugin update |

A PDF page requested with a stale `r` is served `private, no-store` so a new render is never cached under the old key. A rescan that finds the file changed bumps `updated_at` and drops its cached pages; new render settings change the render key, and the page cache stores each setting in its own directory.

## Downloads and fallback

Book downloads use `books.Download.Serve(c, opts...)`, which sets the media type, attachment name, and `private, no-store` (the jobs bulk zip calls `ServeFile` with `application/zip`). KePub is served as `books.KepubContentType` (`application/epub+zip`) on every route but Kobo, which sends `application/octet-stream` with a name from `FormatKepubDownloadFilename`, even though the OPDS KePub feed advertises `application/kepub+zip`.

**Device and recipient routes fall back to the original; web routes do not.** OPDS, eReader, Kobo, and Share Link downloads choose what to send with `books.ResolveFallbackDownload`, because a device or recipient has no Download Original. It sends the original for a supplement, a type with no generator (`filegen.ErrNotImplemented`, checked before the download cache so a full cache cannot fail it), a KePub request for an unconvertible type (`filegen.ErrKepubNotSupported`), and any other `filegen.GenerationError` (warn-logged). A wrapped `fs.ErrPermission`, a canceled or timed-out request, and errors outside generation stay 500, and an original that cannot be opened is a 500. The web app's download routes return 422 (`InvalidState`) for no generator or unconvertible KePub and 500 for other generation failures, since the user sees the error and has Download Original.

## Committed responses

`errcodes.Handler` writes nothing once the response is committed (a stream cut off, the Kobo resize failing after `WriteHeader`), so JSON is never appended to a partial body. It treats `EPIPE`, `ECONNRESET`, `io.EOF`, `io.ErrUnexpectedEOF`, and network timeouts as a departed client only when the response is committed or the request context is canceled; on a live uncommitted request they are a 500, so a truncated archive cannot send an empty 200.
