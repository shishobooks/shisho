package httputil

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
