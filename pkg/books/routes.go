package books

import (
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/uptrace/bun"
)

// RegisterRoutesWithGroup registers book routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, scanner Scanner) {
	bookService := NewService(db)
	libraryService := libraries.NewService(db)
	personService := people.NewService(db)
	searchService := search.NewService(db)
	genreService := genres.NewService(db)
	tagService := tags.NewService(db)
	publisherService := publishers.NewService(db)
	imprintService := imprints.NewService(db)
	listsService := lists.NewService(db)
	cache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())
	pageCache := cbzpages.NewCache(cfg.CacheDir)

	h := &handler{
		bookService:      bookService,
		libraryService:   libraryService,
		personService:    personService,
		searchService:    searchService,
		genreService:     genreService,
		tagService:       tagService,
		publisherService: publisherService,
		imprintService:   imprintService,
		listsService:     listsService,
		downloadCache:    cache,
		pageCache:        pageCache,
		scanner:          scanner,
	}

	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("libraryId"))
	g.GET("", h.list)
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/resync", h.resyncBook, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("/:id/cover", h.bookCover)
	g.GET("/:id/lists", h.bookLists)
	g.POST("/:id/lists", h.updateBookLists)
	g.GET("/files/:id/cover", h.fileCover)
	g.POST("/files/:id", h.updateFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/files/:id/cover", h.uploadFileCover, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.PUT("/files/:id/cover-page", h.updateFileCoverPage, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("/files/:id/download", h.downloadFile)
	g.HEAD("/files/:id/download", h.downloadFile)
	g.GET("/files/:id/download/original", h.downloadOriginalFile)
	g.GET("/files/:id/download/kepub", h.downloadKepubFile)
	g.HEAD("/files/:id/download/kepub", h.downloadKepubFile)
	g.GET("/files/:id/page/:pageNum", h.getPage)
	g.GET("/files/:id/stream", h.streamFile)
	g.POST("/files/:id/resync", h.resyncFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
