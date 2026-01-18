package ereader

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all eReader routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, downloadCache *downloadcache.Cache) {
	apiKeyService := apikeys.NewService(db)
	libraryService := libraries.NewService(db)
	bookService := books.NewService(db)
	seriesService := series.NewService(db)
	peopleService := people.NewService(db)

	mw := NewMiddleware(apiKeyService)
	h := newHandler(db, libraryService, bookService, seriesService, peopleService, downloadCache)

	// Short URL resolution (no auth required - the short code IS the auth)
	e.GET("/e/:shortCode", func(c echo.Context) error {
		return ResolveShortURL(c, apiKeyService)
	})

	// eReader browser UI with API key auth
	ereader := e.Group("/ereader/key/:apiKey", mw.APIKeyAuth(apikeys.PermissionEReaderBrowser))

	ereader.GET("/", h.Libraries)
	ereader.GET("/libraries/:libraryId", h.LibraryNav)
	ereader.GET("/libraries/:libraryId/all", h.LibraryAllBooks)
	ereader.GET("/libraries/:libraryId/series", h.LibrarySeries)
	ereader.GET("/libraries/:libraryId/series/:seriesId", h.SeriesBooks)
	ereader.GET("/libraries/:libraryId/authors", h.LibraryAuthors)
	ereader.GET("/libraries/:libraryId/authors/:authorId", h.AuthorBooks)
	ereader.GET("/libraries/:libraryId/search", h.LibrarySearch)
	ereader.GET("/download/:bookId", h.Download)
	ereader.GET("/cover/:bookId", h.Cover)
	ereader.GET("/file/:fileId", h.DownloadFile)
	ereader.GET("/file/:fileId/kepub", h.DownloadFileKepub)
}
