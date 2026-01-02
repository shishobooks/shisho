package config

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
)

// RegisterRoutesWithAuth registers config routes with authentication.
func RegisterRoutesWithAuth(e *echo.Echo, cfg *Config, authMiddleware *auth.Middleware) {
	configService := NewService(cfg)
	h := &handler{configService: configService}

	configGroup := e.Group("/config")
	configGroup.Use(authMiddleware.Authenticate)
	configGroup.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	configGroup.GET("", h.retrieve)
}
