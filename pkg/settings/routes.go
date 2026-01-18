package settings

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	h := &handler{
		settingsService: NewService(db),
	}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/viewer", h.getViewerSettings)
	g.PUT("/viewer", h.updateViewerSettings)
}
