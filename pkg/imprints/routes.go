package imprints

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers imprint routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	imprintService := NewService(db)
	aliasService := aliases.NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		imprintService: imprintService,
		aliasService:   aliasService,
		searchService:  searchService,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/files", h.files)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.DELETE("/:id", h.deleteImprint, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
