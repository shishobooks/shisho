package opds

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all OPDS routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, authMiddleware *auth.Middleware, cache *downloadcache.Cache, bookService *books.Service) {
	opdsService := NewService(db, bookService)
	deviceService := devices.NewService(db)

	h := &handler{
		opdsService:     opdsService,
		bookService:     bookService,
		libraryService:  libraries.NewService(db),
		listsService:    lists.NewService(db),
		downloadCache:   cache,
		settingsService: settings.NewService(db),
		deliveryService: delivery.NewService(db),
		deviceService:   deviceService,
	}

	// Every OPDS route requires Basic Auth and books:read, the permission
	// Kobo and eReader Devices require of their owners too. Basic Auth takes
	// the account password or an OPDS app Device's credential.
	basicAuth := devices.NewMiddleware(deviceService).OPDSAuth(authMiddleware.BasicAuth)
	requireBooksRead := authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead)

	// One feed tree. What each feed lists and links follows the signed-in
	// Device's Delivery Rules and scope, or the user's default Delivery
	// Rules when signed in with the account password.
	v1 := e.Group("/opds/v1", basicAuth, requireBooksRead)

	// Root catalog - lists libraries
	v1.GET("/catalog", h.catalog)

	// Library catalog and browsing
	v1.GET("/libraries/:libraryID", h.libraryCatalog)
	v1.GET("/libraries/:libraryID/all", h.libraryAllBooks)
	v1.GET("/libraries/:libraryID/series", h.librarySeriesList)
	v1.GET("/libraries/:libraryID/series/:seriesID", h.librarySeriesBooks)
	v1.GET("/libraries/:libraryID/authors", h.libraryAuthorsList)
	v1.GET("/libraries/:libraryID/authors/:authorName", h.libraryAuthorBooks)

	// Search within library
	v1.GET("/libraries/:libraryID/search", h.librarySearch)
	v1.GET("/libraries/:libraryID/opensearch.xml", h.libraryOpenSearch)

	// The user's own and shared lists, across every library they can access
	v1.GET("/lists", h.lists)
	v1.GET("/lists/:listID", h.listBooks)
	v1.GET("/lists/:listID/search", h.listSearch)
	v1.GET("/lists/:listID/opensearch.xml", h.listOpenSearch)

	// Book covers stay under /opds for Basic Auth. The /api/books routes
	// require session authentication, while bare /books paths serve the SPA.
	v1.GET("/books/:id/cover", h.bookCover)

	// File downloads, at "{id}.{extension}" (DownloadURL). Also requires
	// Basic Auth and books:read. HEAD is registered alongside GET so OPDS
	// clients (e.g., KOReader's "Use server filenames" mode) can read the
	// Content-Disposition filename without fetching the body.
	e.GET("/opds/download/:file", h.download, basicAuth, requireBooksRead)
	e.HEAD("/opds/download/:file", h.download, basicAuth, requireBooksRead)
}
