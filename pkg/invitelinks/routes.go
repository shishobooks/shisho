package invitelinks

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the admin routes under /invite-links, which need
// Users Write since an invite adds a user, and the public routes under
// /invites/:token, which need no account. In Demo Mode the middleware
// refuses every one of them: the writes as it refuses every write, and the
// pending list and the public lookup by name in deniedReads.
func RegisterRoutes(api *echo.Group, db *bun.DB, authService *auth.Service, authMiddleware *auth.Middleware) {
	h := &handler{service: NewService(db), authService: authService}

	admin := api.Group("/invite-links",
		authMiddleware.Authenticate,
		authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
	admin.GET("", h.list)
	admin.POST("", h.create)
	admin.POST("/:id/revoke", h.revoke)

	public := api.Group("/invites")
	public.GET("/:token", h.lookup)
	public.POST("/:token/accept", h.accept)
}
