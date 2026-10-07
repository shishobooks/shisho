package series

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers series routes on a group the server has already
// configured with authentication and the resource's read permission.
// bookService is required; pass a books service with app settings attached,
// or the delete handler's review recompute does nothing. The handler also
// uses it for series covers and book listings, so it takes the full service
// rather than the narrow review recompute interface.
func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, bookService *books.Service, coverCache *covers.ThumbnailCache) {
	seriesService := NewService(db)
	aliasService := aliases.NewService(db)
	libraryService := libraries.NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		seriesService:  seriesService,
		aliasService:   aliasService,
		bookService:    bookService,
		libraryService: libraryService,
		searchService:  searchService,
		coverCache:     coverCache,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/books", h.seriesBooks)
	g.GET("/:id/cover", h.seriesCover)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
	g.DELETE("/:id", h.deleteSeries, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
}
