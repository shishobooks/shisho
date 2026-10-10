package ereader

import (
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
)

// TestRegisterRoutes_DownloadAcceptsHEAD ensures the eReader download routes
// respond to HEAD as well as GET. HTTP clients that probe a download URL with
// HEAD first (to read Content-Disposition, content length, etc.) get a 405 if
// the route is GET-only, forcing a fallback to a URL-derived filename.
func TestRegisterRoutes_DownloadAcceptsHEAD(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)

	e := echo.New()
	RegisterRoutes(e, db, nil, books.NewService(db, appsettings.NewService(db)))

	methods := map[string]map[string]bool{}
	for _, r := range e.Routes() {
		if methods[r.Path] == nil {
			methods[r.Path] = map[string]bool{}
		}
		methods[r.Path][r.Method] = true
	}

	for _, path := range []string{
		"/ereader/key/:apiKey/file/:fileId",
		"/ereader/key/:apiKey/file/:fileId/kepub",
		"/ereader/key/:apiKey/file/:fileId/kindle/:filename",
	} {
		assert.True(t, methods[path][http.MethodGet], "GET %s should be registered", path)
		assert.True(t, methods[path][http.MethodHead], "HEAD %s should be registered (clients probe downloads with HEAD for their headers)", path)
	}
}
