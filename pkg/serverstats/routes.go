package serverstats

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers GET /server-stats and GET /server-stats/reading,
// which need Config Read like the rest of Administration's server pages.
// Both are reads, so Demo Mode lets them through.
func RegisterRoutes(e *echo.Group, db *bun.DB, cfg *config.Config, caches CacheSizer, authMiddleware *auth.Middleware) {
	h := newHandler(db, cfg, caches)
	g := e.Group("/server-stats", authMiddleware.Authenticate)
	g.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	g.GET("", h.retrieve)
	// Reading over one range, apart from the rest so changing the range
	// doesn't stat every volume again.
	g.GET("/reading", h.retrieveReading)
}
