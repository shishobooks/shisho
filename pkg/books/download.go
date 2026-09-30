package books

import (
	"context"
	"io/fs"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/models"
)

// Download is a file ready to send: where it is, the name to save it under,
// and its media type ("" to type it by extension).
type Download struct {
	Path        string
	Filename    string
	ContentType string
}

// KepubContentType is the media type a KePub download is served as (Kobo
// overrides it with application/octet-stream). A KePub is an EPUB with Kobo's
// span markup, so it keeps the registered EPUB type, even though the OPDS
// KePub feed advertises its links as application/kepub+zip.
const KepubContentType = "application/epub+zip"

// OriginalDownload is the file as it is on disk, under its own name.
func OriginalDownload(file *models.File) *Download {
	return &Download{
		Path:        file.Filepath,
		Filename:    filepath.Base(file.Filepath),
		ContentType: models.FileTypeMimeType(file.FileType),
	}
}

// ResolveFallbackDownload returns what the OPDS, eReader, Kobo, and Share
// Link download routes send: the file generated with the book's metadata (its
// KePub conversion when kepub is set), or the original when generation is not
// possible. A device or a Share recipient has no Download Original to turn
// to, so a supplement, a type with no generator (filegen.ErrNotImplemented),
// a KePub request for a type KePub cannot convert
// (filegen.ErrKepubNotSupported), and any other filegen.GenerationError get
// the original. The exceptions are a permission error and a canceled or
// timed-out request, which are faults the original cannot fix; they and any
// error outside generation are a 500. The original is still served through
// httputil.ServeFile, so a source that cannot be opened ends as a 500 too.
//
// The web app's download routes do not fall back: a browser user sees the
// error and has Download Original.
func ResolveFallbackDownload(ctx context.Context, cache *downloadcache.Cache, book *models.Book, file *models.File, kepub bool) (*Download, error) {
	if file.FileRole == models.FileRoleSupplement {
		return OriginalDownload(file), nil
	}
	// Check for a generator before the cache is touched, so a full or
	// read-only cache cannot fail a file there is nothing to generate for.
	// GetOrGenerateKepub makes the same check for KePub itself.
	if _, err := filegen.GetGenerator(file.FileType); !kepub && errors.Is(err, filegen.ErrNotImplemented) {
		return OriginalDownload(file), nil
	}

	var path, filename string
	var err error
	if kepub {
		path, filename, err = cache.GetOrGenerateKepub(ctx, book, file)
	} else {
		path, filename, err = cache.GetOrGenerate(ctx, book, file)
	}
	switch {
	case err == nil:
		if kepub {
			return &Download{Path: path, Filename: filename, ContentType: KepubContentType}, nil
		}
		return &Download{Path: path, Filename: filename, ContentType: models.FileTypeMimeType(file.FileType)}, nil
	case errors.Is(err, filegen.ErrNotImplemented), errors.Is(err, filegen.ErrKepubNotSupported):
		return OriginalDownload(file), nil
	case isGenerationFallback(err):
		logger.FromContext(ctx).Warn("file generation failed, serving the original", logger.Data{
			"file_id":   file.ID,
			"file_type": file.FileType,
			"kepub":     kepub,
			"error":     err.Error(),
		})
		return OriginalDownload(file), nil
	}
	return nil, errors.WithStack(err)
}

// isGenerationFallback reports whether a generation failure is one the
// original file can stand in for: a filegen.GenerationError that is not a
// permission error or a canceled or timed-out request.
func isGenerationFallback(err error) bool {
	var genErr *filegen.GenerationError
	if !errors.As(err, &genErr) {
		return false
	}
	return !errors.Is(err, fs.ErrPermission) &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded)
}

// Serve sends the download as an attachment no cache may store. Options
// passed here apply after the defaults, so a caller can override one, as the
// Kobo route does with its Content-Type.
func (d *Download) Serve(c echo.Context, opts ...httputil.ServeOption) error {
	defaults := []httputil.ServeOption{
		httputil.WithContentType(d.ContentType),
		httputil.WithAttachment(d.Filename),
		httputil.WithCacheControl("private, no-store"),
	}
	return httputil.ServeFile(c, d.Path, errcodes.NotFound("File"), append(defaults, opts...)...)
}
