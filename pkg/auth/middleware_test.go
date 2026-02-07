package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupMiddlewareDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createUserWithPasswordResetRequired(ctx context.Context, t *testing.T, db *bun.DB) *models.User {
	t.Helper()

	role := new(models.Role)
	err := db.NewSelect().
		Model(role).
		Where("name = ?", models.RoleViewer).
		Scan(ctx)
	require.NoError(t, err)

	user := &models.User{
		Username:           "testuser",
		PasswordHash:       "hash",
		RoleID:             role.ID,
		IsActive:           true,
		MustChangePassword: true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: nil,
	}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)

	return user
}

func TestMiddlewareAuthenticate_BlocksWhenPasswordResetIsRequired(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret")
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	user := createUserWithPasswordResetRequired(ctx, t, db)
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/books", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/books")

	nextCalled := false
	err = middleware.Authenticate(func(_ echo.Context) error {
		nextCalled = true
		return nil
	})(c)
	require.Error(t, err)
	assert.False(t, nextCalled)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "password_reset_required", codeErr.Code)
}

func TestMiddlewareAuthenticate_AllowsSelfPasswordResetWhenRequired(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret")
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	user := createUserWithPasswordResetRequired(ctx, t, db)
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/users/"+strconv.Itoa(user.ID)+"/reset-password", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(user.ID))

	nextCalled := false
	err = middleware.Authenticate(func(_ echo.Context) error {
		nextCalled = true
		return nil
	})(c)
	require.NoError(t, err)
	assert.True(t, nextCalled)
}

func TestMiddlewareAuthenticate_BlocksCrossUserPasswordReset(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret")
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	user := createUserWithPasswordResetRequired(ctx, t, db)
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)

	// User with must_change_password tries to reset a DIFFERENT user's password (id=9999)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/users/9999/reset-password", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/users/:id/reset-password")
	c.SetParamNames("id")
	c.SetParamValues("9999")

	nextCalled := false
	err = middleware.Authenticate(func(_ echo.Context) error {
		nextCalled = true
		return nil
	})(c)
	require.Error(t, err)
	assert.False(t, nextCalled)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, "password_reset_required", codeErr.Code)
}

func TestMiddlewareBasicAuth_RejectsWhenMustChangePassword(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret")
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	// Create a user with a real password hash so BasicAuth can authenticate them
	role := new(models.Role)
	err := db.NewSelect().
		Model(role).
		Where("name = ?", models.RoleViewer).
		Scan(ctx)
	require.NoError(t, err)

	hashedPassword, err := HashPassword("testpassword")
	require.NoError(t, err)

	user := &models.User{
		Username:           "basicauthuser",
		PasswordHash:       hashedPassword,
		RoleID:             role.ID,
		IsActive:           true,
		MustChangePassword: true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: nil,
	}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
	req.SetBasicAuth("basicauthuser", "testpassword")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	nextCalled := false
	err = middleware.BasicAuth(func(_ echo.Context) error {
		nextCalled = true
		return nil
	})(c)
	// BasicAuth returns nil error but writes 401 directly to the response
	require.NoError(t, err)
	assert.False(t, nextCalled)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Basic")
}
