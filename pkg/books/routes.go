package books

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/uptrace/bun"
)

// RegisterLibraryRoutes registers per-library book routes on a libraries group.
// These routes are mounted under /libraries/:id/... alongside library management routes.
func RegisterLibraryRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	bookService := NewService(db)
	settingsService := settings.NewService(db)
	h := &handler{bookService: bookService, settingsService: settingsService}
	g.GET("/:id/languages", h.listLibraryLanguages, authMiddleware.RequireLibraryAccess("id"))
}

// RegisterRoutesWithGroup registers book routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, scanner Scanner, pm *plugins.Manager, dlCache *downloadcache.Cache, appSettingsSvc *appsettings.Service) {
	bookService := NewService(db).WithAppSettings(appSettingsSvc)
	libraryService := libraries.NewService(db)
	personService := people.NewService(db)
	searchService := search.NewService(db)
	genreService := genres.NewService(db)
	tagService := tags.NewService(db)
	publisherService := publishers.NewService(db)
	imprintService := imprints.NewService(db)
	listsService := lists.NewService(db)
	settingsService := settings.NewService(db)
	pageCache := cbzpages.NewCache(cfg.CacheDir)
	pdfPageCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)

	h := &handler{
		config:             cfg,
		bookService:        bookService,
		libraryService:     libraryService,
		personService:      personService,
		searchService:      searchService,
		genreService:       genreService,
		tagService:         tagService,
		publisherService:   publisherService,
		imprintService:     imprintService,
		listsService:       listsService,
		settingsService:    settingsService,
		appSettingsService: appSettingsSvc,
		downloadCache:      dlCache,
		pageCache:          pageCache,
		pdfPageCache:       pdfPageCache,
		scanner:            scanner,
	}
	// Only set pluginManager if it's not nil to avoid interface holding nil pointer
	if pm != nil {
		h.pluginManager = pm
	}

	// Merge books - must be before /:id routes
	g.POST("/merge", h.mergeBooks, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))

	// Bulk delete books - must be before /:id routes
	g.POST("/delete", h.deleteBooks, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))

	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("libraryId"))
	g.DELETE("/:id", h.deleteBook, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("", h.list)
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/:id/resync", h.resyncBook, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	// Move files between books
	g.POST("/:id/move-files", h.moveFiles, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.GET("/:id/cover", h.bookCover)
	g.GET("/:id/lists", h.bookLists)
	g.POST("/:id/lists", h.updateBookLists)
	g.PUT("/:id/primary-file", h.setPrimaryFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
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
	g.PATCH("/files/:id/review", h.setFileReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.DELETE("/files/:id", h.deleteFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.PATCH("/:id/review", h.setBookReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	g.POST("/bulk/review", h.bulkSetReview, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
