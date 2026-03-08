package plugins

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

// EnrichDeps holds optional dependencies for the enrich endpoint.
// If nil, the enrich endpoint returns metadata without persisting it.
type EnrichDeps struct {
	BookStore     bookStore
	RelStore      relationStore
	IdentStore    identifierStore
	PersonFinder  personFinder
	GenreFinder   genreFinder
	TagFinder     tagFinder
	SearchIndexer searchIndexer
}

// RegisterRoutesWithGroup registers plugin management API routes.
func RegisterRoutesWithGroup(g *echo.Group, service *Service, manager *Manager, installer *Installer, db *bun.DB, ed *EnrichDeps) {
	h := &handler{service: service, manager: manager, installer: installer, db: db}
	if ed != nil {
		h.enrich = &enrichDeps{
			bookStore:     ed.BookStore,
			relStore:      ed.RelStore,
			identStore:    ed.IdentStore,
			personFinder:  ed.PersonFinder,
			genreFinder:   ed.GenreFinder,
			tagFinder:     ed.TagFinder,
			searchIndexer: ed.SearchIndexer,
		}
	}

	g.GET("/identifier-types", h.listIdentifierTypes)
	g.GET("/installed", h.listInstalled)
	g.POST("/installed", h.install)
	g.POST("/scan", h.scan)
	g.DELETE("/installed/:scope/:id", h.uninstall)
	g.PATCH("/installed/:scope/:id", h.update)
	g.GET("/installed/:scope/:id/config", h.getConfig)
	g.GET("/installed/:scope/:id/fields", h.getFieldSettings)
	g.PUT("/installed/:scope/:id/fields", h.setFieldSettings)
	g.GET("/installed/:scope/:id/image", h.getImage)
	g.POST("/installed/:scope/:id/reload", h.reload)
	g.POST("/installed/:scope/:id/update", h.updateVersion)
	g.GET("/order/:hookType", h.getOrder)
	g.PUT("/order/:hookType", h.setOrder)

	g.GET("/repositories", h.listRepositories)
	g.POST("/repositories", h.addRepository)
	g.DELETE("/repositories/:scope", h.removeRepository)
	g.POST("/repositories/:scope/sync", h.syncRepository)

	g.POST("/search", h.searchMetadata)
	g.POST("/enrich", h.enrichMetadata)

	g.GET("/available", h.listAvailable)
	g.GET("/available/:scope/:id", h.retrieveAvailable)
}

// RegisterLibraryRoutes registers per-library plugin order routes on a libraries group.
func RegisterLibraryRoutes(g *echo.Group, service *Service, manager *Manager, authMiddleware *auth.Middleware) {
	h := &handler{service: service, manager: manager}

	g.GET("/:id/plugins/order/:hookType", h.getLibraryOrder, authMiddleware.RequireLibraryAccess("id"))
	g.PUT("/:id/plugins/order/:hookType", h.setLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id/plugins/order/:hookType", h.resetLibraryOrder, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id/plugins/order", h.resetAllLibraryOrders, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))

	g.GET("/:id/plugins/:scope/:pluginId/fields", h.getLibraryFieldSettings, authMiddleware.RequireLibraryAccess("id"))
	g.PUT("/:id/plugins/:scope/:pluginId/fields", h.setLibraryFieldSettings, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
	g.DELETE("/:id/plugins/:scope/:pluginId/fields", h.resetLibraryFieldSettings, authMiddleware.RequirePermission(models.ResourceLibraries, models.OperationWrite), authMiddleware.RequireLibraryAccess("id"))
}
