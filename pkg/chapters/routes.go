package chapters

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers chapter routes on the books group. bookService is
// required; pass a books service with app settings attached, or the replace
// handler's review recompute does nothing.
func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, bookService *books.Service) {
	h := &handler{
		chapterService: NewService(db),
		bookService:    bookService,
	}

	g.GET("/files/:id/chapters", h.list)
	g.PUT("/files/:id/chapters", h.replace, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
