package libraries

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	libraryService := NewService(db)

	h := &handler{
		libraryService: libraryService,
	}

	e.POST("/libraries", h.create)
	e.GET("/libraries/:id", h.retrieve)
	e.GET("/libraries", h.list)
	e.POST("/libraries/:id", h.update)
}
