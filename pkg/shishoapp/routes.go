package shishoapp

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/uptrace/bun"
)

// RegisterRoutes registers the Shisho app's Device surface under /mobile, at
// the root beside the other Device families. It's registered in Demo Mode
// too, where the middleware refuses signing in like every other write, so
// the app hears why. Each Device's first connection is announced on
// broker.
func RegisterRoutes(e *echo.Echo, db *bun.DB, authService *auth.Service, broker *events.Broker, opts Options) {
	deviceService := devices.NewService(db)
	h := &handler{
		db:       db,
		auth:     authService,
		devices:  deviceService,
		feed:     changefeed.NewService(db),
		reading:  reading.NewService(db),
		contract: opts.Contract,
		opts:     opts,
	}
	registerRoutes(e, h, devices.NewMiddleware(deviceService, broker))
}

func registerRoutes(e *echo.Echo, h *handler, mw *devices.Middleware) {
	signedIn := mw.BearerAuth(devices.ConnectionShishoApp)
	// Middleware is per route, because a group's own middleware would add
	// catch-all routes for the group's paths.
	g := e.Group("/mobile")
	g.GET("/server", h.server)
	g.POST("/sign-in", h.signIn)
	g.DELETE("/device", h.signOut, signedIn)
	g.GET("/changes", h.changes, signedIn, h.requireProtocol, h.recordAppVersion)
	g.GET("/books/:id/cover", h.cover, signedIn, h.requireProtocol)
	g.GET("/files/:id/cover", h.fileCover, signedIn, h.requireProtocol)
	g.POST("/journal", h.journal, signedIn, h.requireProtocol, h.recordAppVersion)
	// Original Files (ADR 0022), with Range support so a download resumes
	// and a stream seeks.
	g.GET("/files/:id/download", h.download, signedIn, h.requireProtocol)
	g.HEAD("/files/:id/download", h.download, signedIn, h.requireProtocol)
	g.GET("/files/:id/stream", h.stream, streamCookies, signedIn, h.requireProtocol)
	g.HEAD("/files/:id/stream", h.stream, streamCookies, signedIn, h.requireProtocol)
}
