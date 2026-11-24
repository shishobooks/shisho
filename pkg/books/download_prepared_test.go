package books

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

// A prepared Generated File the cache's cleanup removed before it opened is
// reported before anything is sent, so ServePreparedDownload can stream it
// instead of answering 404.
func TestDownloadServe_PreparedFileGoneIsReported(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	d := &Download{Path: filepath.Join(t.TempDir(), "gone.epub"), Filename: "Gone.epub", gone: errPreparedGone}

	err := d.Serve(c)
	assert.True(t, errors.Is(err, errPreparedGone), "%v", err)
	assert.False(t, c.Response().Committed)
}
