# Covers and File Serving

## Cover paths

- **`file.CoverImageFilename` stores the filename only**, never a path; readers join it with the file's directory, so a stored path doubles. When renaming, store `filepath.Base` of the new cover path.
- **Resolve cover paths from the file, never the book.** `book.Filepath` can be a synthetic organized-folder path that does not exist on disk. Read with `covers.FileCoverPath(file)`, never an inline join; write with `fileutils.ResolveCoverDirForWrite` when the book path may not exist yet.
- Serve book and series covers through `covers.ServeBookCover`, and file covers through `covers.ServeFileCover`; see `pkg/covers/AGENTS.md`.

## Cover writes never destroy a working cover

1. Write the replacement with `fileutils.WriteFileAtomic`.
2. Collect older covers at other extensions with `fileutils.OtherCoverExtensions`.
3. Update the row, then remove the stale covers with `books.RemoveStaleCovers` only after that write succeeds. Whoever owns the `UpdateFile` owns the removal, so extractors return the stale list instead of deleting.

For image bytes from outside the file (enricher, download, upload), the validity gate is `fileutils.NormalizeImage`'s error, which decodes every pixel. `fileutils.ImageResolution` reads only the header and accepts a truncated body, so it only compares resolutions.

## Serving file bytes

- **Every route that serves a file from disk calls `httputil.ServeFile`** (forbidigo enforces it) and sets no `Cache-Control`, `Content-Type`, `Content-Disposition`, or `ETag` before it: Echo's error handler keeps preset headers, so a failure would go out cacheable, typed as an image, or as an attachment. A route that builds its own body sets headers only after the body is ready.
- **Book files pass an explicit content type** (`models.FileTypeMimeType`), because the Alpine image's mime table has no entry for most book formats.
- **Check existence with `books.RequireFileOnDisk`**, not an inline `os.Stat`, before serving or before a cache lookup keyed on the source.
- **Never parse `Range` by hand**; `http.ServeContent` handles it.
- **Each serving route has a test that `chmod 000`s the file** and asserts a 500 with a JSON body and no success headers. `pkg/books/handlers_file_faults_test.go` is the model.

## Cache-Control

Every file-serving response carries `Cache-Control`; without it, reverse proxies heuristically cache anything with `Last-Modified` and keep serving stale content after Shisho's caches clear. All are `private`. A URL versioned with a `?v=` key is immutable for a year; an unversioned URL (device covers, plugin icons) is `no-cache` so it revalidates; generated downloads and streams are `no-store`, since they change without the URL changing.

## Downloads

Book downloads go through `books.Download.Serve`. **Device and Share Link downloads fall back to the original; web routes do not.** OPDS, eReader, Kobo, and Share Link routes choose what to send with `books.ResolveFallbackDownload`, because a device or recipient has no Download Original. The web app's download routes report the generation error instead.
