package audnexus

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutes wires the audnexus endpoint into the Echo instance. The
// endpoint is scoped to authenticated users with books:write (the only
// legitimate use is staging data into an editable chapter form).
func RegisterRoutes(e *echo.Echo, svc *Service, authMiddleware *auth.Middleware) {
	g := e.Group("/audnexus")
	g.Use(authMiddleware.Authenticate)
	g.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))

	h := &handler{service: svc}
	g.GET("/books/:asin/chapters", h.getChapters)
}
