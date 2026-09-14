package server

import (
	"net/http"
	"net/http/httptest"
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

func TestNew_DemoModeSession(t *testing.T) {
	// New changes Echo's global NotFoundHandler.
	originalNotFound := echo.NotFoundHandler
	t.Cleanup(func() { echo.NotFoundHandler = originalNotFound })

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
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/status", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		expected := `{"needs_setup":false,"demo_mode":false}`
		if demo {
			expected = `{"needs_setup":false,"demo_mode":true}`
		}
		assert.JSONEq(t, expected, rec.Body.String())

		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"admin","password":"test-password-123"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		cookies := rec.Result().Cookies()
		require.NotEmpty(t, cookies)

		if demo {
			for _, path := range []string{"/users/1/reset-password", "/jobs", "/books/42", "/settings", "/auth/setup"} {
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				req.AddCookie(cookies[0])
				srv.Handler.ServeHTTP(rec, req)
				assert.Equal(t, http.StatusForbidden, rec.Code, path)
				assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)
			}
		}

		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		req.AddCookie(cookies[0])
		srv.Handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		require.NotEmpty(t, rec.Result().Cookies())
		assert.Negative(t, rec.Result().Cookies()[0].MaxAge)
	}
}

func TestNew_DemoModeRoutes(t *testing.T) {
	// New changes Echo's global NotFoundHandler and test-mode download hosts.
	originalNotFound := echo.NotFoundHandler
	originalHosts := plugins.AllowedDownloadHosts
	t.Cleanup(func() {
		echo.NotFoundHandler = originalNotFound
		plugins.AllowedDownloadHosts = originalHosts
	})

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
					for _, prefix := range []string{"/opds", "/ereader", "/e/", "/kobo", "/plugins", "/test"} {
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
				"GET /plugins/installed",
				"POST /plugins/search",
				"POST /plugins/apply",
				"GET /libraries/:id/plugins/order/:hookType",
				"GET /test/plugins/fixture-info",
			} {
				assert.Equal(t, !demo, routes[route], route)
			}
			for _, route := range []string{
				"GET /auth/status", "POST /auth/login", "POST /auth/logout",
				"GET /books", "GET /books/files/:id/download",
				"GET /books/files/:id/page/:pageNum", "GET /books/files/:id/stream",
			} {
				assert.True(t, routes[route], route)
			}

			if demo {
				for _, path := range []string{
					"/opds/v1/epub/catalog", "/ereader/key/example/", "/e/example",
					"/kobo/example/all/v1/library/sync", "/plugins/installed",
					"/test/plugins/fixture-info",
				} {
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
					assert.Equal(t, http.StatusNotFound, rec.Code, path)
				}
			}

			// Exercise the registered boundary, not just the standalone middleware.
			for _, request := range []struct{ method, path string }{
				{http.MethodPost, "/books/42"},
				{http.MethodPut, "/books/files/42/chapters"},
				{http.MethodDelete, "/books/42"},
				{http.MethodGet, "/books/files/42/download/original"},
				{http.MethodGet, "/books/files/42/download/kepub"},
				{http.MethodGet, "/jobs/42/download"},
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
