package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RoutingBoundary(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cfg := config.NewForTest()
	cfg.Environment = ""
	cfg.CacheDir = t.TempDir()
	originalNotFound := reflect.ValueOf(echo.NotFoundHandler).Pointer()
	srv, err := New(cfg, tc.db, tc.worker, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, originalNotFound, reflect.ValueOf(echo.NotFoundHandler).Pointer(), "constructing a server must not mutate Echo's global fallback")

	for _, path := range []string{"/", "/books/42", "/settings", "/apiary", "/opds-other"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
			assert.Equal(t, http.StatusOK, rec.Code, "%s %s", method, path)
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "text/html")
			assert.Equal(t, "no-cache", rec.Header().Get(echo.HeaderCacheControl))
			if method == http.MethodHead {
				assert.Empty(t, rec.Body.String())
			}
		}
	}
	for _, prefix := range []string{"/api", "/opds", "/kobo", "/ereader", "/e"} {
		for _, path := range []string{prefix, prefix + "/missing/nested/path"} {
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusNotFound, rec.Code, path)
			assert.Contains(t, rec.Header().Get(echo.HeaderContentType), "application/json")
			if strings.HasPrefix(path, "/e/") {
				// Echo's terminal parameter matches the remainder of the path.
				assert.Contains(t, rec.Body.String(), `"code":"short_url_not_found_or_expired"`)
			} else {
				assert.Contains(t, rec.Body.String(), `"code":"not_found"`)
			}
		}
	}
	for _, path := range []string{"/api/books/42/missing", "/opds/v1/epub/missing", "/ereader/key/missing/unknown", "/kobo/missing/all/unknown"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.Contains(t, rec.Body.String(), `"code":"not_found"`)
	}
	for _, path := range []string{"/api/books", "/api/libraries", "/api/settings/user"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNew_ServerAddress(t *testing.T) {
	t.Parallel()
	cfg := config.NewForTest()
	cfg.Environment = ""
	cfg.ServerHost = "::1"
	cfg.ServerPort = 3689
	srv, err := New(cfg, nil, &worker.Worker{}, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "[::1]:3689", srv.Addr)
}

func TestNew_DemoModeSession(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()
	svc := auth.NewService(tc.db, cfg.JWTSecret, cfg.SessionDuration())
	_, err := svc.CreateFirstAdmin(t.Context(), "admin", nil, "test-password-123")
	require.NoError(t, err)

	for _, demo := range []bool{false, true} {
		cfg.DemoMode = demo
		srv, err := New(cfg, tc.db, tc.worker, nil, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		expected := `{"needs_setup":false,"demo_mode":false}`
		if demo {
			expected = `{"needs_setup":false,"demo_mode":true}`
		}
		assert.JSONEq(t, expected, rec.Body.String())

		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"test-password-123"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		cookies := rec.Result().Cookies()
		require.NotEmpty(t, cookies)

		if demo {
			for _, path := range []string{"/api/users/1/reset-password", "/api/jobs", "/api/books/42", "/api/settings", "/api/auth/setup"} {
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				req.AddCookie(cookies[0])
				srv.Handler.ServeHTTP(rec, req)
				assert.Equal(t, http.StatusForbidden, rec.Code, path)
				assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)
			}
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		req.AddCookie(cookies[0])
		srv.Handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		require.NotEmpty(t, rec.Result().Cookies())
		assert.Negative(t, rec.Result().Cookies()[0].MaxAge)
	}
}

func TestNew_DemoModeRoutes(t *testing.T) {
	// Test-mode route registration mutates the shared plugin download hosts.
	originalHosts := plugins.AllowedDownloadHosts
	t.Cleanup(func() { plugins.AllowedDownloadHosts = originalHosts })

	for _, demo := range []bool{false, true} {
		name := "normal"
		if demo {
			name = "demo"
		}
		t.Run(name, func(t *testing.T) {
			cfg := config.NewForTest()
			cfg.DemoMode = demo
			cfg.Environment = "test"
			cfg.CacheDir = t.TempDir()
			srv, err := New(cfg, nil, &worker.Worker{}, nil, nil, nil, nil, nil, nil)
			require.NoError(t, err)
			e := srv.Handler.(*echo.Echo)
			routes := make(map[string]bool)
			for _, route := range e.Routes() {
				routes[route.Method+" "+route.Path] = true
				if demo {
					for _, prefix := range []string{"/opds", "/ereader", "/e/", "/kobo", "/api/plugins", "/api/test"} {
						assert.False(t, strings.HasPrefix(route.Path, prefix), "unexpected route: %s %s", route.Method, route.Path)
					}
					assert.NotContains(t, route.Path, "/plugins/")
				}
			}

			for _, route := range []string{
				"GET /opds/v1/:types/catalog",
				"GET /ereader/key/:apiKey/",
				"GET /e/:shortCode",
				"GET /kobo/:apiKey/all/v1/library/sync",
				"GET /api/plugins/installed",
				"GET /api/plugins/identifier-types",
				"GET /api/plugins/order/:hookType",
				"POST /api/plugins/search",
				"POST /api/plugins/apply",
				"GET /api/libraries/:id/plugins/order/:hookType",
				"GET /api/test/plugins/fixture-info",
			} {
				assert.Equal(t, !demo, routes[route], route)
			}
			for _, route := range []string{
				"GET /api/auth/status", "POST /api/auth/login", "POST /api/auth/logout",
				"GET /api/books", "GET /api/books/files/:id/download",
				"GET /api/books/files/:id/page/:pageNum", "GET /api/books/files/:id/stream",
			} {
				assert.True(t, routes[route], route)
			}

			if demo {
				for _, path := range []string{
					"/opds", "/opds/v1/epub/catalog", "/ereader", "/ereader/key/example/", "/e", "/e/example",
					"/kobo", "/kobo/example/all/v1/library/sync", "/api/plugins/installed",
					"/api/test/plugins/fixture-info",
				} {
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
					assert.Equal(t, http.StatusNotFound, rec.Code, path)
					assert.Contains(t, rec.Body.String(), `"code":"not_found"`)
				}
			}

			// Exercise the registered boundary, not just the standalone middleware.
			for _, request := range []struct{ method, path string }{
				{http.MethodPost, "/api/books/42"},
				{http.MethodPut, "/api/books/files/42/chapters"},
				{http.MethodDelete, "/api/books/42"},
				{http.MethodGet, "/api/books/files/42/download/original"},
				{http.MethodGet, "/api/books/files/42/download/kepub"},
				{http.MethodGet, "/api/jobs/42/download"},
			} {
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, httptest.NewRequest(request.method, request.path, nil))
				if demo {
					assert.Equal(t, http.StatusForbidden, rec.Code, request.path)
					assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)
				} else {
					assert.Equal(t, http.StatusUnauthorized, rec.Code, "normal mode must still reach authentication")
				}
			}
		})
	}
}
