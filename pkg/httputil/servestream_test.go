package httputil

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveStream(t *testing.T, req *http.Request, body io.Reader, size int64) (*httptest.ResponseRecorder, error) {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	err := ServeStream(c, body, size,
		WithContentType("application/epub+zip"),
		WithAttachment("Book.epub"),
		WithCacheControl("private, no-store"),
	)
	return rec, err
}

func TestServeStream(t *testing.T) {
	t.Parallel()

	t.Run("known size sends Content-Length", func(t *testing.T) {
		t.Parallel()
		rec, err := serveStream(t, httptest.NewRequest(http.MethodGet, "/", nil), strings.NewReader("epub bytes"), 10)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "epub bytes", rec.Body.String())
		assert.Equal(t, "10", rec.Header().Get("Content-Length"))
		assert.Equal(t, "application/epub+zip", rec.Header().Get("Content-Type"))
		assert.Equal(t, `attachment; filename="Book.epub"`, rec.Header().Get("Content-Disposition"))
		assert.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
		assert.Empty(t, rec.Header().Get("Accept-Ranges"))
	})

	t.Run("unknown size sends no Content-Length", func(t *testing.T) {
		t.Parallel()
		rec, err := serveStream(t, httptest.NewRequest(http.MethodGet, "/", nil), strings.NewReader("epub bytes"), -1)
		require.NoError(t, err)
		assert.Equal(t, "epub bytes", rec.Body.String())
		assert.Empty(t, rec.Header().Get("Content-Length"))
	})

	t.Run("Range is ignored", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Range", "bytes=5-")
		rec, err := serveStream(t, req, strings.NewReader("epub bytes"), 10)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "epub bytes", rec.Body.String())
	})

	t.Run("HEAD sends headers alone", func(t *testing.T) {
		t.Parallel()
		rec, err := serveStream(t, httptest.NewRequest(http.MethodHead, "/", nil), nil, 10)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "10", rec.Header().Get("Content-Length"))
		assert.Empty(t, rec.Body.String())
	})

	t.Run("a body error is returned after what was sent", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		body := io.MultiReader(strings.NewReader("epub"), failingReader{boom})
		rec, err := serveStream(t, httptest.NewRequest(http.MethodGet, "/", nil), body, 10)
		require.ErrorIs(t, err, boom)
		assert.Equal(t, "epub", rec.Body.String())
	})
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
