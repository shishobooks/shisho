package reach

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the routes that show what the current user's
// Devices get: a book page's Devices tab, which needs books:read
// like the book page, and a Device's delivery preview, which like the rest
// of a user's own Devices needs no permission. Both are GETs, so Demo Mode
// lets them through.
func RegisterRoutes(e *echo.Group, db *bun.DB, bookService *books.Service, authMiddleware *auth.Middleware) {
	h := &handler{service: NewService(db, bookService), bookService: bookService, deviceService: devices.NewService(db)}

	e.GET("/books/:id/devices", h.bookDevices, authMiddleware.Authenticate, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	e.GET("/user/devices/:id/delivery-preview", h.deliveryPreview, authMiddleware.Authenticate)
}
