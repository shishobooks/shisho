package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Create the roles table
	_, err = db.Exec(`
		CREATE TABLE roles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			name TEXT NOT NULL UNIQUE,
			is_system BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	require.NoError(t, err)

	// Create the permissions table
	_, err = db.Exec(`
		CREATE TABLE permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role_id INTEGER REFERENCES roles (id) ON DELETE CASCADE NOT NULL,
			resource TEXT NOT NULL,
			operation TEXT NOT NULL,
			UNIQUE (role_id, resource, operation)
		)
	`)
	require.NoError(t, err)

	// Create the users table
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			username TEXT NOT NULL UNIQUE COLLATE NOCASE,
			email TEXT COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role_id INTEGER REFERENCES roles (id) NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			must_change_password BOOLEAN NOT NULL DEFAULT FALSE
		)
	`)
	require.NoError(t, err)

	// Create user_library_access table
	_, err = db.Exec(`
		CREATE TABLE user_library_access (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
			library_id INTEGER REFERENCES libraries (id) ON DELETE CASCADE
		)
	`)
	require.NoError(t, err)

	// Insert admin role
	_, err = db.Exec(`INSERT INTO roles (name, is_system) VALUES (?, ?)`, models.RoleAdmin, true)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

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
	db := setupTestDB(t)
	svc := NewService(db, "test-jwt-secret")
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

func TestHandler_Login_ReturnsMustChangePassword(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db, "test-jwt-secret")
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
