package cache

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers cache management routes on the given echo instance.
// GET /cache requires config:read; POST /cache/:id/clear requires config:write.
func RegisterRoutes(e *echo.Echo, h *Handler, authMiddleware *auth.Middleware) {
	g := e.Group("/cache")
	g.Use(authMiddleware.Authenticate)

	g.GET("", h.list, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.POST("/:id/clear", h.clear, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}
