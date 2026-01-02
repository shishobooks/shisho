package filesystem

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutesWithAuth registers filesystem routes with authentication.
func RegisterRoutesWithAuth(e *echo.Echo, authMiddleware *auth.Middleware) {
	filesystemService := NewService()

	h := &handler{
		filesystemService: filesystemService,
	}

	fsGroup := e.Group("/filesystem")
	fsGroup.Use(authMiddleware.Authenticate)
	// Filesystem browsing requires libraries:write permission (used when creating/editing libraries)
	fsGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	fsGroup.GET("/browse", h.browse)
}
