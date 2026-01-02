package opds

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all OPDS routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB) {
	opdsService := NewService(db)
	bookService := books.NewService(db)

	h := &handler{
		opdsService: opdsService,
		bookService: bookService,
	}

	// OPDS 1.2 routes with file type parameter
	// File types can be: epub, cbz, m4b, or combinations like epub+cbz
	v1 := e.Group("/opds/v1")

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
	e.GET("/opds/download/:id", h.download)
}
