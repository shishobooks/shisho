package server

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/reading"
)

// deniedDownloads are the explicit download routes. They stay closed to HEAD
// as well as GET, since a HEAD still runs the handler and can start a build.
// The generated download, pages, and audio streaming stay open and still hand
// out complete files, so this is not copy protection, and supplements are
// reachable through the generated download: the demo corpus must not include
// them.
var deniedDownloads = map[string]bool{
	"/api/books/files/:id/download/original":  true,
	"/api/books/files/:id/download/:fileType": true,
	"/api/jobs/:id/download":                  true,
}

// demoModeMiddleware must be registered with Use, not Pre, so Path contains
// the matched route pattern. This boundary applies before authentication too.
func demoModeMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		switch c.Request().Method {
		case http.MethodGet, http.MethodHead:
			if deniedDownloads[c.Path()] {
				return errcodes.DemoMode()
			}
			return next(c)
		case http.MethodOptions:
			return next(c)
		case http.MethodPost:
			if c.Path() == "/api/auth/login" || c.Path() == "/api/auth/logout" {
				return next(c)
			}
		}
		return errcodes.DemoMode()
	}
}

// demoDateShiftMiddleware moves the demo's reading dates on the first
// request of each UTC day, so the latest reading stays on yesterday while a
// suspended Machine resumes without restarting. The shift finishes even if
// the request that started it is canceled, since it won't run again that
// day. Stale dates beat no demo, so a failure only logs.
func demoDateShiftMiddleware(shifter *reading.DemoDateShifter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			logDemoDateShift(shifter.Refresh(context.WithoutCancel(c.Request().Context())))
			return next(c)
		}
	}
}

// logDemoDateShift logs a shift that moved something or failed, and keeps
// the quiet requests of the rest of the day out of the log.
func logDemoDateShift(days int, err error) {
	switch {
	case err != nil:
		logger.New().Err(err).Error("failed to shift demo reading dates")
	case days != 0:
		logger.New().Info("shifted demo reading dates", logger.Data{"days": days})
	}
}
