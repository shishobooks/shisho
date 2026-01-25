package apikeys

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers API key management routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	service := NewService(db)
	h := newHandler(service)

	// All routes require authentication
	g := e.Group("/user/api-keys", authMiddleware.Authenticate)

	g.GET("", h.List)
	g.POST("", h.Create)
	g.PATCH("/:id", h.UpdateName)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/permissions/:permission", h.AddPermission)
	g.DELETE("/:id/permissions/:permission", h.RemovePermission)
	g.POST("/:id/short-url", h.GenerateShortURL)
	g.DELETE("/:id/kobo-sync", h.ClearKoboSync)
}
