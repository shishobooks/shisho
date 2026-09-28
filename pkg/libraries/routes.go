package libraries

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// RegisterRoutesOptions configures optional behaviors for library routes.
type RegisterRoutesOptions struct {
	// OnLibraryChanged is called after a library is created, has its paths
	// updated, or is deleted. Used by the monitor to refresh filesystem watches.
	OnLibraryChanged func()
}

// RegisterListRoutes registers GET /libraries on a group the server has
// already configured with authentication and either libraries:read or
// users:write, since the user forms list libraries to assign access.
func RegisterListRoutes(g *echo.Group, db *bun.DB) {
	h := &handler{libraryService: NewService(db)}
	g.GET("", h.list)
}

// RegisterUserRoutes registers GET /user/libraries on a group the server has
// already configured with authentication only. It lists the caller's
// accessible libraries as LibrarySummary rows.
func RegisterUserRoutes(g *echo.Group, db *bun.DB) {
	h := &handler{libraryService: NewService(db)}
	g.GET("", h.listForUser)
}

// RegisterRoutes registers library routes on a group the server has already
// configured with authentication and the resource's read permission. The
// list lives in RegisterListRoutes because more roles may read it.
func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, opts ...RegisterRoutesOptions) {
	libraryService := NewService(db)
	jobService := jobs.NewService(db)

	h := &handler{
		libraryService: libraryService,
		jobService:     jobService,
	}
	if len(opts) > 0 && opts[0].OnLibraryChanged != nil {
		h.onLibraryChanged = opts[0].OnLibraryChanged
	}

	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("id"))
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id", h.delete,
		authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite),
		authMiddleware.RequireLibraryAccess("id"))
}
