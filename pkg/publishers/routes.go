package publishers

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers publisher routes on a pre-configured group.
// reviewRecomputer is required; pass a books service with app settings
// attached, or the delete handler's review recompute does nothing.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, reviewRecomputer BookReviewRecomputer) {
	publisherService := NewService(db)
	aliasService := aliases.NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		publisherService: publisherService,
		aliasService:     aliasService,
		searchService:    searchService,
		reviewRecomputer: reviewRecomputer,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/files", h.files)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.DELETE("/:id", h.deletePublisher, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/set-child", h.setChild, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
