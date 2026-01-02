package libraries

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers library routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	libraryService := NewService(db)

	h := &handler{
		libraryService: libraryService,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("id"))
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
}
