package jobs

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	jobService := NewService(db)

	h := &handler{
		jobService: jobService,
	}

	e.POST("/jobs", h.create)
	e.GET("/jobs/:id", h.retrieve)
	e.GET("/jobs", h.list)
	// e.POST("/jobs/:id", h.update)
}
