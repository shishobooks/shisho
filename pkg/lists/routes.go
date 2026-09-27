package lists

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers lists routes on a group the server has already
// configured with authentication.
func RegisterRoutes(g *echo.Group, db *bun.DB) {
	listsService := NewService(db)

	h := &handler{
		listsService: listsService,
	}

	// List CRUD
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.retrieve)
	g.PATCH("/:id", h.update)
	g.DELETE("/:id", h.delete)

	// List books
	g.GET("/:id/books", h.listBooks)
	g.POST("/:id/books", h.addBooks)
	g.DELETE("/:id/books", h.removeBooks)
	g.PATCH("/:id/books/reorder", h.reorderBooks)
	g.PATCH("/:id/books/:bookId/position", h.moveBookPosition)

	// Sharing
	g.GET("/:id/shares", h.listShares)
	g.POST("/:id/shares", h.createShare)
	g.PATCH("/:id/shares/:shareId", h.updateShare)
	g.DELETE("/:id/shares/:shareId", h.deleteShare)
	g.GET("/:id/shares/check", h.checkVisibility)

	// Templates
	g.GET("/templates", h.templates)
	g.POST("/templates/:name", h.createFromTemplate)
}
