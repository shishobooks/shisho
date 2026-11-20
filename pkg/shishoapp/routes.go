package shishoapp

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/events"
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
}
