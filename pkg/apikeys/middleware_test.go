package apikeys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_ApiKeyAuth(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	apiKeyService := NewService(db)
	mw := NewMiddleware(apiKeyService)
	ctx := context.Background()

	// Create test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create API key with permission
	apiKey, err := apiKeyService.Create(ctx, 1, "Test Key")
	require.NoError(t, err)
	apiKey, err = apiKeyService.AddPermission(ctx, 1, apiKey.ID, PermissionEReaderBrowser)
	require.NoError(t, err)

	e := echo.New()

	t.Run("valid key with permission", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+apiKey.Key+"/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues(apiKey.Key)

		handler := mw.APIKeyAuth(PermissionEReaderBrowser)(func(c echo.Context) error {
			// The key and its owner are in context
			ctxKey, err := RequireKey(c)
			require.NoError(t, err)
			assert.Equal(t, apiKey.ID, ctxKey.ID)
			owner, err := auth.RequireUser(c)
			require.NoError(t, err)
			assert.Equal(t, 1, owner.ID)
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("invalid key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key/invalid/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues("invalid")

		handler := mw.APIKeyAuth(PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		assert.Error(t, err)
	})

	t.Run("key without required permission", func(t *testing.T) {
		// Create a fresh DB for this subtest to avoid race conditions
		subDB := testdb.New(t)
		subAPIKeyService := NewService(subDB)
		subCtx := context.Background()

		// Create test user
		_, err := subDB.ExecContext(subCtx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
		require.NoError(t, err)

		// Create key without permission
		keyWithoutPerm, err := subAPIKeyService.Create(subCtx, 1, "No Perm Key")
		require.NoError(t, err)

		subMW := NewMiddleware(subAPIKeyService)
		subE := echo.New()

		req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+keyWithoutPerm.Key+"/", nil)
		rec := httptest.NewRecorder()
		c := subE.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues(keyWithoutPerm.Key)

		handler := subMW.APIKeyAuth(PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err = handler(c)
		assert.Error(t, err)
	})

	t.Run("missing key in path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key//", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues("")

		handler := mw.APIKeyAuth(PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		assert.Error(t, err)
	})
}

func TestRequireKey_NoKey_Returns401(t *testing.T) {
	t.Parallel()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	key, err := RequireKey(c)
	assert.Nil(t, key)
	var ec *errcodes.Error
	require.ErrorAs(t, err, &ec)
	assert.Equal(t, http.StatusUnauthorized, ec.HTTPCode)
}

// Each route family words the 403 for a key without its permission.
func TestMiddleware_APIKeyAuth_PermissionDeniedMessage(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)
	key, err := service.Create(ctx, 1, "No Perm Key")
	require.NoError(t, err)

	for permission, message := range map[string]string{
		PermissionKoboSync:       "This API key does not allow Kobo sync access.",
		PermissionEReaderBrowser: "This API key lacks the required permission.",
	} {
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		c.SetParamNames("apiKey")
		c.SetParamValues(key.Key)
		err := NewMiddleware(service).APIKeyAuth(permission)(func(echo.Context) error { return nil })(c)
		var ec *errcodes.Error
		require.ErrorAs(t, err, &ec)
		assert.Equal(t, http.StatusForbidden, ec.HTTPCode)
		assert.Equal(t, message, ec.Message)
	}
}
