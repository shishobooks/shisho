package filesystem

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers filesystem routes with authentication.
func RegisterRoutes(e *echo.Group, authMiddleware *auth.Middleware) {
	filesystemService := NewService()

	h := &handler{
		filesystemService: filesystemService,
		detectBudget:      defaultDetectLimits,
	}

	fsGroup := e.Group("/filesystem")
	fsGroup.Use(authMiddleware.Authenticate)
	// Filesystem browsing requires libraries:write permission (used when creating/editing libraries)
	fsGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	fsGroup.GET("/browse", h.browse)
	// Detection reads the same folders the picker browses, so it has the
	// picker's access rules.
	fsGroup.POST("/detect", h.detect)
}
