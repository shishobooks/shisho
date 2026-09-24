package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerDoesNotGrantCORS(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()
	srv, err := New(cfg, tc.db, tc.worker, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		req := httptest.NewRequest(method, "/api/auth/status", nil)
		req.Header.Set("Origin", "https://other.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		for header := range rec.Header() {
			assert.False(t, strings.HasPrefix(header, "Access-Control-"), header)
		}
	}
}

func TestProxyTrustForLoginAndOPDS(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()
	svc := auth.NewService(tc.db, cfg.JWTSecret, cfg.SessionDuration())
	_, err := svc.CreateFirstAdmin(t.Context(), "admin", nil, "test-password-123")
	require.NoError(t, err)
	srv, err := New(cfg, tc.db, tc.worker, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	for _, test := range []struct {
		peer, base string
		secure     bool
	}{
		{"192.168.1.20:1234", "https://books.example/prefix/opds/v1", true},
		{"8.8.8.8:1234", "http://books.example/opds/v1", false},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://books.example/api/auth/login", strings.NewReader(`{"username":"admin","password":"test-password-123"}`))
		req.RemoteAddr = test.peer
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.NotEmpty(t, rec.Result().Cookies())
		assert.Equal(t, test.secure, rec.Result().Cookies()[0].Secure)

		req = httptest.NewRequest(http.MethodGet, "http://books.example/opds/v1/epub/catalog", nil)
		req.RemoteAddr = test.peer
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Prefix", "/prefix")
		req.SetBasicAuth("admin", "test-password-123")
		rec = httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), `href="`+test.base+`/epub/catalog"`)
	}
}

func TestEventStreamFlushesWithoutGzip(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	cfg := config.NewForTest()
	cfg.CacheDir = t.TempDir()
	svc := auth.NewService(tc.db, cfg.JWTSecret, cfg.SessionDuration())
	user, err := svc.CreateFirstAdmin(t.Context(), "admin", nil, "test-password-123")
	require.NoError(t, err)
	token, err := svc.GenerateToken(user)
	require.NoError(t, err)
	broker := events.NewBroker()
	srv, err := New(cfg, tc.db, tc.worker, nil, broker, nil, nil, nil, nil)
	require.NoError(t, err)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	defer broker.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	res, err := ts.Client().Do(req)
	require.NoError(t, err, "SSE headers must flush before an event is published")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Empty(t, res.Header.Get("Content-Encoding"))
	require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	broker.Publish(events.Event{Type: "job.created", Data: `{"job_id":42}`})
	reader := bufio.NewReader(res.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "event: job.created\n", line)
	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "data: {\"job_id\":42}\n", line)
}
