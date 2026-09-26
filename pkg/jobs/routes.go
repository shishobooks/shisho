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

	// The group only authenticates. Retrieve, download, and create check Jobs
	// Read or Jobs Write in the handler so a user with Books Read can create,
	// poll, and download their own bulk download (see canReadJob).
	g.GET("", h.list, authMiddleware.RequirePermission(models.ResourceJobs, models.OperationRead))
	g.GET("/:id", h.retrieve)
	g.GET("/:id/download", h.download)
	g.POST("", h.create)
}
