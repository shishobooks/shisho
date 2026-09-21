package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
)

// demoModeMiddleware must be registered with Use, not Pre, so Path contains
// the matched route pattern. This boundary applies before authentication too.
func demoModeMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		switch c.Request().Method {
		case http.MethodGet:
			switch c.Path() {
			case "/api/books/files/:id/download/original", "/api/books/files/:id/download/kepub", "/api/jobs/:id/download":
				return errcodes.DemoMode()
			}
			return next(c)
		case http.MethodHead, http.MethodOptions:
			return next(c)
		case http.MethodPost:
			if c.Path() == "/api/auth/login" || c.Path() == "/api/auth/logout" {
				return next(c)
			}
		}
		return errcodes.DemoMode()
	}
}
