package sharelinks

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the sharing settings endpoints under /settings.
// They live here rather than in pkg/settings because pkg/books imports
// pkg/settings, and this package imports pkg/books.
func RegisterRoutes(api *echo.Group, authMiddleware *auth.Middleware, appSettingsService *appsettings.Service) {
	h := &settingsHandler{appSettingsService: appSettingsService}

	g := api.Group("/settings")
	g.Use(authMiddleware.Authenticate)
	// GET allows either shares operation so sharers can read the policy that
	// shapes the Share Link form, and config:read so admins can render the
	// Sharing page.
	g.GET("/sharing", h.get, authMiddleware.RequireAnyPermission(
		auth.Permission{Resource: models.ResourceShares, Operation: models.OperationRead},
		auth.Permission{Resource: models.ResourceShares, Operation: models.OperationWrite},
		auth.Permission{Resource: models.ResourceConfig, Operation: models.OperationRead},
	))
	g.PUT("/sharing", h.update, authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
}

// RegisterBookRoutes registers the management routes on the books group,
// which already requires authentication and Books Read. Each route checks
// its shares operation here and the book's library access in the handler.
func RegisterBookRoutes(booksGroup *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, appSettingsService *appsettings.Service) {
	h := &handler{service: NewService(db), appSettingsService: appSettingsService}
	// Shares Write also lists, so a sharer can always copy the links they make.
	booksGroup.GET("/:id/share-links", h.list, authMiddleware.RequireAnyPermission(
		auth.Permission{Resource: models.ResourceShares, Operation: models.OperationRead},
		auth.Permission{Resource: models.ResourceShares, Operation: models.OperationWrite},
	))
	booksGroup.POST("/:id/share-links", h.create, authMiddleware.RequirePermission(models.ResourceShares, models.OperationWrite))
	booksGroup.POST("/:id/share-links/:linkId/revoke", h.revoke, authMiddleware.RequirePermission(models.ResourceShares, models.OperationWrite))
	booksGroup.DELETE("/:id/share-links/:linkId", h.delete, authMiddleware.RequirePermission(models.ResourceShares, models.OperationWrite))
}

// RegisterPublicRoutes registers the unauthenticated recipient family under
// /share/:token. The server skips it in Demo Mode.
func RegisterPublicRoutes(api *echo.Group, db *bun.DB, bookService *books.Service, dlCache *downloadcache.Cache) {
	h := &publicHandler{
		service:            NewService(db),
		bookService:        bookService,
		appSettingsService: bookService.AppSettings(),
		downloadCache:      dlCache,
	}
	g := api.Group("/share")
	g.GET("/:token", h.book)
	g.GET("/:token/cover", h.bookCover)
	g.GET("/:token/files/:fileId/cover", h.fileCover)
	g.GET("/:token/files/:fileId/download", h.download)
	g.HEAD("/:token/files/:fileId/download", h.download)
}
