package delivery

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the current user's default Delivery Rules
// routes. A user's own rules need no permission.
func RegisterRoutes(e *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	h := &handler{service: NewService(db)}

	g := e.Group("/user/delivery-rules", authMiddleware.Authenticate)
	g.GET("", h.retrieve)
	g.PUT("", h.update)
	g.DELETE("", h.reset)
}
