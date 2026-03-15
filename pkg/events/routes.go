package events

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
)

func RegisterRoutes(e *echo.Echo, broker *Broker, authMiddleware *auth.Middleware) {
	h := &handler{broker: broker}
	e.GET("/events", h.stream, authMiddleware.Authenticate)
}
