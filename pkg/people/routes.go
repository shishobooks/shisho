package people

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers people routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	personService := NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		personService: personService,
		searchService: searchService,
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve)
	g.GET("/:id/authored-books", h.authoredBooks)
	g.GET("/:id/narrated-files", h.narratedFiles)
	g.PATCH("/:id", h.update, authMiddleware.RequirePermission(models.ResourcePeople, models.OperationWrite))
	g.DELETE("/:id", h.deletePerson, authMiddleware.RequirePermission(models.ResourcePeople, models.OperationWrite))
	g.POST("/:id/merge", h.merge, authMiddleware.RequirePermission(models.ResourcePeople, models.OperationWrite))
}
