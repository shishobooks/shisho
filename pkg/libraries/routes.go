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
	// OnLibraryChanged is called after a library is created or its paths/deletion state changes.
	// Used by the monitor to refresh filesystem watches.
	OnLibraryChanged func()
}

// RegisterRoutesWithGroup registers library routes on a pre-configured group.
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware, opts ...RegisterRoutesOptions) {
	libraryService := NewService(db)
	jobService := jobs.NewService(db)

	h := &handler{
		libraryService: libraryService,
		jobService:     jobService,
	}
	if len(opts) > 0 && opts[0].OnLibraryChanged != nil {
		h.onLibraryChanged = opts[0].OnLibraryChanged
	}

	g.GET("", h.list)
	g.GET("/:id", h.retrieve, authMiddleware.RequireLibraryAccess("id"))
	g.POST("", h.create, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite))
	g.POST("/:id", h.update, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id", h.delete,
		authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite),
		authMiddleware.RequireLibraryAccess("id"))
}
