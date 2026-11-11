package serverstats

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers GET /server-stats, which needs Config Read like
// the rest of Administration's server pages. It's a read, so Demo Mode lets
// it through.
func RegisterRoutes(e *echo.Group, db *bun.DB, cfg *config.Config, caches CacheSizer, authMiddleware *auth.Middleware) {
	h := newHandler(db, cfg, caches)
	g := e.Group("/server-stats", authMiddleware.Authenticate)
	g.GET("", h.retrieve, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
}
