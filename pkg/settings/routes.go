package settings

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware) {
	svc := NewService(db)

	userH := &handler{settingsService: svc}
	libraryH := &libraryHandler{settingsService: svc}

	g := e.Group("/settings")
	g.Use(authMiddleware.Authenticate)

	g.GET("/user", userH.getUserSettings)
	g.PUT("/user", userH.updateUserSettings)

	g.GET("/libraries/:library_id", libraryH.getLibrarySettings)
	g.PUT("/libraries/:library_id", libraryH.updateLibrarySettings)
}
