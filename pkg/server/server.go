package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/echo/v4/health"
	"github.com/robinjoseph08/golib/echo/v4/middleware/recovery"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/audnexus"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/cache"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/chapters"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/ereader"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/filesystem"
	"github.com/shishobooks/shisho/pkg/frontend"
	"github.com/shishobooks/shisho/pkg/genres"
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
	"github.com/shishobooks/shisho/pkg/version"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/uptrace/bun"
)

func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
	e := echo.New()

	b, err := binder.New()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	e.Binder = b

	e.Pre(forwardedHeadersMiddleware)
	e.Use(requestLoggerMiddleware)
	e.Use(recovery.Middleware())
	e.Use(securityHeadersMiddleware)
	e.Use(compressionMiddleware())
	if cfg.DemoMode {
		e.Use(demoModeMiddleware)
	}

	health.RegisterRoutes(e)
	api := e.Group("/api")

	// Register test-only routes when in test mode
	// These endpoints allow E2E tests to set up and tear down test data
	if cfg.IsTestMode() && !cfg.DemoMode {
		// Allow localhost download URLs so E2E tests can install the fixture
		// plugin from /test/plugins/fixture.zip. Safe because these hosts are
		// only added in test mode (ENVIRONMENT=test).
		plugins.AllowedDownloadHosts = append(plugins.AllowedDownloadHosts,
			"http://127.0.0.1:",
			"http://localhost:",
		)
		testutils.RegisterRoutes(api, db, pm, plugins.NewInstaller(cfg.PluginDir))
	}

	// Register auth routes and get the auth service
	authService := auth.RegisterRoutes(api, db, cfg.JWTSecret, cfg.SessionDuration(), cfg.DemoMode)
	authMiddleware := auth.NewMiddleware(authService)

	// Register user and role management routes
	users.RegisterRoutes(api, db, authMiddleware)
	roles.RegisterRoutes(api, db, authMiddleware)

	// API Keys routes
	apikeys.RegisterRoutes(api, db, authMiddleware)

	// Register protected API routes
	// These routes require authentication and appropriate permissions
	registerProtectedRoutes(api, db, cfg, authMiddleware, w, pm, broker, dlCache, cbzCache, pdfCache)

	if !cfg.DemoMode {
		// Register OPDS routes with Basic Auth
		opds.RegisterRoutes(e, db, cfg, authMiddleware)

		// Register eReader routes (API key auth for stock browser support)
		ereader.RegisterRoutes(e, db, dlCache)

		// Register Kobo sync routes (API key auth for Kobo device sync)
		kobo.RegisterRoutes(e, db, dlCache)
	}

	// Config routes (require authentication)
	config.RegisterRoutesWithAuth(api, cfg, authMiddleware)

	// Filesystem routes (require authentication)
	filesystem.RegisterRoutesWithAuth(api, authMiddleware)

	// Settings routes (require authentication)
	settings.RegisterRoutes(api, db, authMiddleware)

	// SSE event stream
	events.RegisterRoutes(api, broker, authMiddleware)

	// Log viewer endpoint (admin only)
	logs.RegisterRoutes(api, logBuffer, authMiddleware)

	// Audnexus chapter lookup (books:write required)
	audnexusService := audnexus.NewService(audnexus.ServiceConfig{
		UserAgent: "Shisho/" + version.Version,
	})
	audnexus.RegisterRoutes(api, audnexusService, authMiddleware)

	// Cache management routes (admin only; requires config:read to list, config:write to clear)
	cacheHandler := cache.NewHandler(dlCache, cbzCache, pdfCache)
	cache.RegisterRoutes(api, cacheHandler, authMiddleware)

	// Echo's Group.Use adds authenticated not-found handlers. Unknown paths
	// must return JSON 404 without running a route family's authentication.
	for _, route := range e.Routes() {
		if route.Method == echo.RouteNotFound {
			e.RouteNotFound(route.Path, notFoundHandler)
		}
	}

	spa := echo.WrapHandler(frontend.Handler())
	e.RouteNotFound("/*", func(c echo.Context) error {
		path := c.Request().URL.Path
		for _, prefix := range []string{"/api", "/opds", "/kobo", "/ereader", "/e"} {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return notFoundHandler(c)
			}
		}
		if c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead {
			return notFoundHandler(c)
		}
		return spa(c)
	})
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.ServerHost, strconv.Itoa(cfg.ServerPort)),
		Handler:           e,
		ReadHeaderTimeout: 3 * time.Second,
	}

	return srv, nil
}

// registerProtectedRoutes registers all protected API routes with proper authentication and authorization.
func registerProtectedRoutes(e *echo.Group, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache) {
	// Books routes
	booksGroup := e.Group("/books")
	booksGroup.Use(authMiddleware.Authenticate)
	booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	books.RegisterRoutesWithGroup(booksGroup, db, cfg, authMiddleware, w, pm, dlCache, appsettings.NewService(db))
	chapters.RegisterRoutes(booksGroup, db, authMiddleware)

	// Libraries routes
	librariesGroup := e.Group("/libraries")
	librariesGroup.Use(authMiddleware.Authenticate)
	librariesGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationRead))
	libraries.RegisterRoutesWithGroup(librariesGroup, db, authMiddleware, libraries.RegisterRoutesOptions{
		OnLibraryChanged: w.RefreshMonitorWatches,
	})
	if !cfg.DemoMode {
		plugins.RegisterLibraryRoutes(librariesGroup, plugins.NewService(db), pm, authMiddleware)
	}
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
	series.RegisterRoutesWithGroup(seriesGroup, db, authMiddleware, appsettings.NewService(db))

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

	// Search routes (requires read access to books since search returns book data)
	searchGroup := e.Group("/search")
	searchGroup.Use(authMiddleware.Authenticate)
	searchGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	search.RegisterRoutesWithGroup(searchGroup, db)

	if cfg.DemoMode {
		return
	}

	// Plugin identify routes (editors can search/apply metadata)
	pluginService := plugins.NewService(db)
	appSettingsSvc := appsettings.NewService(db)
	bookSvc := books.NewService(db).WithAppSettings(appSettingsSvc)
	bookAdapter := books.NewPluginMetadataStore(bookSvc)
	pageExtractor := books.NewPluginPageExtractor(cbzCache, pdfCache)
	enrichDeps := &plugins.EnrichDeps{
		BookStore:       bookAdapter,
		RelStore:        bookAdapter,
		IdentStore:      bookAdapter,
		PersonFinder:    people.NewService(db),
		GenreFinder:     genres.NewService(db),
		TagFinder:       tags.NewService(db),
		PublisherFinder: publishers.NewService(db),
		SearchIndexer:   search.NewService(db),
		PageExtractor:   pageExtractor,
	}
	pluginIdentifyGroup := e.Group("/plugins")
	pluginIdentifyGroup.Use(authMiddleware.Authenticate)
	pluginIdentifyGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
	plugins.RegisterIdentifyRoutes(pluginIdentifyGroup, pluginService, pm, enrichDeps)

	// Read-only plugin lookups (identifier types and hook order). Book and file
	// pages read identifier types for every role and the identify dialog reads
	// hook order, so they need only books:read. Keep them out of the
	// config:write management group below.
	pluginLookupGroup := e.Group("/plugins")
	pluginLookupGroup.Use(authMiddleware.Authenticate)
	pluginLookupGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	plugins.RegisterLookupRoutes(pluginLookupGroup, pluginService)

	// Plugins management routes (admin only)
	pluginsGroup := e.Group("/plugins")
	pluginsGroup.Use(authMiddleware.Authenticate)
	pluginsGroup.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
	pluginInstaller := plugins.NewInstaller(cfg.PluginDir)
	plugins.RegisterRoutesWithGroup(pluginsGroup, pluginService, pm, pluginInstaller, db, enrichDeps)
}

func notFoundHandler(_ echo.Context) error {
	return errcodes.NotFound("Page")
}
