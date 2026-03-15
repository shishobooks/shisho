package jobs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers job routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, broker *events.Broker) {
	jobService := NewService(db)

	h := &handler{
		jobService: jobService,
		broker:     broker,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceJobs, models.OperationWrite))
}
