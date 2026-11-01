package reading

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the reading routes on a group the server has
// already configured with authentication and Books Read. Reading data is
// the caller's own, so every route acts only on the signed-in user's
// records and needs no further permission; each that names a Book or File checks
// its library access. Every write is a POST, PATCH, PUT, or DELETE, which
// Demo Mode rejects.
func RegisterRoutes(g *echo.Group, db *bun.DB) {
	h := &handler{readingService: NewService(db)}

	g.GET("/books/:id", h.retrieve)
	g.POST("/books/:id/start", h.start)
	g.POST("/books/:id/pause", h.pause)
	g.POST("/books/:id/resume", h.resume)
	g.POST("/books/:id/finish", h.finish)
	g.POST("/books/:id/abandon", h.abandon)
	g.PUT("/books/:id/rating", h.setRating)
	g.PUT("/books/:id/page-flow", h.setPageFlow)
	g.POST("/books/:id/to-read", h.addToRead)
	g.DELETE("/books/:id/to-read", h.removeFromToRead)
	g.PUT("/to-read/order", h.reorderToRead)
	g.PATCH("/read-throughs/:id", h.updateReadThrough)
	g.DELETE("/read-throughs/:id", h.deleteReadThrough)
	g.POST("/read-throughs/:id/undo-finish", h.undoFinish)
	g.GET("/files/:id/position", h.retrievePosition)
	g.POST("/files/:id/position", h.updatePosition)
}
