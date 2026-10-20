package kobo

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all Kobo sync routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, downloadCache *downloadcache.Cache, bookService *books.Service) {
	deviceService := devices.NewService(db)
	syncService := NewService(db)

	deviceAuth := devices.NewMiddleware(deviceService).DeviceAuth(devices.ConnectionKoboSync)
	h := newHandler(syncService, bookService, downloadCache)

	// /kobo/:credential/v1/... syncs the Device's scope.
	registerKoboEndpoints(e.Group("/kobo/:credential", deviceAuth), h)

	// The sync URLs API keys used, with a scope segment. See legacyURLScope.
	registerKoboEndpoints(e.Group("/kobo/:credential/all", deviceAuth, legacyURLScope(deviceService, devices.ScopeAll)), h)
	registerKoboEndpoints(e.Group("/kobo/:credential/library/:scopeId", deviceAuth, legacyURLScope(deviceService, devices.ScopeLibrary)), h)
	registerKoboEndpoints(e.Group("/kobo/:credential/list/:scopeId", deviceAuth, legacyURLScope(deviceService, devices.ScopeList)), h)
}

// registerKoboEndpoints registers the Kobo API endpoints on a group.
func registerKoboEndpoints(g *echo.Group, h *handler) {
	g.GET("/v1/initialization", h.handleInitialization)
	g.POST("/v1/auth/device", h.handleAuth)
	g.GET("/v1/library/sync", h.handleSync)
	g.GET("/v1/library/:bookId/metadata", h.handleMetadata)
	// Download routes - support both Komga style and Calibre-web style
	g.GET("/v1/books/:bookId/file/epub", h.handleDownload)
	g.HEAD("/v1/books/:bookId/file/epub", h.handleDownload)
	g.GET("/download/:bookId/kepub", h.handleDownload) // Calibre-web style
	g.HEAD("/download/:bookId/kepub", h.handleDownload)
	g.GET("/v1/books/:imageId/thumbnail/:w/:h/*", h.handleCover)

	// Catch-all: proxy unhandled requests to Kobo store
	g.Any("/v1/*", proxyToKoboStore)
}
