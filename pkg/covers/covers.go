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

// SelectFile picks the file whose cover should represent a book based on the
// library's preferred cover aspect ratio. It always falls back across types
// when the preferred kind has no covers — a book-only library still gets a
// cover for an audiobook-only book, and vice versa.
func SelectFile(files []*models.File, coverAspectRatio string) *models.File {
	var bookFiles, audiobookFiles []*models.File
	for _, f := range files {
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

	switch coverAspectRatio {
	case "audiobook", "audiobook_fallback_book":
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
	default: // "book", "book_fallback_audiobook", or any other value
		if len(bookFiles) > 0 {
			return bookFiles[0]
		}
		if len(audiobookFiles) > 0 {
			return audiobookFiles[0]
		}
	}
	return nil
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
// Conditional GET uses an ETag of `"<file_id>-<mtime_unix>"` and intentionally
// omits Last-Modified. SelectFile's choice depends on the library's
// CoverAspectRatio and which files belong to the book, so the served file's
// identity can change without any change to the new cover's mtime — flipping
// CoverAspectRatio on a hybrid book (EPUB + M4B) swaps which file's cover is
// served, and the new cover may have an older mtime than the previously-served
// one. Mtime-only revalidation would return stale 304s in that case; baking
// the file ID into the validator ensures it bumps whenever selection changes.
func ServeBookCover(c echo.Context, files []*models.File, coverAspectRatio string) error {
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

	c.Response().Header().Set("Cache-Control", "private, no-cache")
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
