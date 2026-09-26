package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginRoutes_Permissions exercises the registered /api/plugins boundary
// for each built-in role and a custom role without books:read. The read-only
// lookups that book pages and the identify dialog call must be reachable with
// Books Read, while every management route stays on Config Write.
func TestPluginRoutes_Permissions(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	// Every pooled connection to an in-memory SQLite database opens a
	// separate, empty database, so keep the pool at one connection. Subtests
	// run sequentially because they share this database and the final admin
	// PUT below rewrites the hook order they read.
	tc.db.SetMaxOpenConns(1)
	ctx := t.Context()
	cfg := config.NewForTest()
	cfg.Environment = ""
	cfg.CacheDir = t.TempDir()
	cfg.PluginDir = t.TempDir()
	svc := auth.NewService(tc.db, cfg.JWTSecret, cfg.SessionDuration())
	generateToken := func(user *models.User) string {
		token, err := svc.GenerateToken(user)
		require.NoError(t, err)
		return token
	}

	// A custom role with library access but no books:read proves the lookups
	// check books:read rather than authentication alone.
	const noBooksRole = "no-books"
	now := time.Now()
	customRole := &models.Role{CreatedAt: now, UpdatedAt: now, Name: noBooksRole}
	_, err := tc.db.NewInsert().Model(customRole).Exec(ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.Permission{
		RoleID:    customRole.ID,
		Resource:  models.ResourceLibraries,
		Operation: models.OperationRead,
	}).Exec(ctx)
	require.NoError(t, err)

	admin, err := svc.CreateFirstAdmin(ctx, "admin", nil, "test-password-123")
	require.NoError(t, err)
	tokens := map[string]string{models.RoleAdmin: generateToken(admin)}
	for _, roleName := range []string{models.RoleEditor, models.RoleViewer, noBooksRole} {
		role := &models.Role{}
		require.NoError(t, tc.db.NewSelect().Model(role).Where("name = ?", roleName).Scan(ctx))
		user := &models.User{
			CreatedAt:    now,
			UpdatedAt:    now,
			Username:     roleName,
			PasswordHash: "unused",
			RoleID:       role.ID,
			IsActive:     true,
		}
		_, err := tc.db.NewInsert().Model(user).Exec(ctx)
		require.NoError(t, err)
		loaded, err := svc.GetUserByID(ctx, user.ID)
		require.NoError(t, err)
		tokens[roleName] = generateToken(loaded)
	}

	// Seed one row behind each lookup so the responses carry real data.
	_, err = tc.db.NewInsert().Model(&models.Plugin{
		Scope:       "community",
		ID:          "example",
		Name:        "Example",
		Version:     "1.0.0",
		InstalledAt: now,
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.PluginIdentifierType{
		ID:       "example_id",
		Scope:    "community",
		PluginID: "example",
		Name:     "Example ID",
	}).Exec(ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.PluginHookConfig{
		HookType: models.PluginHookMetadataEnricher,
		Scope:    "community",
		PluginID: "example",
		Position: 0,
		Mode:     models.PluginModeEnabled,
	}).Exec(ctx)
	require.NoError(t, err)

	srv, err := New(cfg, tc.db, tc.worker, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	do := func(method, path, body, roleName string) *httptest.ResponseRecorder {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, path, nil)
		} else {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}
		if roleName != "" {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tokens[roleName]})
		}
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		return rec
	}

	lookups := []struct{ path, want string }{
		{
			"/api/plugins/identifier-types",
			`[{"id":"example_id","scope":"community","plugin_id":"example","name":"Example ID","url_template":null,"pattern":null}]`,
		},
		{
			"/api/plugins/order/" + models.PluginHookMetadataEnricher,
			`[{"hook_type":"metadataEnricher","scope":"community","plugin_id":"example","position":0,"mode":"enabled"}]`,
		},
	}
	for _, lookup := range lookups {
		t.Run("GET "+lookup.path, func(t *testing.T) {
			for _, roleName := range []string{models.RoleAdmin, models.RoleEditor, models.RoleViewer} {
				rec := do(http.MethodGet, lookup.path, "", roleName)
				assert.Equal(t, http.StatusOK, rec.Code, "%s: %s", roleName, rec.Body.String())
				assert.JSONEq(t, lookup.want, rec.Body.String(), roleName)
			}
			assert.Equal(t, http.StatusForbidden, do(http.MethodGet, lookup.path, "", noBooksRole).Code)
			assert.Equal(t, http.StatusUnauthorized, do(http.MethodGet, lookup.path, "", "").Code)
		})
	}

	management := []struct{ method, path, body string }{
		{http.MethodGet, "/api/plugins/installed", ""},
		{http.MethodPost, "/api/plugins/scan", ""},
		{http.MethodPut, "/api/plugins/order/" + models.PluginHookMetadataEnricher, `{"order":[]}`},
		{http.MethodDelete, "/api/plugins/installed/community/example", ""},
		{http.MethodPatch, "/api/plugins/installed/community/example", `{}`},
		{http.MethodPut, "/api/plugins/installed/community/example/fields", `{"fields":{}}`},
		{http.MethodGet, "/api/plugins/repositories", ""},
		{http.MethodDelete, "/api/plugins/repositories/community", ""},
	}
	for _, route := range management {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, roleName := range []string{models.RoleEditor, models.RoleViewer, noBooksRole} {
				rec := do(route.method, route.path, route.body, roleName)
				assert.Equal(t, http.StatusForbidden, rec.Code, "%s: %s", roleName, rec.Body.String())
			}
			assert.Equal(t, http.StatusUnauthorized, do(route.method, route.path, route.body, "").Code)
		})
	}

	// Admin still reaches the management handlers.
	rec := do(http.MethodPut, "/api/plugins/order/"+models.PluginHookMetadataEnricher, `{"order":[]}`, models.RoleAdmin)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = do(http.MethodGet, "/api/plugins/installed", "", models.RoleAdmin)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
