package chapters

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	h := &handler{
		chapterService: NewService(db),
		bookService:    books.NewService(db),
	}

	g.GET("/files/:id/chapters", h.list)
	g.PUT("/files/:id/chapters", h.replace, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
