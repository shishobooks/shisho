package tags

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/bookpreviews"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers tag routes on a group the server has already
// configured with authentication and the resource's read permission.
// reviewRecomputer is required; pass a books service with app settings
// attached, or the delete handler's review recompute does nothing.
func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, reviewRecomputer review.BookReviewRecomputer) {
	tagService := NewService(db)
	aliasService := aliases.NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		tagService:       tagService,
		bookPreviews:     bookpreviews.NewService(db),
		aliasService:     aliasService,
		searchService:    searchService,
		reviewRecomputer: reviewRecomputer,
	}

	g.GET("", h.list)
	g.GET("/namespaces", h.listNamespaces)
	g.GET("/:id", h.retrieve)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.DELETE("/:id", h.deleteTag, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
