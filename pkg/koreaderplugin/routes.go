package koreaderplugin

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the KOReader plugin's API at the root, beside
// KOReader's progress sync. Every route is a POST, so Demo Mode refuses
// them all as writes. The address limiter on pairing is this family's own.
func RegisterRoutes(e *echo.Echo, db *bun.DB, bundle *Bundle, bookService *books.Service) {
	h := newHandler(db, bundle, bookService)
	registerPluginRoutes(e, h, devices.NewMiddleware(h.devices))
}

func registerPluginRoutes(e *echo.Echo, h *handler, mw *devices.Middleware) {
	signIn := mw.BasicAuth(devices.ConnectionKOReaderPlugin)
	g := e.Group("/koreader/plugin")
	g.POST("/pair", h.pair, h.requireProtocol)
	// Signed in first, so only a Device's owner learns what a refused
	// plugin should update to.
	g.POST("/status", h.status, signIn, h.requireProtocol)
	g.POST("/download", h.download, signIn)
	g.POST("/upload", h.upload, signIn, h.requireProtocol)
	g.POST("/pull", h.pull, signIn, h.requireProtocol)
}

// RegisterUserRoutes registers the plugin download on a user's own Device
// page, under the /user/devices group's path.
func RegisterUserRoutes(api *echo.Group, db *bun.DB, bundle *Bundle, bookService *books.Service, authMiddleware *auth.Middleware) {
	h := newHandler(db, bundle, bookService)
	api.POST("/user/devices/:id/koreader-plugin", h.ownerDownload, authMiddleware.Authenticate)
}
