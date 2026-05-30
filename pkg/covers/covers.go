// Package covers centralizes cover-file selection and serving so that the
// books, series, ereader, and OPDS handlers don't drift independently.
package covers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"

	"github.com/shishobooks/shisho/pkg/errcodes"
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
func SelectFile(files []*models.File, coverAspectRatio string) *models.File {
	var bookFiles, audiobookFiles []*models.File
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			continue
		}
		if f.CoverImageFilename == nil || *f.CoverImageFilename == "" {
			continue
		}
		switch f.FileType {
		case models.FileTypeEPUB, models.FileTypeCBZ, models.FileTypePDF:
			bookFiles = append(bookFiles, f)
		case models.FileTypeM4B:
			audiobookFiles = append(audiobookFiles, f)
		}
	}

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
// library-access checks before calling. Returns errcodes.NotFound when no
// suitable cover exists or the cover image is missing on disk.
//
// The cover is resolved via the file's parent directory rather than the book's
// filepath because book.Filepath can be a synthetic organized-folder path that
// never exists on disk for root-level books.
//
// cacheControl sets the Cache-Control header. API callers should pass
// CacheControlImmutable (the frontend uses ?v=cover_cache_key to bust cache);
// external callers (OPDS, eReader, Kobo) should pass CacheControlNoCache.
//
// Conditional GET uses an ETag of `"<file_id>-<mtime_unix>"` and intentionally
// omits Last-Modified. SelectFile's choice depends on the library's
// CoverAspectRatio and which files belong to the book, so the served file's
// identity can change without any change to the new cover's mtime — flipping
// CoverAspectRatio on a hybrid book (EPUB + M4B) swaps which file's cover is
// served, and the new cover may have an older mtime than the previously-served
// one. Mtime-only revalidation would return stale 304s in that case; baking
// the file ID into the validator ensures it bumps whenever selection changes.
func ServeBookCover(c echo.Context, files []*models.File, coverAspectRatio string, cacheControl string) error {
	coverFile := SelectFile(files, coverAspectRatio)
	if coverFile == nil || coverFile.CoverImageFilename == nil || *coverFile.CoverImageFilename == "" {
		return errcodes.NotFound("Cover")
	}

	coverPath := filepath.Join(filepath.Dir(coverFile.Filepath), *coverFile.CoverImageFilename)
	// Stat first so a cover file deleted from disk surfaces as a typed 404
	// (matching the no-filename branch above) instead of bubbling up as
	// echo.HTTPError's generic "Not Found".
	stat, err := os.Stat(coverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}
	modTime := stat.ModTime().UTC().Truncate(time.Second)

	etag := fmt.Sprintf(`"%d-%d"`, coverFile.ID, modTime.Unix())

	c.Response().Header().Set("Cache-Control", cacheControl)
	c.Response().Header().Set("ETag", etag)

	if inm := c.Request().Header.Get("If-None-Match"); inm != "" && inm == etag {
		c.Response().WriteHeader(http.StatusNotModified)
		return nil
	}

	fh, err := os.Open(coverPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}
	defer fh.Close()

	// Zero modtime suppresses Last-Modified and IMS handling inside ServeContent.
	http.ServeContent(c.Response(), c.Request(), filepath.Base(coverPath), time.Time{}, fh)
	return nil
}
