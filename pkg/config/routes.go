package config

import (
	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo, cfg *Config) {
	configService := NewService(cfg)
	h := &handler{configService: configService}

	e.GET("/config", h.retrieve)
}
