package search

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers search routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB) {
	searchService := NewService(db)

	h := &handler{
		searchService: searchService,
	}

	g.GET("", h.globalSearch)
}
