package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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

	// Enable foreign keys to match production behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

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
	authService := NewService(db, "test-secret", 30*24*time.Hour)
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
	authService := NewService(db, "test-secret", 30*24*time.Hour)
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
	authService := NewService(db, "test-secret", 30*24*time.Hour)
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

func TestMiddlewareBasicAuth_CachesSuccessfulAuth(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret", 30*24*time.Hour)
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	role := new(models.Role)
	require.NoError(t, db.NewSelect().Model(role).Where("name = ?", models.RoleViewer).Scan(ctx))

	hashed, err := HashPassword("password1")
	require.NoError(t, err)
	user := &models.User{
		Username:     "cacheuser",
		PasswordHash: hashed,
		RoleID:       role.ID,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: nil}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)

	doRequest := func() bool {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.SetBasicAuth("cacheuser", "password1")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		called := false
		err := middleware.BasicAuth(func(_ echo.Context) error {
			called = true
			return nil
		})(c)
		require.NoError(t, err)
		return called
	}

	require.True(t, doRequest(), "first request should authenticate")

	// Break the password hash in the DB. If the cache is working, the next
	// request still succeeds because it never touches the DB or bcrypt.
	_, err = db.NewUpdate().
		Model((*models.User)(nil)).
		Set("password_hash = ?", "$2a$12$broken.hash.that.cannot.match.any.password.at.all.aaaaaaaaaaa").
		Where("id = ?", user.ID).
		Exec(ctx)
	require.NoError(t, err)

	assert.True(t, doRequest(), "second request should hit the cache and succeed despite invalidated DB hash")
}

func TestMiddlewareBasicAuth_CacheRespectsTTL(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret", 30*24*time.Hour)
	middleware := NewMiddleware(authService)
	middleware.basicAuthCache = newBasicAuthCache(100 * time.Millisecond)
	ctx := context.Background()

	role := new(models.Role)
	require.NoError(t, db.NewSelect().Model(role).Where("name = ?", models.RoleViewer).Scan(ctx))

	hashed, err := HashPassword("password1")
	require.NoError(t, err)
	user := &models.User{
		Username:     "ttluser",
		PasswordHash: hashed,
		RoleID:       role.ID,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: nil}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)

	doRequest := func() (called bool, status int) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.SetBasicAuth("ttluser", "password1")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := middleware.BasicAuth(func(_ echo.Context) error {
			called = true
			return nil
		})(c)
		require.NoError(t, err)
		return called, rec.Code
	}

	called, _ := doRequest()
	require.True(t, called)

	// Break the hash and wait for the cache entry to expire.
	_, err = db.NewUpdate().
		Model((*models.User)(nil)).
		Set("password_hash = ?", "$2a$12$broken.hash.that.cannot.match.any.password.at.all.aaaaaaaaaaa").
		Where("id = ?", user.ID).
		Exec(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	called, status := doRequest()
	assert.False(t, called, "after TTL expiry the cache must miss and the broken hash should reject the request")
	assert.Equal(t, http.StatusUnauthorized, status)
}

func TestMiddlewareBasicAuth_DoesNotCacheFailedAuth(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret", 30*24*time.Hour)
	middleware := NewMiddleware(authService)
	ctx := context.Background()

	role := new(models.Role)
	require.NoError(t, db.NewSelect().Model(role).Where("name = ?", models.RoleViewer).Scan(ctx))

	hashed, err := HashPassword("rightpassword")
	require.NoError(t, err)
	user := &models.User{
		Username:     "neguser",
		PasswordHash: hashed,
		RoleID:       role.ID,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	access := &models.UserLibraryAccess{UserID: user.ID, LibraryID: nil}
	_, err = db.NewInsert().Model(access).Exec(ctx)
	require.NoError(t, err)

	doRequest := func(password string) int {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.SetBasicAuth("neguser", password)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		require.NoError(t, middleware.BasicAuth(func(_ echo.Context) error { return nil })(c))
		return rec.Code
	}

	require.Equal(t, http.StatusUnauthorized, doRequest("wrong"))

	// The failed attempt above must not have inserted anything into the cache.
	middleware.basicAuthCache.mu.Lock()
	entries := len(middleware.basicAuthCache.entries)
	middleware.basicAuthCache.mu.Unlock()
	assert.Equal(t, 0, entries, "failed auth must not populate the cache")

	// And the wrong password must keep getting rejected even after the DB hash
	// is broken — i.e. the prior failure didn't poison the cache to "succeed"
	// against a now-invalidated DB.
	_, err = db.NewUpdate().
		Model((*models.User)(nil)).
		Set("password_hash = ?", "$2a$12$broken.hash.that.cannot.match.any.password.at.all.aaaaaaaaaaa").
		Where("id = ?", user.ID).
		Exec(ctx)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, doRequest("wrong"))
}

func TestMiddlewareBasicAuth_RejectsWhenMustChangePassword(t *testing.T) {
	t.Parallel()

	db := setupMiddlewareDB(t)
	authService := NewService(db, "test-secret", 30*24*time.Hour)
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
