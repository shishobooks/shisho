package httputil

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
)

// ServeOption sets a header on a file ServeFile serves. Options apply only
// once the file has opened, so an error response never carries them.
type ServeOption func(*serveOptions)

type serveOptions struct {
	contentType  string
	attachment   string
	cacheControl string
	etag         string
}

// WithContentType sets the Content-Type instead of deriving it from the
// extension. Pass the type for the file type (models.FileTypeMimeType) for a
// book download, since the host mime table may not know .epub, .cbz, or .m4b.
// An empty type leaves the extension-based type in place.
func WithContentType(contentType string) ServeOption {
	return func(o *serveOptions) { o.contentType = contentType }
}

// WithAttachment sends the file as a download named filename, through
// SetAttachmentFilename (an escaped ASCII fallback plus filename* for names
// outside printable ASCII).
func WithAttachment(filename string) ServeOption {
	return func(o *serveOptions) { o.attachment = filename }
}

// WithCacheControl sets the Cache-Control header.
func WithCacheControl(value string) ServeOption {
	return func(o *serveOptions) { o.cacheControl = value }
}

// WithETag sets an ETag and makes it the only validator: Last-Modified is
// omitted and If-Modified-Since ignored, while If-None-Match still answers
// 304. Use it when the file served at a URL can change without its mtime
// changing, as for a book cover selected from several files.
func WithETag(etag string) ServeOption {
	return func(o *serveOptions) { o.etag = etag }
}

// ServeFile serves the file at path through http.ServeContent, which handles
// Range, If-Range, and conditional GETs. It differs from echo's c.File in two
// ways. Only a missing file or a directory returns notFound; c.File turns
// every open failure, including a permission error, into echo's generic 404,
// and here any other failure is a server fault. And the headers the options
// set are written only after the open succeeds, so a failure goes out as a
// plain JSON error rather than one cached for a year, typed as an image, or
// saved as the book.
//
// A Range header with a unit other than bytes is ignored, as RFC 9110
// section 14.2 requires, and the whole file is served with 200. A malformed
// bytes range, or one that starts past the end, is answered 416 by
// ServeContent.
func ServeFile(c echo.Context, path string, notFound error, opts ...ServeOption) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return notFound
		}
		return errors.WithStack(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return errors.WithStack(err)
	}
	if info.IsDir() {
		return notFound
	}

	var o serveOptions
	for _, opt := range opts {
		opt(&o)
	}
	header := c.Response().Header()
	if o.contentType != "" {
		header.Set(echo.HeaderContentType, o.contentType)
	}
	if o.attachment != "" {
		SetAttachmentFilename(c.Response(), o.attachment)
		// ServeContent can still answer 412 or 416 after the open, and Go
		// strips the cache headers from those but not this one.
		res := c.Response()
		res.Before(func() {
			if res.Status >= http.StatusBadRequest {
				header.Del("Content-Disposition")
			}
		})
	}
	if o.cacheControl != "" {
		header.Set("Cache-Control", o.cacheControl)
	}
	modTime := info.ModTime()
	if o.etag != "" {
		header.Set("ETag", o.etag)
		// A zero modtime keeps ServeContent from sending Last-Modified or
		// answering If-Modified-Since.
		modTime = time.Time{}
	}

	req := c.Request()
	if rng := req.Header.Get("Range"); rng != "" && !strings.HasPrefix(rng, "bytes=") {
		req.Header.Del("Range")
	}
	http.ServeContent(c.Response(), req, filepath.Base(path), modTime, f)
	return nil
}
