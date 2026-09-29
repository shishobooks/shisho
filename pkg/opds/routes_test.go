package opds

import (
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
)

// TestRegisterRoutes_DownloadAcceptsHEAD ensures the OPDS download routes
// respond to HEAD as well as GET. KOReader's "Use server filenames" mode
// issues a HEAD request to read Content-Disposition before downloading; if
// the route is GET-only, Echo returns 405 and KOReader falls back to the
// URL-derived filename (the file ID).
func TestRegisterRoutes_DownloadAcceptsHEAD(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	authMw := auth.NewMiddleware(auth.NewService(db, "test-secret", time.Hour))

	e := echo.New()
	RegisterRoutes(e, db, authMw, downloadcache.NewCache(t.TempDir(), 1<<30), books.NewService(db))

	methods := map[string]map[string]bool{}
	for _, r := range e.Routes() {
		if methods[r.Path] == nil {
			methods[r.Path] = map[string]bool{}
		}
		methods[r.Path][r.Method] = true
	}

	for _, path := range []string{"/opds/download/:id", "/opds/download/:id/kepub"} {
		assert.True(t, methods[path][http.MethodGet], "GET %s should be registered", path)
		assert.True(t, methods[path][http.MethodHead], "HEAD %s should be registered (KOReader 'Use server filenames' relies on it)", path)
	}
}
