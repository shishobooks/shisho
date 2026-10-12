package httputil

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
// outside printable ASCII). An empty filename sends no Content-Disposition,
// which overrides an earlier WithAttachment.
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

// ServeStream sends body, a file still being written, as a whole-file 200
// with the headers the options set (WithETag is ignored). size is the file's
// exact length, sent as Content-Length, or -1 when it is not known yet, in
// which case the response is chunked. A Range header is ignored, since bytes
// past what has been written do not exist yet; once the file is complete it
// is served with ServeFile, which handles Range. Each read is flushed to the
// client as it arrives, so a slow writer never leaves bytes sitting in a
// buffer. A nil body, as for HEAD, sends the headers alone.
//
// Call it only once body is ready to read: the headers are sent first, so an
// error from body can only cut the response short.
func ServeStream(c echo.Context, body io.Reader, size int64, opts ...ServeOption) error {
	var o serveOptions
	for _, opt := range opts {
		opt(&o)
	}
	res := c.Response()
	header := res.Header()
	contentType := o.contentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set(echo.HeaderContentType, contentType)
	if o.attachment != "" {
		SetAttachmentFilename(res, o.attachment)
	}
	if o.cacheControl != "" {
		header.Set("Cache-Control", o.cacheControl)
	}
	if size >= 0 {
		header.Set(echo.HeaderContentLength, strconv.FormatInt(size, 10))
	}
	res.WriteHeader(http.StatusOK)
	if body == nil || c.Request().Method == http.MethodHead {
		return nil
	}

	// A writer that cannot flush (a test recorder) buffers nothing anyway.
	flusher := http.NewResponseController(res.Writer)
	buf := make([]byte, 32<<10)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, werr := res.Write(buf[:n]); werr != nil {
				return errors.WithStack(werr)
			}
			if ferr := flusher.Flush(); ferr != nil && !errors.Is(ferr, http.ErrNotSupported) {
				return errors.WithStack(ferr)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
}
