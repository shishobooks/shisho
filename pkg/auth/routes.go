package auth

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers all auth routes. The server builds authService,
// because the auth middleware every other route family uses wraps it too.
// pdfRenderKey is reported by GET /auth/status (see StatusResponse), and so
// is demoMode, which is a bool rather than the config because pkg/config
// imports pkg/auth.
func RegisterRoutes(e *echo.Group, authService *Service, demoMode bool, pdfRenderKey string) {
	h := &handler{
		authService:  authService,
		demoMode:     demoMode,
		pdfRenderKey: pdfRenderKey,
	}

	auth := e.Group("/auth")
	auth.POST("/login", h.login)
	auth.POST("/logout", h.logout)
	auth.GET("/status", h.status)
	auth.POST("/setup", h.setup)

	// /auth/me requires authentication - will be added via middleware
	auth.GET("/me", h.me)
}
