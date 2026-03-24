package jobs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers job routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, broker *events.Broker, dlCache *downloadcache.Cache) {
	jobService := NewService(db)

	h := &handler{
		jobService:    jobService,
		db:            db,
		broker:        broker,
		downloadCache: dlCache,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/download", h.download)
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceJobs, models.OperationWrite))
}
