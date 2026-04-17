package logs

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes registers the log viewer endpoint.
func RegisterRoutes(e *echo.Echo, buffer *RingBuffer, authMiddleware *auth.Middleware) {
	h := &handler{buffer: buffer}
	e.GET("/logs", h.listLogs,
		authMiddleware.Authenticate,
		authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead),
	)
}
