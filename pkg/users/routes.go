package users

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all user routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) *Service {
	userService := NewService(db)

	h := &handler{
		userService: userService,
	}

	users := e.Group("/users")

	// All user routes require authentication
	users.Use(authMiddleware.Authenticate)

	// Read routes require users:read permission
	users.GET("", h.list, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))
	users.GET("/:id", h.retrieve, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))

	// Write routes require users:write permission
	users.POST("", h.create, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
	users.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))
	users.DELETE("/:id", h.deactivate, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationWrite))

	// Password reset is special - you can reset your own password without users:write
	// The handler checks permissions internally for non-self resets
	// RequirePermission is used here just to load the user into context (permission check done in handler)
	users.POST("/:id/reset-password", h.resetPassword, authMiddleware.RequirePermission(models.ResourceUsers, models.OperationRead))

	return userService
}
