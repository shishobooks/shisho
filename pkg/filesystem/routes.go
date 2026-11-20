package filesystem

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
)

// RegisterRoutes registers filesystem routes with authentication. pm, which
// may be nil, lets detection count the file types plugins add.
func RegisterRoutes(e *echo.Group, authMiddleware *auth.Middleware, pm *plugins.Manager) {
	filesystemService := NewService()

	h := &handler{
		filesystemService: filesystemService,
		detectBudget:      defaultDetectLimits,
	}
	// Only set plugins if it's not nil to avoid an interface holding a nil
	// pointer.
	if pm != nil {
		h.plugins = pm
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
