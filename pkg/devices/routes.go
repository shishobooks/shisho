package devices

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers Device management routes: each user's own
// Devices under /user/devices, and every user's Devices under /devices for
// admins, which never return a credential.
func RegisterRoutes(e *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	h := newHandler(NewService(db), lists.NewService(db))

	// A user's own Devices need no permission.
	g := e.Group("/user/devices", authMiddleware.Authenticate)
	g.GET("", h.List)
	g.POST("", h.Create)
	g.GET("/:id", h.Retrieve)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/credential", h.RegenerateCredential)
	g.POST("/:id/short-url", h.GenerateShortURL)
	g.DELETE("/:id/kobo-sync", h.ClearKoboSync)

	admin := e.Group("/devices", authMiddleware.Authenticate)
	admin.GET("", h.ListAll, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))
	admin.DELETE("/:id", h.Revoke, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
}
