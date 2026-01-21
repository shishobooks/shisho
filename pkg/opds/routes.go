package opds

import (
	"path/filepath"

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
	cache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())

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

	// KePub routes - same structure but downloads as KePub format
	// These routes generate feeds with KePub download links for EPUB and CBZ files
	v1Kepub := e.Group("/opds/v1/kepub", authMiddleware.BasicAuth)

	v1Kepub.GET("/:types/catalog", h.catalogKepub)
	v1Kepub.GET("/:types/libraries/:libraryID", h.libraryCatalogKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/all", h.libraryAllBooksKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/series", h.librarySeriesListKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/series/:seriesID", h.librarySeriesBooksKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/authors", h.libraryAuthorsListKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/authors/:authorName", h.libraryAuthorBooksKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/search", h.librarySearchKepub)
	v1Kepub.GET("/:types/libraries/:libraryID/opensearch.xml", h.libraryOpenSearchKepub)

	// File downloads (version-agnostic, shared across OPDS versions)
	// Also requires Basic Auth
	e.GET("/opds/download/:id", h.download, authMiddleware.BasicAuth)
	e.GET("/opds/download/:id/kepub", h.downloadKepub, authMiddleware.BasicAuth)
}
