package server

import (
	"net"
	"net/http"
	"path/filepath"
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
	"github.com/shishobooks/shisho/pkg/sharelinks"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/shishobooks/shisho/pkg/users"
	"github.com/shishobooks/shisho/pkg/version"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/uptrace/bun"
)

func New(cfg *config.Config, db *bun.DB, w *worker.Worker, pluginService *plugins.Service, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache, cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache, logBuffer *logs.RingBuffer) (*http.Server, error) {
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
		// Each E2E browser runs its own server with its own cache dir, so its
		// seeded EPUBs never collide with another browser's.
		testutils.RegisterRoutes(api, db, pm, plugins.NewInstaller(cfg.PluginDir), filepath.Join(cfg.CacheDir, "e2e-epubs"))
	}

	// Services and caches that more than one route family uses are built once
	// and injected. The books service carries app settings so every mutation
	// through it recomputes Reviewed. See "Shared services" in pkg/AGENTS.md.
	// Tests may pass nil page caches and a nil plugin service. Build them
	// here, as the books routes did before the caches were injected, so page
	// and plugin routes still work.
	if pluginService == nil {
		pluginService = plugins.NewService(db)
	}
	if cbzCache == nil {
		cbzCache = cbzpages.NewCache(cfg.CacheDir)
	}
	if pdfCache == nil {
		pdfCache = pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)
	}
	svcs := sharedServices{
		appSettings: appsettings.NewService(db),
		plugins:     pluginService,
		shareLinks:  sharelinks.NewService(db),
		dlCache:     dlCache,
		cbzCache:    cbzCache,
		pdfCache:    pdfCache,
	}
	svcs.books = books.NewService(db).WithAppSettings(svcs.appSettings)

	// Register auth routes. The auth middleware wraps the same service.
	authService := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	auth.RegisterRoutes(api, authService, cfg.DemoMode)
	authMiddleware := auth.NewMiddleware(authService)

	// Register user and role management routes
	users.RegisterRoutes(api, db, authMiddleware)
	// The user directory lists every active username. In Demo Mode that
	// would show every visitor the admin's name, so it is not registered
	// there, and the path falls through to GET /users/:id (Users Read).
	if !cfg.DemoMode {
		userDirectoryGroup := api.Group("/users/directory")
		userDirectoryGroup.Use(authMiddleware.Authenticate)
		users.RegisterDirectoryRoutes(userDirectoryGroup, db)
	}
	roles.RegisterRoutes(api, db, authMiddleware)

	// API Keys routes
	apikeys.RegisterRoutes(api, db, authMiddleware)

	// Register protected API routes
	// These routes require authentication and appropriate permissions
	registerProtectedRoutes(api, db, cfg, authMiddleware, w, pm, broker, svcs)

	if !cfg.DemoMode {
		// Register OPDS routes with Basic Auth
		opds.RegisterRoutes(e, db, authMiddleware, svcs.dlCache, svcs.books)

		// Register eReader routes (API key auth for stock browser support)
		ereader.RegisterRoutes(e, db, svcs.dlCache, svcs.books)

		// Register Kobo sync routes (API key auth for Kobo device sync)
		kobo.RegisterRoutes(e, db, svcs.dlCache, svcs.books)

		// Register the anonymous Share Link recipient routes
		sharelinks.RegisterPublicRoutes(api, svcs.shareLinks, svcs.books, svcs.dlCache)
	}

	// Config routes (require authentication)
	config.RegisterRoutes(api, cfg, authMiddleware)

	// Filesystem routes (require authentication)
	filesystem.RegisterRoutes(api, authMiddleware)

	// Settings routes (require authentication)
	settings.RegisterRoutes(api, db, authMiddleware, svcs.appSettings)
	sharelinks.RegisterRoutes(api, authMiddleware, svcs.appSettings)

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
	cacheHandler := cache.NewHandler(svcs.dlCache, svcs.cbzCache, svcs.pdfCache)
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

// sharedServices holds the services and caches that pkg/server builds once
// and injects into every route family that needs them. The app settings,
// plugin, and Share Link services hold only the database handle but are
// shared so each has one construction site. Other database-only services
// (search, aliases, libraries, jobs, settings, API keys, and the entity
// services) are cheap and stateless, so route packages may still build those
// locally.
type sharedServices struct {
	// books carries app settings, so the chapter replace handler, the genre,
	// tag, people, series, and publisher delete handlers, and every books
	// mutation recompute Reviewed. Without app settings the recompute
	// silently does nothing.
	books       *books.Service
	appSettings *appsettings.Service
	plugins     *plugins.Service
	shareLinks  *sharelinks.Service
	dlCache     *downloadcache.Cache
	cbzCache    *cbzpages.Cache
	pdfCache    *pdfpages.Cache
}

// registerProtectedRoutes registers all protected API routes with proper authentication and authorization.
func registerProtectedRoutes(e *echo.Group, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker, pm *plugins.Manager, broker *events.Broker, svcs sharedServices) {
	bookService := svcs.books

	// Books routes
	booksGroup := e.Group("/books")
	booksGroup.Use(authMiddleware.Authenticate)
	booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	books.RegisterRoutes(booksGroup, db, cfg, authMiddleware, w, pm, svcs.dlCache, bookService, svcs.cbzCache, svcs.pdfCache)
	chapters.RegisterRoutes(booksGroup, db, authMiddleware, bookService)

	// Share Link management under /books/:id/share-links. A role may hold
	// only shares operations, so this group authenticates without Books
	// Read; each route checks its shares permission and the handlers check
	// the book's library access.
	shareLinksGroup := e.Group("/books")
	shareLinksGroup.Use(authMiddleware.Authenticate)
	sharelinks.RegisterBookRoutes(shareLinksGroup, authMiddleware, svcs.shareLinks, svcs.appSettings)

	// The caller's accessible libraries, for reader pages. Authentication
	// only: every role that can sign in can see which libraries it reaches.
	userLibrariesGroup := e.Group("/user/libraries")
	userLibrariesGroup.Use(authMiddleware.Authenticate)
	libraries.RegisterUserRoutes(userLibrariesGroup, db)

	// GET /libraries also opens to users:write, which lists libraries to
	// assign access when creating or editing a user.
	libraryListGroup := e.Group("/libraries")
	libraryListGroup.Use(authMiddleware.Authenticate)
	libraryListGroup.Use(authMiddleware.RequireAnyPermission(
		auth.Permission{Resource: models.ResourceLibraries, Operation: models.OperationRead},
		auth.Permission{Resource: models.ResourceUsers, Operation: models.OperationWrite},
	))
	libraries.RegisterListRoutes(libraryListGroup, db)

	// Libraries routes
	librariesGroup := e.Group("/libraries")
	librariesGroup.Use(authMiddleware.Authenticate)
	librariesGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationRead))
	libraries.RegisterRoutes(librariesGroup, db, authMiddleware, libraries.RegisterRoutesOptions{
		OnLibraryChanged: w.RefreshMonitorWatches,
	})
	if !cfg.DemoMode {
		plugins.RegisterLibraryRoutes(librariesGroup, svcs.plugins, pm, authMiddleware)
	}

	// Per-library book data (languages) is Books Read plus library access,
	// not Libraries Read, so it gets its own group.
	libraryBooksGroup := e.Group("/libraries")
	libraryBooksGroup.Use(authMiddleware.Authenticate)
	libraryBooksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	books.RegisterLibraryRoutes(libraryBooksGroup, db, authMiddleware, bookService)

	// Jobs routes
	jobsGroup := e.Group("/jobs")
	// Jobs permissions are per route: bulk download creators need only Books Read.
	jobsGroup.Use(authMiddleware.Authenticate)
	jobs.RegisterRoutes(jobsGroup, db, authMiddleware, broker, svcs.dlCache)
	joblogs.RegisterRoutes(jobsGroup, db, authMiddleware)

	// People routes
	peopleGroup := e.Group("/people")
	peopleGroup.Use(authMiddleware.Authenticate)
	peopleGroup.Use(authMiddleware.RequirePermission(models.ResourcePeople, models.OperationRead))
	fileOrganizer := NewFileOrganizer(db, bookService)
	people.RegisterRoutes(peopleGroup, db, authMiddleware, bookService, fileOrganizer)

	// Series routes
	seriesGroup := e.Group("/series")
	seriesGroup.Use(authMiddleware.Authenticate)
	seriesGroup.Use(authMiddleware.RequirePermission(models.ResourceSeries, models.OperationRead))
	series.RegisterRoutes(seriesGroup, db, authMiddleware, bookService)

	// Lists routes
	listsGroup := e.Group("/lists")
	listsGroup.Use(authMiddleware.Authenticate)
	lists.RegisterRoutes(listsGroup, db, authMiddleware)

	// Genres routes
	genresGroup := e.Group("/genres")
	genresGroup.Use(authMiddleware.Authenticate)
	genresGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	genres.RegisterRoutes(genresGroup, db, authMiddleware, bookService)

	// Tags routes
	tagsGroup := e.Group("/tags")
	tagsGroup.Use(authMiddleware.Authenticate)
	tagsGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	tags.RegisterRoutes(tagsGroup, db, authMiddleware, bookService)

	// Publishers routes
	publishersGroup := e.Group("/publishers")
	publishersGroup.Use(authMiddleware.Authenticate)
	publishersGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	publishers.RegisterRoutes(publishersGroup, db, authMiddleware, bookService)

	// Search routes (requires read access to books since search returns book
	// data). The handler adds the series and people sections only for roles
	// holding series:read and people:read.
	searchGroup := e.Group("/search")
	searchGroup.Use(authMiddleware.Authenticate)
	searchGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	search.RegisterRoutes(searchGroup, db)

	if cfg.DemoMode {
		return
	}

	// Plugin identify routes (editors can search/apply metadata)
	pluginService := svcs.plugins
	bookAdapter := books.NewPluginMetadataStore(bookService)
	pageExtractor := books.NewPluginPageExtractor(svcs.cbzCache, svcs.pdfCache)
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

	// Plugin management reads: the plugins page requires config:read, so
	// its reads do too.
	pluginInstaller := plugins.NewInstaller(cfg.PluginDir)
	pluginReadGroup := e.Group("/plugins")
	pluginReadGroup.Use(authMiddleware.Authenticate)
	pluginReadGroup.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationRead))
	plugins.RegisterReadRoutes(pluginReadGroup, pluginService, pm, pluginInstaller)

	// Plugins management mutations (admin only)
	pluginsGroup := e.Group("/plugins")
	pluginsGroup.Use(authMiddleware.Authenticate)
	pluginsGroup.Use(authMiddleware.RequirePermission(models.ResourceConfig, models.OperationWrite))
	plugins.RegisterRoutes(pluginsGroup, pluginService, pm, pluginInstaller, db, enrichDeps)
}

func notFoundHandler(_ echo.Context) error {
	return errcodes.NotFound("Page")
}
