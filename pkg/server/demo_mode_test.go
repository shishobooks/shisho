package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
)

func TestDemoModeMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		pattern string
		target  string
		allowed bool
	}{
		{"browse", http.MethodGet, "/books", "/books?limit=10", true},
		{"head", http.MethodHead, "/books", "/books", true},
		{"options", http.MethodOptions, "/books", "/books", true},
		{"login", http.MethodPost, "/auth/login", "/auth/login", true},
		{"logout", http.MethodPost, "/auth/logout", "/auth/logout", true},
		{"generated reader download", http.MethodGet, "/books/files/:id/download", "/books/files/42/download", true},
		{"audio stream", http.MethodGet, "/books/files/:id/stream", "/books/files/42/stream", true},
		{"original download", http.MethodGet, "/books/files/:id/download/original", "/books/files/42/download/original?x=1", false},
		{"kepub download", http.MethodGet, "/books/files/:id/download/kepub", "/books/files/42/download/kepub", false},
		{"bulk download", http.MethodGet, "/jobs/:id/download", "/jobs/42/download", false},
		{"put", http.MethodPut, "/books/:id", "/books/42", false},
		{"delete", http.MethodDelete, "/books/:id", "/books/42", false},
		{"patch", http.MethodPatch, "/books/:id", "/books/42", false},
		{"post", http.MethodPost, "/books/:id", "/books/42", false},
		{"setup", http.MethodPost, "/auth/setup", "/auth/setup", false},
		{"login prefix is not allowed", http.MethodPost, "/auth/login/:id", "/auth/login/42", false},
		{"put login is not allowed", http.MethodPut, "/auth/login", "/auth/login", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			e.HTTPErrorHandler = errcodes.NewHandler().Handle
			e.Use(demoModeMiddleware)
			called := false
			e.Group("/api").Add(tt.method, tt.pattern, func(c echo.Context) error {
				called = true
				return c.NoContent(http.StatusNoContent)
			})

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(tt.method, "/api"+tt.target, nil))

			assert.Equal(t, tt.allowed, called, "rejected requests must not reach the handler")
			if tt.allowed {
				assert.Equal(t, http.StatusNoContent, rec.Code)
			} else {
				assert.Equal(t, http.StatusForbidden, rec.Code)
				assert.JSONEq(t, `{"error":{"code":"demo_mode","message":"This action is unavailable in the demo.","status_code":403}}`, rec.Body.String())
			}
		})
	}
}
