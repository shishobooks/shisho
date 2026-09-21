package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/robinjoseph08/golib/logger"
	"github.com/stretchr/testify/assert"
)

func TestRequestLoggingSkipsSuccessfulFrontendResponses(t *testing.T) {
	// This test changes the process-wide logger output.
	var output bytes.Buffer
	previous := logger.Output()
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetOutput(previous) })
	e := echo.New()
	e.Use(requestLoggerMiddleware)
	e.GET("/*", func(c echo.Context) error {
		if c.Request().URL.Path == "/broken" {
			return echo.ErrInternalServerError
		}
		return c.String(http.StatusOK, "frontend")
	})
	e.GET("/api/books", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	for _, path := range []string{"/assets/app.js", "/library/42", "/api/books", "/broken"} {
		output.Reset()
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if path == "/api/books" || path == "/broken" {
			assert.Contains(t, output.String(), "request handled", path)
			assert.Contains(t, output.String(), path)
		} else {
			assert.Empty(t, output.String(), path)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()
	e := echo.New()
	e.Use(securityHeadersMiddleware)
	e.GET("/", func(c echo.Context) error {
		c.Response().Header().Set("Server", "secret")
		return c.NoContent(http.StatusOK)
	})
	for _, path := range []string{"/", "/missing"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, "SAMEORIGIN", rec.Header().Get("X-Frame-Options"))
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
		assert.Empty(t, rec.Header().Get("Server"))
		assert.Empty(t, rec.Header().Get("X-XSS-Protection"))
	}
}

func TestCompressionHonorsAcceptEncoding(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ accept, encoding string }{
		{"gzip", "gzip"}, {"br, gzip;q=0.5", "gzip"},
		{"gzip;q=0, identity;q=1", ""}, {"gzip;q=0.000", ""},
		{"gzip;q=invalid", ""}, {"gzip;q=NaN", ""}, {"gzip;q=2", ""},
		{"notgzip", ""}, {"", ""},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			e.Use(compressionMiddleware())
			e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, strings.Repeat("a", 2048)) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", tc.accept)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, tc.encoding, rec.Header().Get("Content-Encoding"))
			assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")
		})
	}
}

func TestCompression(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path     string
		size     int
		encoding string
	}{
		{"/api/books", 2048, "gzip"}, {"/api/books", 20, ""},
		{"/assets/app-123.js", 2048, "gzip"}, {"/assets/cover-123.webp", 2048, ""},
		{"/api/events", 2048, ""}, {"/api/books/:id/cover", 2048, ""},
		{"/api/plugins/installed/:scope/:id/image", 2048, ""},
		{"/api/books/files/:id/page/:pageNum", 2048, ""},
		{"/api/books/files/:id/download/original", 2048, ""},
		{"/api/books/files/:id/stream", 2048, ""}, {"/api/jobs/:id/download", 2048, ""},
		{"/opds/download/:id/kepub", 2048, ""}, {"/opds/v1/books/:id/cover", 2048, ""},
		{"/ereader/key/:apiKey/file/:fileId", 2048, ""},
		{"/kobo/:apiKey/all/v1/books/:bookId/file/epub", 2048, ""},
		{"/kobo/:apiKey/all/v1/books/:imageId/thumbnail/:w/:h/*", 2048, ""},
	} {
		t.Run(tc.path+tc.encoding, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			e.Use(compressionMiddleware())
			e.GET(tc.path, func(c echo.Context) error { return c.String(http.StatusOK, strings.Repeat("a", tc.size)) })
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			e.ServeHTTP(rec, req)
			assert.Equal(t, tc.encoding, rec.Header().Get("Content-Encoding"))
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestForwardedHeadersTrustDirectPeer(t *testing.T) {
	t.Parallel()
	headers := []string{"X-Forwarded-Proto", "X-Forwarded-Host", "X-Forwarded-Port", "X-Forwarded-Prefix", "X-Forwarded-For"}
	for _, tc := range []struct {
		peer    string
		trusted bool
	}{
		{"8.8.8.8:1234", false}, {"[2001:4860:4860::8888]:1234", false},
		{"100.64.0.1:1234", false}, {"invalid", false},
		{"127.0.0.1:1234", true}, {"10.1.2.3:1234", true}, {"172.16.0.1:1234", true},
		{"192.168.1.2:1234", true}, {"169.254.1.2:1234", true}, {"[::1]:1234", true},
		{"[fe80::1%eth0]:1234", true}, {"[fd00::1]:1234", true}, {"[::ffff:192.168.1.2]:1234", true},
	} {
		t.Run(tc.peer, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			e.Pre(forwardedHeadersMiddleware)
			e.GET("/", func(c echo.Context) error {
				for _, name := range headers {
					expected := ""
					if tc.trusted {
						expected = "value"
					}
					assert.Equal(t, expected, c.Request().Header.Get(name), name)
				}
				return c.NoContent(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.peer
			for _, name := range headers {
				req.Header.Set(name, "value")
			}
			e.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
