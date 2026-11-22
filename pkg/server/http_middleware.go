package server

import (
	"fmt"
	"mime"
	"net/http"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/httputil"
	"github.com/shishobooks/shisho/pkg/shishoapp"
)

// clientVersion is the version of what sent the request, for the access
// log. The web app sends the version of the bundle it runs, which lags the
// server's in a tab left open across an upgrade. The Shisho app sends its
// own, in a header, or in a cookie from the system player, which streams
// audio and sends no headers; it's marked as the app's. Other devices send
// nothing.
func clientVersion(r *http.Request) string {
	if version := r.Header.Get("X-Version"); version != "" {
		return version
	}
	version := r.Header.Get(shishoapp.HeaderAppVersion)
	if version == "" {
		if cookie, err := r.Cookie(shishoapp.CookieAppVersion); err == nil {
			version = cookie.Value
		}
	}
	if version != "" {
		return "app/" + version
	}
	return "unknown"
}

// requestLoggerMiddleware preserves the request context and fields used by
// golib's logger, but omits successful frontend and log viewer requests from
// the access log.
func requestLoggerMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		started := time.Now()
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		c.Set("id", id.String())
		version := clientVersion(c.Request())
		log := logger.NewWithLevel(c.Request().Header.Get("X-Log-Level")).ID(id.String()).Root(logger.Data{
			"method": c.Request().Method, "path": loggedPath(c),
			"route": c.Path(), "client_version": version,
		})
		c.SetRequest(c.Request().WithContext(log.WithContext(c.Request().Context())))
		if err := next(c); err != nil {
			c.Error(err)
		}
		// "/*" is the frontend fallback pattern server.New registers. The log
		// viewer refetches /api/logs whenever a line is logged, so logging
		// that request would feed itself and fill the ring buffer.
		switch c.Path() {
		case "/*", "/", "/api/logs":
			if c.Response().Status < 400 {
				return nil
			}
		}
		logger.FromContext(c.Request().Context()).Root(logger.Data{
			"status_code": c.Response().Status,
			"duration":    fmt.Sprintf("%.5f", time.Since(started).Seconds()*1000),
			"referer":     loggedReferer(c.Request().Referer()), "user_agent": c.Request().UserAgent(),
		}).Info("request handled")
		return nil
	}
}

// tokenPages are the app pages whose path carries a Share Link or Invite Link
// token, which works like a password.
var tokenPages = []string{"/share/", "/invite/"}

// loggedPath is the request path for the access log. A route whose path
// carries a link token logs its pattern instead, since the log viewer needs
// only Config Read and the token would let a reader use the link.
func loggedPath(c echo.Context) string {
	if strings.Contains(c.Path(), ":token") {
		return c.Path()
	}
	return c.Request().URL.Path
}

// loggedReferer drops the token from a referer on a page that carries one,
// for the same reason as loggedPath.
func loggedReferer(referer string) string {
	for _, page := range tokenPages {
		if i := strings.Index(referer, page); i >= 0 {
			return referer[:i+len(page)] + ":token"
		}
	}
	return referer
}

// buildHeader names the frontend build the server embeds. The web app compares
// it with the build it runs and offers a reload when they differ.
const buildHeader = "X-Shisho-Build"

func buildHeaderMiddleware(buildID string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if buildID != "" {
				c.Response().Header().Set(buildHeader, buildID)
			}
			return next(c)
		}
	}
}

func securityHeadersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		h := c.Response().Header()
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Response().Before(func() { h.Del("Server") })
		return next(c)
	}
}

func compressionMiddleware() echo.MiddlewareFunc {
	return middleware.GzipWithConfig(middleware.GzipConfig{
		MinLength: 1024,
		Skipper: func(c echo.Context) bool {
			route := c.Path()
			if route == "/api/events" {
				return true
			}
			// These path segments identify binary responses across the API and device
			// routes. Skip before handlers run so streaming responses never buffer.
			for _, segment := range strings.Split(route, "/") {
				switch segment {
				case "cover", "page", "download", "stream", "file", "thumbnail", "image":
					return true
				}
			}
			switch strings.ToLower(path.Ext(c.Request().URL.Path)) {
			case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".woff", ".woff2", ".zip", ".epub", ".mobi", ".azw", ".azw3", ".prc", ".cbz", ".mp3", ".mp4", ".m4b", ".m4a", ".pdf", ".gz":
				return true
			}
			// Echo checks only for the substring "gzip", ignoring q=0. Keep Vary
			// on eligible identity responses too, so caches can distinguish them.
			if !acceptsGzip(c.Request().Header.Get(echo.HeaderAcceptEncoding)) {
				c.Response().Header().Add(echo.HeaderVary, echo.HeaderAcceptEncoding)
				return true
			}
			return false
		},
	})
}

func acceptsGzip(header string) bool {
	for _, entry := range strings.Split(header, ",") {
		encoding, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err != nil || encoding != "gzip" {
			continue
		}
		quality, ok := params["q"]
		if !ok {
			return true
		}
		q, err := strconv.ParseFloat(quality, 64)
		return err == nil && q > 0 && q <= 1
	}
	return false
}

// Trust forwarding metadata only from the direct peer, never from an address
// supplied in X-Forwarded-For itself. This matches Caddy's private_ranges.
func forwardedHeadersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		peer, err := netip.ParseAddrPort(c.Request().RemoteAddr)
		if err != nil || !httputil.TrustedPeer(peer.Addr()) {
			for _, name := range []string{"X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Prefix", "X-Forwarded-For"} {
				c.Request().Header.Del(name)
			}
		}
		return next(c)
	}
}
