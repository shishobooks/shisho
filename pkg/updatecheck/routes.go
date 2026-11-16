package updatecheck

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers GET /update-check, which needs Config Read: only
// someone who runs the server can act on a newer release. It reads a cached
// answer and changes nothing, so Demo Mode lets it through; Demo Mode turns
// the check itself off.
func RegisterRoutes(e *echo.Group, service *Service, authMiddleware *auth.Middleware) {
	h := &handler{service: service}
	g := e.Group("/update-check", authMiddleware.Authenticate)
	g.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.GET("", h.retrieve)
}
