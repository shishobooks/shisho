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

func TestHandlerMissingAssetIsNotFound(t *testing.T) {
	t.Parallel()
	// A page loaded before an upgrade still asks for the old build's chunks.
	// Answering with index.html makes the browser report a MIME type error
	// instead of a missing file, and hides the 404 from caches and logs.
	h := NewHandler(fstest.MapFS{"index.html": {Data: []byte("<html>Shisho</html>")}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/epub-old123.js", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Shisho")
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestBuildIDReadsTheBuildMetaTag(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{"index.html": {Data: []byte(`<!doctype html><html><head><meta charset="UTF-8" /><meta name="shisho-build" content="a1b2c3d4e5f6" /></head></html>`)}}
	assert.Equal(t, "a1b2c3d4e5f6", BuildID(files))
}

func TestBuildIDIsEmptyWithoutABuild(t *testing.T) {
	t.Parallel()
	// Development serves the app from Vite, and the binary embeds only the
	// placeholder, so there is no build to compare a tab against.
	assert.Empty(t, BuildID(fstest.MapFS{"placeholder.html": {Data: []byte("<html>Frontend not built</html>")}}))
	assert.Empty(t, BuildID(fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}))
}
