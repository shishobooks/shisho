package books

import (
	"context"
	"io"
	"io/fs"
	"net/http"
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

	// building is set instead of Path for a Generated File that is still
	// being built: Serve streams it as it grows.
	building *downloadcache.Generated
}

// OriginalDownload is the file as it is on disk, under its own name.
func OriginalDownload(file *models.File) *Download {
	return &Download{
		Path:        file.Filepath,
		Filename:    filepath.Base(file.Filepath),
		ContentType: models.FileTypeMimeType(file.FileType),
	}
}

// FailedPartwayMessage is the web UI routes' answer to a Generated File
// whose earlier build failed partway (downloadcache.ErrFailedPartway).
const FailedPartwayMessage = "This file's generated download failed partway through an earlier attempt. Download the original, or edit the book's metadata to try again."

// CannotDecryptMessage is the web UI routes' answer to a file whose
// encryption Shisho cannot open (filegen.ErrCannotDecrypt), such as a PDF
// that needs a password.
const CannotDecryptMessage = "This file is password-protected or encrypted in a way Shisho can't open, so its metadata can't be added. Download the original instead."

// ResolveFallbackDownload returns what a device or a Share recipient gets
// for the file as its own File Type: the Generated File, or the Original
// File when generation is not possible, since a failed generation of a
// File's own File Type only loses Shisho's metadata. A device or a Share
// recipient has no Download Original to turn to, so a supplement, a type
// with no generator (filegen.ErrNotImplemented), and any other
// filegen.GenerationError get the original. The exceptions are a permission
// error and a canceled or timed-out request, which are faults the original
// cannot fix; they and any error outside generation are a 500. The original
// is still served through httputil.ServeFile, so a source that cannot be
// opened ends as a 500 too.
//
// Nothing falls back from a Conversion: a channel delivering another File
// Type uses ResolveDeliveryDownload, and a download menu entry
// GeneratedDownload. The web app's download routes do not fall back either:
// a browser user sees the error and has Download Original.
func ResolveFallbackDownload(c echo.Context, cache *downloadcache.Cache, book *models.Book, file *models.File) (*Download, error) {
	ctx := c.Request().Context()
	if file.FileRole == models.FileRoleSupplement {
		return OriginalDownload(file), nil
	}
	// Check for a generator before the cache is touched, so a full or
	// read-only cache cannot fail a file there is nothing to generate for.
	if _, err := filegen.GeneratorFor(file.FileType, file.FileType); errors.Is(err, filegen.ErrNotImplemented) {
		return OriginalDownload(file), nil
	}

	download, err := GeneratedDownload(c, cache, book, file, file.FileType)
	switch {
	case err == nil:
		return download, nil
	case isGenerationFallback(err):
		logger.FromContext(ctx).Warn("file generation failed, serving the original", logger.Data{
			"file_id":   file.ID,
			"file_type": file.FileType,
			"error":     err.Error(),
		})
		return OriginalDownload(file), nil
	}
	return nil, errors.WithStack(err)
}

// ResolveDeliveryDownload returns what a channel on Delivery Rules (OPDS,
// the eReader browser, and Kobo sync) sends for the file delivered as
// deliveredType. Delivered as its own File Type, it falls back to the
// Original File as ResolveFallbackDownload does. Delivered as another File
// Type, any failure of the Conversion is returned, whether it fails now or
// failed partway through an earlier attempt
// (downloadcache.ErrFailedPartway): a reader never receives a File Type its
// rules did not choose (ADR 0011).
func ResolveDeliveryDownload(c echo.Context, cache *downloadcache.Cache, book *models.Book, file *models.File, deliveredType string) (*Download, error) {
	if deliveredType == file.FileType {
		return ResolveFallbackDownload(c, cache, book, file)
	}
	download, err := GeneratedDownload(c, cache, book, file, deliveredType)
	if err != nil {
		return nil, errors.Wrapf(err, "delivering file %d (%s) as %s", file.ID, file.Filepath, deliveredType)
	}
	return download, nil
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

// GeneratedDownload returns the Generated File for the book's file delivered
// as deliveredType: the cached file, or the file streamed while it is built
// (ADR 0010). A HEAD request runs only the preparation step, so it reports
// an error or the size without generating. Errors from the preparation step
// are returned before anything is sent, and a File Type no Conversion
// reaches returns filegen.ErrNoConversion. A Download returned here must be
// served: a file still being built holds the cache's handle on it until
// Serve has read it.
func GeneratedDownload(c echo.Context, cache *downloadcache.Cache, book *models.Book, file *models.File, deliveredType string) (*Download, error) {
	ctx := c.Request().Context()
	var generated *downloadcache.Generated
	var err error
	if c.Request().Method == http.MethodHead {
		generated, err = cache.Head(ctx, book, file, deliveredType)
	} else {
		generated, err = cache.Open(ctx, book, file, deliveredType)
	}
	if err != nil {
		return nil, err
	}

	download := &Download{Path: generated.Path, Filename: generated.Filename, ContentType: models.FileTypeMimeType(deliveredType)}
	if generated.Path == "" {
		download.building = generated
	}
	return download, nil
}

// Serve sends the download as an attachment no cache may store. Options
// passed here apply after the defaults, so a caller can override one, as the
// Kobo route does with its Content-Type. A Generated File still being built
// is streamed whole, ignoring Range, with its size when it is known.
func (d *Download) Serve(c echo.Context, opts ...httputil.ServeOption) error {
	opts = append([]httputil.ServeOption{
		httputil.WithContentType(d.ContentType),
		httputil.WithAttachment(d.Filename),
		httputil.WithCacheControl("private, no-store"),
	}, opts...)
	if d.building != nil {
		var body io.Reader
		if d.building.Body != nil {
			defer d.building.Body.Close()
			body = d.building.Body
		}
		return httputil.ServeStream(c, body, d.building.Size, opts...)
	}
	return httputil.ServeFile(c, d.Path, errcodes.NotFound("File"), opts...)
}
