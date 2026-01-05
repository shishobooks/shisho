package opds

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all OPDS routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware) {
	opdsService := NewService(db)
	bookService := books.NewService(db)
	cache := downloadcache.NewCache(cfg.DownloadCacheDir, cfg.DownloadCacheMaxSizeBytes())

	h := &handler{
		opdsService:   opdsService,
		bookService:   bookService,
		downloadCache: cache,
	}

	// OPDS 1.2 routes with file type parameter
	// File types can be: epub, cbz, m4b, or combinations like epub+cbz
	// All OPDS routes require Basic Auth
	v1 := e.Group("/opds/v1", authMiddleware.BasicAuth)

	// Root catalog - lists libraries
	v1.GET("/:types/catalog", h.catalog)

	// Library catalog and browsing
	v1.GET("/:types/libraries/:libraryID", h.libraryCatalog)
	v1.GET("/:types/libraries/:libraryID/all", h.libraryAllBooks)
	v1.GET("/:types/libraries/:libraryID/series", h.librarySeriesList)
	v1.GET("/:types/libraries/:libraryID/series/:seriesID", h.librarySeriesBooks)
	v1.GET("/:types/libraries/:libraryID/authors", h.libraryAuthorsList)
	v1.GET("/:types/libraries/:libraryID/authors/:authorName", h.libraryAuthorBooks)

	// Search within library
	v1.GET("/:types/libraries/:libraryID/search", h.librarySearch)
	v1.GET("/:types/libraries/:libraryID/opensearch.xml", h.libraryOpenSearch)

	// File downloads (version-agnostic, shared across OPDS versions)
	// Also requires Basic Auth
	e.GET("/opds/download/:id", h.download, authMiddleware.BasicAuth)
}
