package series

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	seriesService := NewService(db)
	bookService := books.NewService(db)

	h := &handler{
		seriesService: seriesService,
		bookService:   bookService,
	}

	e.GET("/series", h.list)
	e.GET("/series/:id", h.retrieve)
	e.PATCH("/series/:id", h.update)
	e.DELETE("/series/:id", h.deleteSeries)
	e.GET("/series/:id/books", h.seriesBooks)
	e.GET("/series/:id/cover", h.seriesCover)
	e.POST("/series/:id/merge", h.merge)
}
