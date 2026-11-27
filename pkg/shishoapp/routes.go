package shishoapp

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/bookmarks"
	"github.com/shishobooks/shisho/pkg/changefeed"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/lists"
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
		db:        db,
		books:     opts.Books,
		downloads: opts.Downloads,
		auth:      authService,
		devices:   deviceService,
		feed:      changefeed.NewService(db),
		reading:   reading.NewService(db),
		bookmarks: bookmarks.NewService(db),
		lists:     lists.NewService(db),
		contract:  opts.Contract,
		opts:      opts,
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
	// Both ways of syncing check the instance first, so a server set up
	// again says so rather than only that the Device is unknown.
	g.GET("/changes", h.changes, h.requireInstance, signedIn, h.requireProtocol, h.recordAppVersion)
	g.GET("/books/:id/cover", h.cover, signedIn, h.requireProtocol)
	g.GET("/files/:id/cover", h.fileCover, signedIn, h.requireProtocol)
	g.POST("/journal", h.journal, h.requireInstance, signedIn, h.requireProtocol, h.recordAppVersion)
	// The user's Reading Stats, which the app keeps a copy of to show offline.
	g.GET("/stats", h.stats, signedIn, h.requireProtocol)
	// Reading Stats' Calendar, a day at a time.
	g.GET("/stats/heatmap", h.statsHeatmap, signedIn, h.requireProtocol)
	g.GET("/stats/calendar", h.statsCalendar, signedIn, h.requireProtocol)
	g.GET("/stats/day", h.statsDay, signedIn, h.requireProtocol)
	// Original Files (ADR 0022), with Range support so a download resumes
	// and a stream seeks.
	g.GET("/files/:id/download", h.download, signedIn, h.requireProtocol)
	g.HEAD("/files/:id/download", h.download, signedIn, h.requireProtocol)
	g.GET("/files/:id/stream", h.stream, streamCookies, signedIn, h.requireProtocol)
	g.HEAD("/files/:id/stream", h.stream, streamCookies, signedIn, h.requireProtocol)
	// A File's Generated File, for sharing out of the app with Shisho's
	// current metadata.
	g.GET("/files/:id/share", h.share, signedIn, h.requireProtocol)
	g.HEAD("/files/:id/share", h.share, signedIn, h.requireProtocol)
	// A CBZ's or PDF's pages, as thumbnails for the page grid of a File
	// that isn't downloaded. The reader draws its pages from the downloaded
	// File (ADR 0025).
	g.GET("/files/:id/pages/:page", h.page, signedIn, h.requireProtocol)
}
