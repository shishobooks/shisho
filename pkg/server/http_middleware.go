package server

import (
	"fmt"
	"mime"
	"net/netip"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/robinjoseph08/golib/logger"
)

// requestLoggerMiddleware preserves the request context and fields used by
// golib's logger, but omits successful frontend requests from the access log.
func requestLoggerMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		started := time.Now()
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		c.Set("id", id.String())
		version := c.Request().Header.Get("X-Version")
		if version == "" {
			version = "unknown"
		}
		log := logger.NewWithLevel(c.Request().Header.Get("X-Log-Level")).ID(id.String()).Root(logger.Data{
			"method": c.Request().Method, "path": c.Request().URL.Path,
			"route": c.Path(), "version": version,
		})
		c.SetRequest(c.Request().WithContext(log.WithContext(c.Request().Context())))
		if err := next(c); err != nil {
			c.Error(err)
		}
		if (c.Path() == "/*" || c.Path() == "/") && c.Response().Status < 400 {
			return nil
		}
		logger.FromContext(c.Request().Context()).Root(logger.Data{
			"status_code": c.Response().Status,
			"duration":    fmt.Sprintf("%.5f", time.Since(started).Seconds()*1000),
			"referer":     c.Request().Referer(), "user_agent": c.Request().UserAgent(),
		}).Info("request handled")
		return nil
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
			case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".woff", ".woff2", ".zip", ".epub", ".cbz", ".mp3", ".mp4", ".m4b", ".pdf", ".gz":
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
		ip := peer.Addr().Unmap()
		if err != nil || (!ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast()) {
			for _, name := range []string{"X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Prefix", "X-Forwarded-For"} {
				c.Request().Header.Del(name)
			}
		}
		return next(c)
	}
}
