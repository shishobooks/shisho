package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/echo/v4/health"
	"github.com/robinjoseph08/golib/echo/v4/middleware/logger"
	"github.com/robinjoseph08/golib/echo/v4/middleware/recovery"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/filesystem"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/opds"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/roles"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/shishobooks/shisho/pkg/users"
	"github.com/uptrace/bun"
)

func New(cfg *config.Config, db *bun.DB) (*http.Server, error) {
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

	// Register auth routes and get the auth service
	authService := auth.RegisterRoutes(e, db, cfg.JWTSecret)
	authMiddleware := auth.NewMiddleware(authService)

	// Register user and role management routes
	users.RegisterRoutes(e, db, authMiddleware)
	roles.RegisterRoutes(e, db, authMiddleware)

	// Register protected API routes
	// These routes require authentication and appropriate permissions
	registerProtectedRoutes(e, db, cfg, authMiddleware)

	// Register OPDS routes with Basic Auth
	opds.RegisterRoutes(e, db, cfg, authMiddleware)

	// Config routes (require authentication)
	config.RegisterRoutesWithAuth(e, cfg, authMiddleware)

	// Filesystem routes (require authentication)
	filesystem.RegisterRoutesWithAuth(e, authMiddleware)

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
func registerProtectedRoutes(e *echo.Echo, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware) {
	// Books routes
	booksGroup := e.Group("/books")
	booksGroup.Use(authMiddleware.Authenticate)
	booksGroup.Use(authMiddleware.RequirePermission(models.ResourceBooks, models.OperationRead))
	books.RegisterRoutesWithGroup(booksGroup, db, cfg, authMiddleware)

	// Libraries routes
	librariesGroup := e.Group("/libraries")
	librariesGroup.Use(authMiddleware.Authenticate)
	librariesGroup.Use(authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationRead))
	libraries.RegisterRoutesWithGroup(librariesGroup, db, authMiddleware)

	// Jobs routes
	jobsGroup := e.Group("/jobs")
	jobsGroup.Use(authMiddleware.Authenticate)
	jobsGroup.Use(authMiddleware.RequirePermission(models.ResourceJobs, models.OperationRead))
	jobs.RegisterRoutesWithGroup(jobsGroup, db, authMiddleware)

	// People routes
	peopleGroup := e.Group("/people")
	peopleGroup.Use(authMiddleware.Authenticate)
	peopleGroup.Use(authMiddleware.RequirePermission(models.ResourcePeople, models.OperationRead))
	people.RegisterRoutesWithGroup(peopleGroup, db, authMiddleware)

	// Series routes
	seriesGroup := e.Group("/series")
	seriesGroup.Use(authMiddleware.Authenticate)
	seriesGroup.Use(authMiddleware.RequirePermission(models.ResourceSeries, models.OperationRead))
	series.RegisterRoutesWithGroup(seriesGroup, db, authMiddleware)

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

	// Search routes (requires read access to books, series, and people)
	searchGroup := e.Group("/search")
	searchGroup.Use(authMiddleware.Authenticate)
	search.RegisterRoutesWithGroup(searchGroup, db)
}

func notFoundHandler(c echo.Context) error {
	c.SetPath("/:path")
	return errcodes.NotFound("Page")
}
