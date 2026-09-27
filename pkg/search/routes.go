package search

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers search routes on a group the server has already
// configured with authentication.
func RegisterRoutes(g *echo.Group, db *bun.DB) {
	searchService := NewService(db)

	h := &handler{
		searchService: searchService,
	}

	g.GET("", h.globalSearch)
}
