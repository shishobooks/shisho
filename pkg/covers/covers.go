// Package covers centralizes cover-file selection and serving so that the
// books, series, ereader, and OPDS handlers don't drift independently.
package covers

import (
	"os"
	"path/filepath"

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
// preferred aspect ratio and serves it via c.File. Callers must perform any
// auth and library-access checks before calling. Returns errcodes.NotFound
// when no suitable cover exists or the cover image is missing on disk.
//
// The cover is resolved via the file's parent directory rather than the book's
// filepath because book.Filepath can be a synthetic organized-folder path that
// never exists on disk for root-level books.
func ServeBookCover(c echo.Context, files []*models.File, coverAspectRatio string) error {
	coverFile := SelectFile(files, coverAspectRatio)
	if coverFile == nil || coverFile.CoverImageFilename == nil || *coverFile.CoverImageFilename == "" {
		return errcodes.NotFound("Cover")
	}

	coverPath := filepath.Join(filepath.Dir(coverFile.Filepath), *coverFile.CoverImageFilename)
	// Stat first so a cover file deleted from disk surfaces as a typed 404
	// (matching the no-filename branch above) instead of bubbling up as
	// echo.HTTPError's generic "Not Found" from c.File.
	if _, err := os.Stat(coverPath); err != nil {
		if os.IsNotExist(err) {
			return errcodes.NotFound("Cover")
		}
		return errors.WithStack(err)
	}

	c.Response().Header().Set("Cache-Control", "private, no-cache")
	return errors.WithStack(c.File(coverPath))
}
