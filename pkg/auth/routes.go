package auth

import (
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all auth routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, jwtSecret string) *Service {
	authService := NewService(db, jwtSecret)

	h := &handler{
		authService: authService,
	}

	auth := e.Group("/auth")
	auth.POST("/login", h.login)
	auth.POST("/logout", h.logout)
	auth.GET("/status", h.status)
	auth.POST("/setup", h.setup)

	// /auth/me requires authentication - will be added via middleware
	auth.GET("/me", h.me)

	return authService
}
