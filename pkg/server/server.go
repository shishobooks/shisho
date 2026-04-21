package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/echo/v4/health"
	"github.com/robinjoseph08/golib/echo/v4/middleware/logger"
	"github.com/robinjoseph08/golib/echo/v4/middleware/recovery"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/chapters"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/ereader"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/filesystem"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/kobo"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/logs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/opds"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/roles"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/shishobooks/shisho/pkg/users"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/uptrace/bun"
)

func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
	e := echo.New()

	b, err := binder.New()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	e.Binder = b

	e.Use(logger.Middleware())
	e.Use(recovery.Middleware())
	e.Use(middleware.CORS())

	health.RegisterRoutes(e)

	// Register test-only routes when in test mode
	// These endpoints allow E2E tests to set up and tear down test data
	if cfg.IsTestMode() {
		// Allow localhost download URLs so E2E tests can install the fixture
		// plugin from /test/plugins/fixture.zip. Safe because these hosts are
		// only added in test mode (ENVIRONMENT=test).
		plugins.AllowedDownloadHosts = append(plugins.AllowedDownloadHosts,
			"http://127.0.0.1:",
			"http://localhost:",
		)
		testutils.RegisterRoutes(e, db, pm, plugins.NewInstaller(cfg.PluginDir))
	}

	// Register auth routes and get the auth service
	authService := auth.RegisterRoutes(e, db, cfg.JWTSecret, cfg.SessionDuration())
	authMiddleware := auth.NewMiddleware(authService)

	// Register user and role management routes
	users.RegisterRoutes(e, db, authMiddleware)
	roles.RegisterRoutes(e, db, authMiddleware)

	// API Keys routes
	apikeys.RegisterRoutes(e, db, authMiddleware)

	// Register protected API routes
	// These routes require authentication and appropriate permissions
	registerProtectedRoutes(e, db, cfg, authMiddleware, w, pm, broker, dlCache)

	// Register OPDS routes with Basic Auth
	opds.RegisterRoutes(e, db, cfg, authMiddleware)

	// Register eReader routes (API key auth for stock browser support)
	ereader.RegisterRoutes(e, db, dlCache)

	// Register Kobo sync routes (API key auth for Kobo device sync)
	kobo.RegisterRoutes(e, db, dlCache)

	// Config routes (require authentication)
	config.RegisterRoutesWithAuth(e, cfg, authMiddleware)

	// Filesystem routes (require authentication)
	filesystem.RegisterRoutesWithAuth(e, authMiddleware)

	// Settings routes (require authentication)
	settings.RegisterRoutes(e, db, authMiddleware)

	// SSE event stream
	events.RegisterRoutes(e, broker, authMiddleware)

	// Log viewer endpoint (admin only)
	logs.RegisterRoutes(e, logBuffer, authMiddleware)

	echo.NotFoundHandler = notFoundHandler
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:           e,
		ReadHeaderTimeout: 3 * time.Second,
	}

	return srv, nil
}

// registerProtectedRoutes registers all protected API routes with proper authentication and authorization.
func registerProtectedRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) {
	// Books routes
	booksGroup := e.Group("/books")
	booksGroup.Use(authMiddleware.Authenticate)
	booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	books.RegisterRoutesWithGroup(booksGroup, db, cfg, authMiddleware, w, pm, dlCache)
	chapters.RegisterRoutes(booksGroup, db, authMiddleware)

	// Libraries routes
	librariesGroup := e.Group("/libraries")
	librariesGroup.Use(authMiddleware.Authenticate)
	librariesGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationRead))
	libraries.RegisterRoutesWithGroup(librariesGroup, db, authMiddleware, libraries.RegisterRoutesOptions{
		OnLibraryChanged: w.RefreshMonitorWatches,
	})
	plugins.RegisterLibraryRoutes(librariesGroup, plugins.NewService(db), pm, authMiddleware)
	books.RegisterLibraryRoutes(librariesGroup, db, authMiddleware)

	// Jobs routes
	jobsGroup := e.Group("/jobs")
	jobsGroup.Use(authMiddleware.Authenticate)
	jobsGroup.Use(authMiddleware.RequirePermission(models.ResourceJobs, models.OperationRead))
	jobs.RegisterRoutesWithGroup(jobsGroup, db, authMiddleware, broker, dlCache)
	joblogs.RegisterRoutes(jobsGroup, db)

	// People routes
	peopleGroup := e.Group("/people")
	peopleGroup.Use(authMiddleware.Authenticate)
	peopleGroup.Use(authMiddleware.RequirePermission(models.ResourcePeople, models.OperationRead))
	fileOrganizer := NewFileOrganizer(db)
	people.RegisterRoutesWithGroup(peopleGroup, db, authMiddleware, fileOrganizer)

	// Series routes
	seriesGroup := e.Group("/series")
	seriesGroup.Use(authMiddleware.Authenticate)
	seriesGroup.Use(authMiddleware.RequirePermission(models.ResourceSeries, models.OperationRead))
	series.RegisterRoutesWithGroup(seriesGroup, db, authMiddleware)

	// Lists routes
	listsGroup := e.Group("/lists")
	listsGroup.Use(authMiddleware.Authenticate)
	lists.RegisterRoutesWithGroup(listsGroup, db, authMiddleware)

	// Genres routes
	genresGroup := e.Group("/genres")
	genresGroup.Use(authMiddleware.Authenticate)
	genresGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	genres.RegisterRoutesWithGroup(genresGroup, db, authMiddleware)

	// Tags routes
	tagsGroup := e.Group("/tags")
	tagsGroup.Use(authMiddleware.Authenticate)
	tagsGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	tags.RegisterRoutesWithGroup(tagsGroup, db, authMiddleware)

	// Publishers routes
	publishersGroup := e.Group("/publishers")
	publishersGroup.Use(authMiddleware.Authenticate)
	publishersGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	publishers.RegisterRoutesWithGroup(publishersGroup, db, authMiddleware)

	// Imprints routes
	imprintsGroup := e.Group("/imprints")
	imprintsGroup.Use(authMiddleware.Authenticate)
	imprintsGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	imprints.RegisterRoutesWithGroup(imprintsGroup, db, authMiddleware)

	// Search routes (requires read access to books since search returns book data)
	searchGroup := e.Group("/search")
	searchGroup.Use(authMiddleware.Authenticate)
	searchGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	search.RegisterRoutesWithGroup(searchGroup, db)

	// Plugin identify routes (editors can search/apply metadata)
	pluginService := plugins.NewService(db)
	bookSvc := books.NewService(db)
	bookAdapter := &bookUpdaterAdapter{svc: bookSvc}
	cbzCache := cbzpages.NewCache(cfg.CacheDir)
	pdfCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
	pageExtractor := books.NewPluginPageExtractor(cbzCache, pdfCache)
	enrichDeps := &plugins.EnrichDeps{
		BookStore:       bookAdapter,
		RelStore:        bookAdapter,
		IdentStore:      bookAdapter,
		PersonFinder:    people.NewService(db),
		GenreFinder:     genres.NewService(db),
		TagFinder:       tags.NewService(db),
		PublisherFinder: publishers.NewService(db),
		ImprintFinder:   imprints.NewService(db),
		SearchIndexer:   search.NewService(db),
		PageExtractor:   pageExtractor,
	}
	pluginIdentifyGroup := e.Group("/plugins")
	pluginIdentifyGroup.Use(authMiddleware.Authenticate)
	pluginIdentifyGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	plugins.RegisterIdentifyRoutes(pluginIdentifyGroup, pluginService, pm, enrichDeps)

	// Plugins management routes (admin only)
	pluginsGroup := e.Group("/plugins")
	pluginsGroup.Use(authMiddleware.Authenticate)
	pluginsGroup.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
	pluginInstaller := plugins.NewInstaller(cfg.PluginDir)
	plugins.RegisterRoutesWithGroup(pluginsGroup, pluginService, pm, pluginInstaller, db, enrichDeps)
}

func notFoundHandler(c echo.Context) error {
	c.SetPath("/:path")
	return errcodes.NotFound("Page")
}

// bookUpdaterAdapter adapts books.Service to plugins.BookUpdater.
type bookUpdaterAdapter struct {
	svc *books.Service
}

func (a *bookUpdaterAdapter) UpdateBook(ctx context.Context, book *models.Book, columns []string) error {
	return a.svc.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: columns})
}

func (a *bookUpdaterAdapter) RetrieveBook(ctx context.Context, bookID int) (*models.Book, error) {
	return a.svc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
}

func (a *bookUpdaterAdapter) DeleteAuthors(ctx context.Context, bookID int) error {
	return a.svc.DeleteAuthors(ctx, bookID)
}

func (a *bookUpdaterAdapter) CreateAuthor(ctx context.Context, author *models.Author) error {
	return a.svc.CreateAuthor(ctx, author)
}

func (a *bookUpdaterAdapter) DeleteBookSeries(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookSeries(ctx, bookID)
}

func (a *bookUpdaterAdapter) CreateBookSeries(ctx context.Context, bs *models.BookSeries) error {
	return a.svc.CreateBookSeries(ctx, bs)
}

func (a *bookUpdaterAdapter) FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error) {
	return a.svc.FindOrCreateSeries(ctx, name, libraryID, nameSource)
}

func (a *bookUpdaterAdapter) DeleteBookGenres(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookGenres(ctx, bookID)
}

func (a *bookUpdaterAdapter) CreateBookGenre(ctx context.Context, bg *models.BookGenre) error {
	return a.svc.CreateBookGenre(ctx, bg)
}

func (a *bookUpdaterAdapter) DeleteBookTags(ctx context.Context, bookID int) error {
	return a.svc.DeleteBookTags(ctx, bookID)
}

func (a *bookUpdaterAdapter) CreateBookTag(ctx context.Context, bt *models.BookTag) error {
	return a.svc.CreateBookTag(ctx, bt)
}

func (a *bookUpdaterAdapter) DeleteIdentifiersForFile(ctx context.Context, fileID int) (int, error) {
	return a.svc.DeleteIdentifiersForFile(ctx, fileID)
}

func (a *bookUpdaterAdapter) CreateFileIdentifier(ctx context.Context, identifier *models.FileIdentifier) error {
	return a.svc.CreateFileIdentifier(ctx, identifier)
}

func (a *bookUpdaterAdapter) UpdateFile(ctx context.Context, file *models.File, columns []string) error {
	return a.svc.UpdateFile(ctx, file, books.UpdateFileOptions{Columns: columns})
}

func (a *bookUpdaterAdapter) DeleteNarratorsForFile(ctx context.Context, fileID int) (int, error) {
	return a.svc.DeleteNarratorsForFile(ctx, fileID)
}

func (a *bookUpdaterAdapter) CreateNarrator(ctx context.Context, narrator *models.Narrator) error {
	return a.svc.CreateNarrator(ctx, narrator)
}

func (a *bookUpdaterAdapter) OrganizeBookFiles(ctx context.Context, book *models.Book) error {
	return a.svc.OrganizeBookFiles(ctx, book)
}
