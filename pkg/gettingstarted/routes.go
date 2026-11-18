package gettingstarted

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the signed-in user's Getting Started routes, which
// need only authentication since each reads and writes the caller's own list,
// and an admin's "Show Getting Started" for another user under /users, which
// needs Users Write. In Demo Mode the GET reports the list off and the
// middleware refuses the writes.
func RegisterRoutes(e *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, demoMode bool) {
	h := newHandler(db, demoMode)

	g := e.Group("/getting-started", authMiddleware.Authenticate)
	g.GET("", h.retrieve)
	g.POST("/dismiss", h.dismiss)
	g.POST("/restore", h.restore)
	g.DELETE("/auto-open", h.clearAutoOpen)
	g.POST("/steps/:step/not-for-me", h.markNotForMe)
	g.DELETE("/steps/:step/not-for-me", h.unmarkNotForMe)

	e.POST("/users/:id/show-getting-started", h.show,
		authMiddleware.Authenticate,
		authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
}
