package bookmarks

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the Bookmark routes on the reading group, which
// the server has already configured with authentication and Books Read.
// Bookmarks are the caller's own, so every route acts only on the
// signed-in user's and needs no further permission, and each checks the
// library access of the File it touches. Every write is a POST, PATCH, or
// DELETE, which Demo Mode rejects.
func RegisterRoutes(g *echo.Group, db *bun.DB) {
	h := &handler{bookmarkService: NewService(db)}

	g.GET("/files/:id/bookmarks", h.list)
	g.POST("/files/:id/bookmarks", h.create)
	g.PATCH("/bookmarks/:id", h.update)
	g.DELETE("/bookmarks/:id", h.delete)
}
