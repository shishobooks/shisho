package cache

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers cache management routes on the given echo instance.
// GET /cache requires config:read; POST /cache/:id/clear requires config:write.
func RegisterRoutes(e *echo.Group, h *Handler, authMiddleware *auth.Middleware) {
	g := e.Group("/cache")
	g.Use(authMiddleware.Authenticate)

	g.GET("", h.list, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.POST("/:id/clear", h.clear, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}

// RegisterSettingsRoutes registers persisted policy alongside other admin settings.
func RegisterSettingsRoutes(e *echo.Group, authMiddleware *auth.Middleware, store *appsettings.Service, thumbnails *covers.ThumbnailCache) {
	h := &settingsHandler{store: store, thumbnails: thumbnails}
	g := e.Group("/settings/cache", authMiddleware.Authenticate)
	g.GET("", h.get, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.PUT("", h.update, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}
