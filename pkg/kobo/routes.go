package kobo

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all Kobo sync routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, downloadCache *downloadcache.Cache) {
	apiKeyService := apikeys.NewService(db)
	bookService := books.NewService(db)
	syncService := NewService(db)

	mw := NewMiddleware(apiKeyService)
	h := newHandler(syncService, bookService, downloadCache)

	// Kobo routes with scope-based URL structure
	// "all" scope: /kobo/:apiKey/all/v1/...
	koboAll := e.Group("/kobo/:apiKey/all", mw.APIKeyAuth(), mw.ScopeParser("all"))
	registerKoboEndpoints(koboAll, h)

	// "library" scope: /kobo/:apiKey/library/:scopeId/v1/...
	koboLibrary := e.Group("/kobo/:apiKey/library/:scopeId", mw.APIKeyAuth(), mw.ScopeParser("library"))
	registerKoboEndpoints(koboLibrary, h)

	// "list" scope: /kobo/:apiKey/list/:scopeId/v1/...
	koboList := e.Group("/kobo/:apiKey/list/:scopeId", mw.APIKeyAuth(), mw.ScopeParser("list"))
	registerKoboEndpoints(koboList, h)
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
