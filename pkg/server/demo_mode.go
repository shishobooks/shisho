package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// deniedDownloads are the explicit download routes. They stay closed to HEAD
// as well as GET, since a HEAD still runs the handler and its file generation.
// The generated download, pages, and audio streaming stay open and still hand
// out complete files, so this is not copy protection, and supplements are
// reachable through the generated download: the demo corpus must not include
// them.
var deniedDownloads = map[string]bool{
	"/api/books/files/:id/download/original": true,
	"/api/books/files/:id/download/kepub":    true,
	"/api/jobs/:id/download":                 true,
}

// demoModeMiddleware must be registered with Use, not Pre, so Path contains
// the matched route pattern. This boundary applies before authentication too.
func demoModeMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead:
			if deniedDownloads[c.Path()] {
				return errcodes.DemoMode()
			}
			return next(c)
		case http.MethodOptions:
			return next(c)
		case http.MethodPost:
			if c.Path() == "/api/auth/login" || c.Path() == "/api/auth/logout" {
				return next(c)
			}
		}
		return errcodes.DemoMode()
	}
}
