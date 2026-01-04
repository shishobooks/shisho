package books

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers book routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	personService := people.NewService(db)
	searchService := search.NewService(db)

	h := &handler{
		bookService:    bookService,
		libraryService: libraryService,
		personService:  personService,
		searchService:  searchService,
	}

	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("libraryId"))
	g.GET("", h.list)
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("/:id/cover", h.bookCover)
	g.POST("/:id/cover", h.uploadBookCover, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("/files/:id/cover", h.fileCover)
	g.POST("/files/:id", h.updateFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/files/:id/cover", h.uploadFileCover, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
