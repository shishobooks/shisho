package koreaderplugin

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
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
	// listCapabilities comes first on each route, so every response carries
	// the list, a refused sign-in included: it names only features. It's per
	// route because a group's own middleware would add catch-all routes for
	// the group's paths.
	g := e.Group("/koreader/plugin")
	g.POST("/pair", h.pair, listCapabilities, h.requireProtocol)
	// Signed in before the protocol check, so only a Device's owner learns
	// the version and build a refused plugin should update to.
	g.POST("/status", h.status, listCapabilities, signIn, h.requireProtocol)
	g.POST("/download", h.download, listCapabilities, signIn)
	g.POST("/lookup", h.lookup, listCapabilities, signIn, h.requireProtocol)
	g.POST("/upload", h.upload, listCapabilities, signIn, h.requireProtocol)
	g.POST("/pull", h.pull, listCapabilities, signIn, h.requireProtocol)
}

// RegisterUserRoutes registers the plugin download and the unrecognized
// books on a user's own Device page, under the /user/devices group's path.
// Only the Device's owner reaches them, and the unrecognized books, which
// name the library's Books, need permission to read them.
func RegisterUserRoutes(api *echo.Group, db *bun.DB, bundle *Bundle, bookService *books.Service, authMiddleware *auth.Middleware) {
	h := newHandler(db, bundle, bookService)
	api.POST("/user/devices/:id/koreader-plugin", h.ownerDownload, authMiddleware.Authenticate)
	g := api.Group("/user/devices/:id/koreader-unrecognized",
		authMiddleware.Authenticate, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	g.GET("", h.unrecognized)
	g.GET("/candidates", h.linkCandidates)
	g.POST("/dismiss-all", h.dismissAllUnrecognized)
	g.POST("/:document/link", h.linkUnrecognized)
	g.POST("/:document/unlink", h.unlinkUnrecognized)
	g.POST("/:document/dismiss", h.dismissUnrecognized)
	g.POST("/:document/restore", h.restoreUnrecognized)
	g.POST("/:document/delete-reading", h.deleteUnrecognizedReading)
}
