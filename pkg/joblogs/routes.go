package joblogs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers job log routes on the jobs group.
func RegisterRoutes(jobsGroup *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	jobLogService := NewService(db)
	jobService := jobs.NewService(db)

	h := &handler{
		jobLogService: jobLogService,
		jobService:    jobService,
	}

	// GET /api/jobs/:id/logs
	jobsGroup.GET("/:id/logs", h.listLogs, authMiddleware.RequirePermission(models.ResourceJobs, models.OperationRead))
}
