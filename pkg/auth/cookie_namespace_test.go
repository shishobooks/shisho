package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCookieName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "shisho_session", SessionCookieName(""))
	assert.Equal(t, "shisho_session_feature_1a2b3c4d", SessionCookieName("feature_1a2b3c4d"))
}

func TestHandler_LoginAndLogout_UseTheNamespacedCookie(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour, WithCookieNamespace("feature"))
	h := &handler{authService: svc}

	hashedPassword, err := HashPassword("securepassword123")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, 1, 1)`, "reader", hashedPassword)
	require.NoError(t, err)

	c, rr := newTestContext(t, `{"username":"reader","password":"securepassword123"}`, http.MethodPost, "/auth/login")
	require.NoError(t, h.login(c))
	require.Len(t, rr.Result().Cookies(), 1)
	assert.Equal(t, "shisho_session_feature", rr.Result().Cookies()[0].Name)

	c, rr = newTestContext(t, "", http.MethodPost, "/auth/logout")
	require.NoError(t, h.logout(c))
	require.Len(t, rr.Result().Cookies(), 1)
	assert.Equal(t, "shisho_session_feature", rr.Result().Cookies()[0].Name)
}

func TestMiddlewareAuthenticate_ReadsOnlyTheNamespacedCookie(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour, WithCookieNamespace("feature"))
	middleware := NewMiddleware(svc)

	_, err := db.Exec(`INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, 1, 1)`, "reader", "hash")
	require.NoError(t, err)
	user, err := svc.GetUserByID(context.Background(), 1)
	require.NoError(t, err)
	token, err := svc.GenerateToken(user)
	require.NoError(t, err)

	authenticate := func(cookieName string) error {
		req := httptest.NewRequest(http.MethodGet, "/books", nil)
		req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		c := echo.New().NewContext(req, httptest.NewRecorder())
		return middleware.Authenticate(func(echo.Context) error { return nil })(c)
	}

	// Another worktree on the same host sets the default name.
	require.Error(t, authenticate(CookieName))
	require.NoError(t, authenticate("shisho_session_feature"))
}
