package joblogs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers job log routes on the jobs group.
func RegisterRoutes(jobsGroup *echo.Group, db *bun.DB) {
	jobLogService := NewService(db)
	jobService := jobs.NewService(db)

	h := &handler{
		jobLogService: jobLogService,
		jobService:    jobService,
	}

	// GET /api/jobs/:id/logs
	jobsGroup.GET("/:id/logs", h.listLogs)
}
