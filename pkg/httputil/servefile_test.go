package httputil

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cover.jpg")
	require.NoError(t, os.WriteFile(path, []byte("jpeg bytes"), 0o644))
	notFound := errcodes.NotFound("Cover")

	serve := func(p string) (*httptest.ResponseRecorder, error) {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		return rec, ServeFile(c, p, notFound)
	}

	t.Run("serves the file", func(t *testing.T) {
		t.Parallel()
		rec, err := serve(path)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "jpeg bytes", rec.Body.String())
		assert.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
		assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
	})
	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := serve(filepath.Join(dir, "missing.jpg"))
		assert.Equal(t, notFound, err)
	})
	t.Run("directory", func(t *testing.T) {
		t.Parallel()
		_, err := serve(dir)
		assert.Equal(t, notFound, err)
	})
}

// A file that exists but cannot be opened is a server fault, not a 404.
func TestServeFile_OpenFaultIsServerError(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	path := filepath.Join(t.TempDir(), "locked.jpg")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	err := ServeFile(c, path, errcodes.NotFound("Cover"))
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr)
	var httpErr *echo.HTTPError
	assert.NotErrorAs(t, err, &httpErr)
}

// serveWith runs ServeFile through echo's error handler path the way a route
// does: a returned error is rendered by errcodes.Handler onto the same
// response, so the test sees exactly what a client would.
func serveWith(t *testing.T, req *http.Request, path string, opts ...ServeOption) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := ServeFile(c, path, errcodes.NotFound("File"), opts...); err != nil {
		errcodes.NewHandler().Handle(err, c)
	}
	return rec
}

func TestServeFile_OptionsSetHeadersOnSuccess(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "book.epub")
	require.NoError(t, os.WriteFile(path, []byte("epub bytes"), 0o644))

	rec := serveWith(t, httptest.NewRequest(http.MethodGet, "/", nil), path,
		WithContentType("application/epub+zip"),
		WithAttachment(`Title "Quoted" Ünïcode.epub`),
		WithCacheControl("private, no-store"),
	)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "epub bytes", rec.Body.String())
	assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t,
		`attachment; filename="Title \"Quoted\" ncode.epub"; filename*=UTF-8''Title%20%22Quoted%22%20%C3%9Cn%C3%AFcode.epub`,
		rec.Header().Get("Content-Disposition"))
	assert.NotEmpty(t, rec.Header().Get("Last-Modified"))
}

// Success headers are set only once the open succeeds, so a failure goes out
// as a plain JSON error: not cacheable for a year, not typed as the file, and
// not saved by the browser under the book's filename.
func TestServeFile_FailureCarriesNoSuccessHeaders(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.jpg")
	require.NoError(t, os.WriteFile(locked, []byte("x"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	tests := []struct {
		name   string
		path   string
		status int
		root   bool
	}{
		{"missing", filepath.Join(dir, "missing.jpg"), http.StatusNotFound, false},
		{"directory", dir, http.StatusNotFound, false},
		{"unreadable", locked, http.StatusInternalServerError, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.root && os.Geteuid() == 0 {
				t.Skip("permission bits do not restrict root")
			}
			rec := serveWith(t, httptest.NewRequest(http.MethodGet, "/", nil), tt.path,
				WithContentType("image/jpeg"),
				WithAttachment("Book.epub"),
				WithCacheControl("private, max-age=31536000, immutable"),
				WithETag(`"1-2"`),
			)

			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Empty(t, rec.Header().Get("Cache-Control"))
			assert.Empty(t, rec.Header().Get("Content-Disposition"))
			assert.Empty(t, rec.Header().Get("ETag"))
			assert.Contains(t, rec.Body.String(), `"status_code":`)
		})
	}
}

// WithETag is the mode for a URL whose served file can change without its
// mtime changing: the ETag is the validator, and Last-Modified is omitted so
// If-Modified-Since cannot answer 304 for a different file.
func TestServeFile_WithETag(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "cover.jpg")
	require.NoError(t, os.WriteFile(path, []byte("jpeg bytes"), 0o644))

	t.Run("sets the ETag and omits Last-Modified", func(t *testing.T) {
		t.Parallel()
		rec := serveWith(t, httptest.NewRequest(http.MethodGet, "/", nil), path, WithETag(`"7-100"`))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, `"7-100"`, rec.Header().Get("ETag"))
		assert.Empty(t, rec.Header().Get("Last-Modified"))
		assert.Equal(t, "jpeg bytes", rec.Body.String())
	})
	t.Run("matching If-None-Match is 304", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("If-None-Match", `"7-100"`)
		rec := serveWith(t, req, path, WithETag(`"7-100"`), WithCacheControl("private, no-cache"))
		assert.Equal(t, http.StatusNotModified, rec.Code)
		assert.Empty(t, rec.Body.String())
		assert.Equal(t, "private, no-cache", rec.Header().Get("Cache-Control"))
	})
	t.Run("If-Modified-Since alone does not give 304", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("If-Modified-Since", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat))
		rec := serveWith(t, req, path, WithETag(`"7-100"`))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// Range requests are answered by http.ServeContent. A malformed bytes range
// is a 416, as Go answers it; a range unit other than bytes is ignored, which
// RFC 9110 section 14.2 requires.
func TestServeFile_Range(t *testing.T) {
	t.Parallel()
	const size = 1000
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "audio.m4b")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	tests := []struct {
		rangeHeader  string
		status       int
		contentRange string
		body         []byte
	}{
		{"bytes=0-", http.StatusPartialContent, "bytes 0-999/1000", data},
		{"bytes=100-199", http.StatusPartialContent, "bytes 100-199/1000", data[100:200]},
		{"bytes=900-5000", http.StatusPartialContent, "bytes 900-999/1000", data[900:]},
		{"bytes=-500", http.StatusPartialContent, "bytes 500-999/1000", data[500:]},
		{"bytes=1000-", http.StatusRequestedRangeNotSatisfiable, "bytes */1000", nil},
		{"bytes=5000-6000", http.StatusRequestedRangeNotSatisfiable, "bytes */1000", nil},
		{"bytes=abc", http.StatusRequestedRangeNotSatisfiable, "", nil},
		{"items=0-10", http.StatusOK, "", data},
	}
	for _, tt := range tests {
		t.Run(tt.rangeHeader, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Range", tt.rangeHeader)
			rec := serveWith(t, req, path, WithContentType("audio/mp4"), WithCacheControl("private, no-store"))

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			assert.Equal(t, tt.contentRange, rec.Header().Get("Content-Range"))
			if tt.body != nil {
				assert.Equal(t, tt.body, rec.Body.Bytes())
				assert.Equal(t, "audio/mp4", rec.Header().Get("Content-Type"))
				assert.Equal(t, "bytes", rec.Header().Get("Accept-Ranges"))
			}
		})
	}
}

// A 416 or 412 that ServeContent answers after the open must not go out as an
// attachment, or a download manager saves the error text as the book.
func TestServeFile_ServeContentErrorIsNotAnAttachment(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "book.epub")
	require.NoError(t, os.WriteFile(path, []byte("epub bytes"), 0o644))

	for _, tt := range []struct {
		name, header, value string
		status              int
	}{
		{"range past the end", "Range", "bytes=5000-", http.StatusRequestedRangeNotSatisfiable},
		{"failed If-Match", "If-Match", `"other"`, http.StatusPreconditionFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set(tt.header, tt.value)
			rec := serveWith(t, req, path, WithAttachment("Book.epub"), WithContentType("application/epub+zip"))
			require.Equal(t, tt.status, rec.Code)
			assert.Empty(t, rec.Header().Get("Content-Disposition"))
		})
	}
}
