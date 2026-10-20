package ereader

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers all eReader routes.
func RegisterRoutes(e *echo.Echo, db *bun.DB, downloadCache *downloadcache.Cache, bookService *books.Service) {
	deviceService := devices.NewService(db)
	libraryService := libraries.NewService(db)
	seriesService := series.NewService(db)
	peopleService := people.NewService(db)

	mw := devices.NewMiddleware(deviceService)
	h := newHandler(libraryService, bookService, seriesService, peopleService, downloadCache, settings.NewService(db), lists.NewService(db), deviceService)

	// Short URL resolution (no auth required - the short code IS the auth)
	e.GET("/e/:shortCode", func(c echo.Context) error {
		return ResolveShortURL(c, deviceService)
	})

	// eReader browser UI, authenticated by the Device credential in the URL
	ereader := e.Group("/ereader/key/:credential", mw.DeviceAuth(devices.ConnectionEReaderBrowser))

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
	ereader.HEAD("/file/:fileId", h.DownloadFile)
	ereader.GET("/file/:fileId/:deliveredType", h.DownloadFileAs)
	ereader.HEAD("/file/:fileId/:deliveredType", h.DownloadFileAs)
	ereader.GET("/file/:fileId/kindle/:filename", h.DownloadFileKindle)
	ereader.HEAD("/file/:fileId/kindle/:filename", h.DownloadFileKindle)
}
