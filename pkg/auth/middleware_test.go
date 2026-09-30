package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

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

	db := testdb.New(t)
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

	db := testdb.New(t)
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

	db := testdb.New(t)
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

	db := testdb.New(t)
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

	db := testdb.New(t)
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
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.SetBasicAuth("ttluser", "password1")
		called, rec := serveBasicAuth(middleware, req)
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

	db := testdb.New(t)
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
		req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
		req.SetBasicAuth("neguser", password)
		_, rec := serveBasicAuth(middleware, req)
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

	db := testdb.New(t)
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

	req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
	req.SetBasicAuth("basicauthuser", "testpassword")
	nextCalled, rec := serveBasicAuth(middleware, req)
	assert.False(t, nextCalled)
	assertBasicAuthChallenge(t, rec)
}

// serveBasicAuth runs BasicAuth for req and renders any error it returns
// through the errcodes handler, as the server does. It reports whether the
// next handler ran.
func serveBasicAuth(m *Middleware, req *http.Request) (bool, *httptest.ResponseRecorder) {
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	called := false
	if err := m.BasicAuth(func(_ echo.Context) error {
		called = true
		return nil
	})(c); err != nil {
		e.HTTPErrorHandler(err, c)
	}
	return called, rec
}

// assertBasicAuthChallenge requires a 401 that carries the Basic challenge
// and the errcodes JSON body every other API error uses.
func assertBasicAuthChallenge(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusUnauthorized, rec.Code, "response body: %s", rec.Body.String())
	assert.Equal(t, `Basic realm="Shisho OPDS"`, rec.Header().Get("WWW-Authenticate"))
	var body struct {
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			StatusCode int    `json:"status_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "response body: %s", rec.Body.String())
	assert.Equal(t, "unauthorized", body.Error.Code)
	assert.Equal(t, "Authentication required", body.Error.Message)
	assert.Equal(t, http.StatusUnauthorized, body.Error.StatusCode)
}

// Every Basic Auth rejection, whether the header is missing, malformed, or
// names wrong credentials, is the same challenge in the errcodes shape.
func TestMiddlewareBasicAuth_RejectionsUseErrcodesChallenge(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	middleware := NewMiddleware(NewService(db, "test-secret", 30*24*time.Hour))

	tests := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"not Basic", "Bearer abc"},
		{"not base64", "Basic !!!"},
		{"no colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon"))},
		{"unknown user", "Basic " + base64.StdEncoding.EncodeToString([]byte("nobody:secret"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			called, rec := serveBasicAuth(middleware, req)
			assert.False(t, called)
			assertBasicAuthChallenge(t, rec)
		})
	}
}

// A database fault while loading the user is a server error, not a
// credentials challenge that would prompt the reader for a password again.
func TestMiddlewareBasicAuth_LoadFaultIsServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	middleware := NewMiddleware(NewService(db, "test-secret", 30*24*time.Hour))
	_, err := db.Exec("DROP TABLE roles")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/opds/catalog", nil)
	req.SetBasicAuth("anyone", "secret")
	called, rec := serveBasicAuth(middleware, req)
	assert.False(t, called)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "response body: %s", rec.Body.String())
	assert.Empty(t, rec.Header().Get("WWW-Authenticate"))
}

// A session whose user lookup fails for any reason but a missing or
// deactivated user is a server error, not a 401 that signs the user out.
func TestMiddlewareAuthenticate_LoadFaultIsServerError(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	authService := NewService(db, "test-secret", 30*24*time.Hour)
	user := createUserWithPasswordResetRequired(context.Background(), t, db)
	token, err := authService.GenerateToken(user)
	require.NoError(t, err)
	// A created_at that cannot scan into a time fails the lookup with an
	// error other than sql.ErrNoRows.
	_, err = db.Exec("UPDATE users SET created_at = 'not a time' WHERE id = ?", user.ID)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/books", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	c := e.NewContext(req, httptest.NewRecorder())
	err = NewMiddleware(authService).Authenticate(func(echo.Context) error { return nil })(c)
	require.Error(t, err)
	var codeErr *errcodes.Error
	assert.NotErrorAs(t, err, &codeErr, "want a server fault, got %v", err)
}

// A session token that does not validate is the shared invalid-session 401.
func TestMiddlewareAuthenticate_InvalidTokenIsInvalidSession(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/books", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "not-a-token"})
	c := e.NewContext(req, httptest.NewRecorder())
	err := NewMiddleware(NewService(db, "test-secret", time.Hour)).Authenticate(func(echo.Context) error { return nil })(c)
	assert.Equal(t, errcodes.InvalidSession(), err)
}

func TestRequirePermission_Message(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	SetUser(c, &models.User{
		Role: &models.Role{Name: models.RoleViewer},
	})

	m := &Middleware{}
	handler := m.RequirePermission(models.ResourceJobs, models.OperationWrite)(
		func(echo.Context) error { return nil },
	)

	err := handler(c)
	require.Error(t, err)

	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
	assert.Equal(t, "You don't have permission to write jobs", codeErr.Message)
}

func TestRequireAnyPermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		permissions []*models.Permission
		allowed     bool
	}{
		{"first permission", []*models.Permission{{Resource: models.ResourceShares, Operation: models.OperationRead}}, true},
		{"second permission, same resource", []*models.Permission{{Resource: models.ResourceShares, Operation: models.OperationWrite}}, true},
		{"third permission", []*models.Permission{{Resource: models.ResourceConfig, Operation: models.OperationRead}}, true},
		{"unlisted operation", []*models.Permission{{Resource: models.ResourceConfig, Operation: models.OperationWrite}}, false},
		{"neither", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/settings/sharing", nil), httptest.NewRecorder())
			SetUser(c, &models.User{Role: &models.Role{Permissions: tt.permissions}})

			m := &Middleware{}
			called := false
			err := m.RequireAnyPermission(
				Permission{Resource: models.ResourceShares, Operation: models.OperationRead},
				Permission{Resource: models.ResourceShares, Operation: models.OperationWrite},
				Permission{Resource: models.ResourceConfig, Operation: models.OperationRead},
			)(
				func(echo.Context) error { called = true; return nil },
			)(c)

			assert.Equal(t, tt.allowed, called)
			if tt.allowed {
				require.NoError(t, err)
				return
			}
			var codeErr *errcodes.Error
			require.ErrorAs(t, err, &codeErr)
			assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
			assert.Equal(t, "You don't have permission to read shares, write shares, or read config", codeErr.Message)
		})
	}
}
