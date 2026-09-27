package auth

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers all auth routes. The server builds authService,
// because the auth middleware every other route family uses wraps it too.
func RegisterRoutes(e *echo.Group, authService *Service, demoMode bool) {
	h := &handler{
		authService: authService,
		demoMode:    demoMode,
	}

	auth := e.Group("/auth")
	auth.POST("/login", h.login)
	auth.POST("/logout", h.logout)
	auth.GET("/status", h.status)
	auth.POST("/setup", h.setup)

	// /auth/me requires authentication - will be added via middleware
	auth.GET("/me", h.me)
}
