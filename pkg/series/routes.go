package series

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers series routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	seriesService := NewService(db)
	bookService := books.NewService(db)

	h := &handler{
		seriesService: seriesService,
		bookService:   bookService,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/books", h.seriesBooks)
	g.GET("/:id/cover", h.seriesCover)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
	g.DELETE("/:id", h.deleteSeries, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourceSeries, models.OperationWrite))
}
