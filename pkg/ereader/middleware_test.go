package ereader

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
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

func TestMiddleware_ApiKeyAuth(t *testing.T) {
	db := newTestDB(t)
	apiKeyService := apikeys.NewService(db)
	mw := NewMiddleware(apiKeyService)
	ctx := context.Background()

	// Create test user
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'testuser', 'hash', 1)`)
	require.NoError(t, err)

	// Create API key with permission
	apiKey, err := apiKeyService.Create(ctx, 1, "Test Key")
	require.NoError(t, err)
	apiKey, err = apiKeyService.AddPermission(ctx, 1, apiKey.ID, apikeys.PermissionEReaderBrowser)
	require.NoError(t, err)

	e := echo.New()

	t.Run("valid key with permission", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ereader/key/"+apiKey.Key+"/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ereader/key/:apiKey/*")
		c.SetParamNames("apiKey")
		c.SetParamValues(apiKey.Key)

		handler := mw.APIKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
			// Verify API key is in context
			ctxKey := GetAPIKeyFromContext(c.Request().Context())
			assert.NotNil(t, ctxKey)
			assert.Equal(t, apiKey.ID, ctxKey.ID)
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

		handler := mw.APIKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		assert.Error(t, err)
	})

	t.Run("key without required permission", func(t *testing.T) {
		// Create a fresh DB for this subtest to avoid race conditions
		subDB := newTestDB(t)
		subAPIKeyService := apikeys.NewService(subDB)
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

		handler := subMW.APIKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
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

		handler := mw.APIKeyAuth(apikeys.PermissionEReaderBrowser)(func(c echo.Context) error {
			return c.String(http.StatusOK, "success")
		})

		err := handler(c)
		assert.Error(t, err)
	})
}

func TestGetAPIKeyFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	apiKey := GetAPIKeyFromContext(ctx)
	assert.Nil(t, apiKey)
}
