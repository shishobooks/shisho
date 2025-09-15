package books

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	bookService := NewService(db)

	h := &handler{
		bookService: bookService,
	}

	e.GET("/books/:id", h.retrieve)
	e.GET("/books", h.list)
	e.POST("/books/:id", h.update)
	e.GET("/books/:id/cover", h.bookCover)
	e.GET("/files/:id/cover", h.fileCover)
	e.GET("/series", h.listSeries)
	e.GET("/series/:name/books", h.seriesBooks)
	e.GET("/series/:name/cover", h.seriesCover)
}
