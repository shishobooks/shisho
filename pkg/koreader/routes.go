// Package koreader serves KOReader's progress sync protocol, so KOReader
// keeps its place through Shisho as a custom sync server. KOReader names a
// document by a hash of its bytes (ADR 0014).
package koreader

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/reach"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the protocol under the prefix users enter as
// KOReader's custom sync server.
func RegisterRoutes(e *echo.Echo, db *bun.DB, bookService *books.Service) {
	deviceService := devices.NewService(db)
	h := &handler{
		deviceService:      deviceService,
		fingerprintService: fingerprints.NewService(db),
		readingService:     reading.NewService(db),
		reachService:       reach.NewService(db, bookService),
	}

	g := e.Group("/koreader")
	g.POST("/users/create", h.register)

	authed := g.Group("", h.authenticate)
	authed.GET("/users/auth", h.authorize)
	authed.PUT("/syncs/progress", h.updateProgress)
	authed.GET("/syncs/progress/:document", h.retrieveProgress)
}
