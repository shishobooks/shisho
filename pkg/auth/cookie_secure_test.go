package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The session cookie is Secure exactly when device URLs are built as https,
// so a chained or uppercase X-Forwarded-Proto agrees with both.
func TestSessionCookie_SecureFollowsPublicScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		proto  string
		secure bool
	}{
		{name: "no forwarded proto", proto: "", secure: false},
		{name: "https", proto: "https", secure: true},
		{name: "http", proto: "http", secure: false},
		{name: "chained, client-facing https first", proto: "https, http", secure: true},
		{name: "chained, client-facing http first", proto: "http, https", secure: false},
		{name: "uppercase", proto: "HTTPS", secure: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			h := &handler{authService: NewService(db, "test-jwt-secret", 30*24*time.Hour)}

			hashedPassword, err := HashPassword("securepassword123")
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, 1, 1)`, "reader", hashedPassword)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (1, NULL)`)
			require.NoError(t, err)

			c, rr := newTestContext(t, `{"username":"reader","password":"securepassword123"}`, http.MethodPost, "/auth/login")
			if tt.proto != "" {
				c.Request().Header.Set("X-Forwarded-Proto", tt.proto)
			}
			require.NoError(t, h.login(c))
			assert.Equal(t, tt.secure, sessionCookie(t, rr).Secure, "login cookie")

			c, rr = newTestContext(t, "", http.MethodPost, "/auth/logout")
			if tt.proto != "" {
				c.Request().Header.Set("X-Forwarded-Proto", tt.proto)
			}
			require.NoError(t, h.logout(c))
			assert.Equal(t, tt.secure, sessionCookie(t, rr).Secure, "logout cookie")
		})
	}
}

func sessionCookie(t *testing.T, rr *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == CookieName {
			return ck
		}
	}
	require.FailNow(t, "session cookie not set")
	return nil
}
