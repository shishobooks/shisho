package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestHandlerWithoutFrontendBuild(t *testing.T) {
	t.Parallel()
	h := NewHandler(fstest.MapFS{"placeholder.html": {Data: []byte("<html>Frontend not built</html>")}})
	for _, path := range []string{"/", "/libraries/42"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "<html>Frontend not built</html>", rec.Body.String())
		assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	}
}

func TestHandlerHeadAndRange(t *testing.T) {
	t.Parallel()
	h := NewHandler(fstest.MapFS{"index.html": {Data: []byte("<html>Shisho</html>")}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/libraries/42", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Range", "bytes=0-5")
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "<html>", rec.Body.String())
}

func TestHandlerSPAFallback(t *testing.T) {
	t.Parallel()
	h := NewHandler(fstest.MapFS{
		"index.html":           {Data: []byte("<html>Shisho</html>")},
		"assets/app-abc123.js": {Data: []byte("console.log('Shisho')")},
	})
	for _, path := range []string{"/", "/index.html", "/libraries/42", "/missing", "/assets/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusOK, rec.Code, path)
		assert.Equal(t, "<html>Shisho</html>", rec.Body.String(), path)
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"), path)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))
	assert.Equal(t, "console.log('Shisho')", rec.Body.String())
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
}
