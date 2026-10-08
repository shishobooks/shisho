// Package covers centralizes cover-file selection and serving so that the
// books, series, ereader, and OPDS handlers don't drift independently.
package covers

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

const (
	CacheControlImmutable = "private, max-age=31536000, immutable"
	CacheControlNoCache   = "private, no-cache"
)

// SelectFile picks the file whose cover should represent a book based on the
// library's preferred cover aspect ratio. It always falls back across types
// when the preferred kind has no covers — a book-only library still gets a
// cover for an audiobook-only book, and vice versa. Supplements are excluded
// from selection regardless of cover state — they don't represent the book.
// Among ebook files with no Preferred Cover, EPUB comes first, then AZW3, then
// MOBI, then the rest in their given order (models.EbookCoverRank).
func SelectFile(files []*models.File, coverAspectRatio string) *models.File {
	var bookFiles, audiobookFiles []*models.File
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			continue
		}
		if f.CoverImageFilename == nil || *f.CoverImageFilename == "" {
			continue
		}
		switch {
		case models.IsEbookFileType(f.FileType):
			bookFiles = append(bookFiles, f)
		case f.FileType == models.FileTypeM4B:
			audiobookFiles = append(audiobookFiles, f)
		}
	}
	slices.SortStableFunc(bookFiles, func(a, b *models.File) int {
		return models.EbookCoverRank(a.FileType) - models.EbookCoverRank(b.FileType)
	})

	// Within each bucket, prefer a file with IsPreferredCover set.
	pickFirst := func(files []*models.File) *models.File {
		for _, f := range files {
			if f.IsPreferredCover {
				return f
			}
		}
		return files[0]
	}

	switch coverAspectRatio {
	case "audiobook", "audiobook_fallback_book":
		if len(audiobookFiles) > 0 {
			return pickFirst(audiobookFiles)
		}
		if len(bookFiles) > 0 {
			return pickFirst(bookFiles)
		}
	default: // "book", "book_fallback_audiobook", or any other value
		if len(bookFiles) > 0 {
			return pickFirst(bookFiles)
		}
		if len(audiobookFiles) > 0 {
			return pickFirst(audiobookFiles)
		}
	}
	return nil
}

// FileCoverPath returns where a file's own cover lives on disk: the stored
// CoverImageFilename, or {filename}.cover.{ext} when none is stored, in the
// file's directory. It resolves via the file rather than book.Filepath, which
// can be a synthetic organized-folder path that never exists on disk.
func FileCoverPath(file *models.File) string {
	coverFilename := filepath.Base(file.Filepath) + ".cover" + file.CoverExtension()
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		coverFilename = *file.CoverImageFilename
	}
	return filepath.Join(filepath.Dir(file.Filepath), coverFilename)
}

// CacheKey returns a stable cache key for the cover that would be served for
// the given files and aspect ratio. The key only changes when the selected
// cover file changes (different file selected, or the file's UpdatedAt bumps
// after a cover upload/regeneration). Returns "" when no cover exists.
func CacheKey(files []*models.File, coverAspectRatio string) string {
	f := SelectFile(files, coverAspectRatio)
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", f.ID, f.UpdatedAt.Unix())
}

// ServeBookCover selects the cover file from `files` using the library's
// preferred aspect ratio and serves it. Callers must perform any auth and
// library-access checks before calling. Returns errcodes.NotFound(resource)
// when no suitable cover exists or the cover image is missing on disk. Book
// covers pass "Cover"; the series route passes "Series cover".
//
// The cover is resolved via the file's parent directory rather than the book's
// filepath because book.Filepath can be a synthetic organized-folder path that
// never exists on disk for root-level books.
//
// cacheControl sets the Cache-Control header. API callers should pass
// CacheControlImmutable (the frontend uses ?v=cover_cache_key to bust cache);
// external callers (OPDS, eReader, Kobo) should pass CacheControlNoCache.
// API callers may supply the shared thumbnail cache to honor size/aspect query
// parameters. Calls without a cache always serve the original image.
//
// Original-image conditional GET uses an ETag of `"<file_id>-<mtime_unix>"` and intentionally
// omits Last-Modified. SelectFile's choice depends on the library's
// CoverAspectRatio and which files belong to the book, so the served file's
// identity can change without any change to the new cover's mtime — flipping
// CoverAspectRatio on a hybrid book (EPUB + M4B) swaps which file's cover is
// served, and the new cover may have an older mtime than the previously-served
// one. Mtime-only revalidation would return stale 304s in that case; baking
// the file ID into the validator ensures it bumps whenever selection changes.
func ServeBookCover(c echo.Context, files []*models.File, coverAspectRatio, cacheControl, resource string, thumbnailCaches ...*ThumbnailCache) error {
	coverFile := SelectFile(files, coverAspectRatio)
	if coverFile == nil || coverFile.CoverImageFilename == nil || *coverFile.CoverImageFilename == "" {
		return errcodes.NotFound(resource)
	}
	if len(thumbnailCaches) > 0 {
		if handled, err := serveThumbnail(c, coverFile, cacheControl, resource, thumbnailCaches[0]); handled {
			return err
		}
	}

	coverPath := FileCoverPath(coverFile)
	notFound := errcodes.NotFound(resource)
	// The ETag needs the cover's mtime. ServeFile opens the file and sets the
	// headers only once that succeeds, so a stat that passes before an open
	// that fails still yields a plain 500.
	stat, err := os.Stat(coverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return notFound
		}
		return errors.WithStack(err)
	}
	etag := fmt.Sprintf(`"%d-%d"`, coverFile.ID, stat.ModTime().Unix())

	// WithETag makes the ETag the only validator: ServeContent answers a
	// matching If-None-Match with 304 and sends no Last-Modified.
	return httputil.ServeFile(c, coverPath, notFound,
		httputil.WithCacheControl(cacheControl),
		httputil.WithETag(etag))
}

// ServeFileCover serves an individual file's cover, optionally resized on demand.
// Like ServeBookCover, callers must authorize the file before calling; no
// thumbnail parameter bypasses authorization.
func ServeFileCover(c echo.Context, file *models.File, cacheControl, resource string, thumbnails *ThumbnailCache) error {
	if handled, err := serveThumbnail(c, file, cacheControl, resource, thumbnails); handled {
		return err
	}
	return httputil.ServeFile(c, FileCoverPath(file), errcodes.NotFound(resource), httputil.WithCacheControl(cacheControl))
}
