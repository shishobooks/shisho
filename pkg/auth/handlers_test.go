package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func newTestContext(t *testing.T, payload, method, path string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rr := httptest.NewRecorder()
	return e.NewContext(req, rr), rr
}

func TestHandler_Setup_RejectsWhenUsersExist(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour)
	h := &handler{authService: svc}

	// First create a user using raw SQL to simulate existing user
	_, err := db.Exec(`INSERT INTO users (username, password_hash, role_id, is_active) VALUES (?, ?, 1, 1)`, "existingadmin", "hashedpassword")
	require.NoError(t, err)

	// Now try to setup again - should be rejected by handler-level guard
	payload := `{"username":"newadmin","password":"securepassword123"}`
	c, _ := newTestContext(t, payload, http.MethodPost, "/auth/setup")

	err = h.setup(c)

	// The handler should return an error
	require.Error(t, err)

	var errResp *errcodes.Error
	require.ErrorAs(t, err, &errResp)
	assert.Equal(t, http.StatusForbidden, errResp.HTTPCode)
	assert.Contains(t, errResp.Message, "Setup has already been completed")
}

func TestHandler_Logout_Returns204NoContent(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour)
	h := &handler{authService: svc}

	c, rr := newTestContext(t, "", http.MethodPost, "/auth/logout")

	err := h.logout(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Empty(t, rr.Body.String(), "logout response body must be empty")

	// The session cookie must still be cleared (MaxAge < 0).
	require.NotEmpty(t, rr.Result().Cookies())
	var sessionCookie *http.Cookie
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == CookieName {
			sessionCookie = ck
		}
	}
	require.NotNil(t, sessionCookie, "logout must set the session cookie")
	assert.Negative(t, sessionCookie.MaxAge, "logout must expire the session cookie")
}

func TestHandler_Login_ReturnsMustChangePassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	svc := NewService(db, "test-jwt-secret", 30*24*time.Hour)
	h := &handler{authService: svc}

	hashedPassword, err := HashPassword("securepassword123")
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO users (username, password_hash, role_id, is_active, must_change_password)
		VALUES (?, ?, 1, 1, 1)
	`, "resetme", hashedPassword)
	require.NoError(t, err)

	// Give access to all libraries (nil library_id)
	_, err = db.Exec(`INSERT INTO user_library_access (user_id, library_id) VALUES (1, NULL)`)
	require.NoError(t, err)

	payload := `{"username":"resetme","password":"securepassword123"}`
	c, rr := newTestContext(t, payload, http.MethodPost, "/auth/login")

	err = h.login(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp MeResponse
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.MustChangePassword)
	assert.NotEmpty(t, resp.Username)
}

// /api/auth/me reports a missing session, an invalid token, and a missing or
// deactivated user with the shared 401 constructors, and a failed user lookup
// as a server fault.
func TestHandler_Me_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup returns the cookie value to send ("" sends none).
		setup func(t *testing.T, db *bun.DB, svc *Service) string
		want  error
	}{
		{
			name:  "no session cookie",
			setup: func(*testing.T, *bun.DB, *Service) string { return "" },
			want:  errcodes.AuthenticationRequired(),
		},
		{
			name:  "invalid token",
			setup: func(*testing.T, *bun.DB, *Service) string { return "not-a-token" },
			want:  errcodes.InvalidSession(),
		},
		{
			name: "deactivated user",
			setup: func(t *testing.T, db *bun.DB, svc *Service) string {
				user := createUserWithPasswordResetRequired(context.Background(), t, db)
				_, err := db.NewUpdate().Model(user).Set("is_active = ?", false).WherePK().Exec(context.Background())
				require.NoError(t, err)
				token, err := svc.GenerateToken(user)
				require.NoError(t, err)
				return token
			},
			want: errcodes.UserInactive(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			svc := NewService(db, "test-jwt-secret", time.Hour)
			h := &handler{authService: svc}
			cookie := tt.setup(t, db, svc)

			c, _ := newTestContext(t, "", http.MethodGet, "/auth/me")
			if cookie != "" {
				c.Request().AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
			}
			assert.Equal(t, tt.want, h.me(c))
		})
	}

	t.Run("user lookup fault", func(t *testing.T) {
		t.Parallel()
		db := testdb.New(t)
		svc := NewService(db, "test-jwt-secret", time.Hour)
		h := &handler{authService: svc}
		user := createUserWithPasswordResetRequired(context.Background(), t, db)
		token, err := svc.GenerateToken(user)
		require.NoError(t, err)
		// A created_at that cannot scan into a time fails the lookup with an
		// error other than sql.ErrNoRows.
		_, err = db.Exec("UPDATE users SET created_at = 'not a time' WHERE id = ?", user.ID)
		require.NoError(t, err)

		c, _ := newTestContext(t, "", http.MethodGet, "/auth/me")
		c.Request().AddCookie(&http.Cookie{Name: CookieName, Value: token})
		err = h.me(c)
		require.Error(t, err)
		var codeErr *errcodes.Error
		assert.NotErrorAs(t, err, &codeErr, "want a server fault, got %v", err)
	})
}
