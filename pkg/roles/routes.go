package roles

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all role routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) *Service {
	roleService := NewService(db)

	h := &handler{
		roleService: roleService,
	}

	roles := e.Group("/roles")

	// All role routes require authentication
	roles.Use(authMiddleware.Authenticate)

	// Read routes require users:read permission (since roles are part of user management)
	roles.GET("", h.list, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))
	roles.GET("/:id", h.retrieve, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))

	// Write routes require users:write permission
	roles.POST("", h.create, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
	roles.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
	roles.DELETE("/:id", h.delete, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))

	return roleService
}
