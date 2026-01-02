package people

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	personService := NewService(db)

	h := &handler{
		personService: personService,
	}

	e.GET("/people", h.list)
	e.GET("/people/:id", h.retrieve)
	e.PATCH("/people/:id", h.update)
	e.DELETE("/people/:id", h.deletePerson)
	e.GET("/people/:id/authored-books", h.authoredBooks)
	e.GET("/people/:id/narrated-files", h.narratedFiles)
	e.POST("/people/:id/merge", h.merge)
}
