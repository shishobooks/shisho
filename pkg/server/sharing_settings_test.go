package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// sharingSettingsFixture serves the real route table so the settings group
// middleware and the permission checks on the sharing endpoint are exercised
// together.
type sharingSettingsFixture struct {
	db           *bun.DB
	handler      http.Handler
	authSvc      *auth.Service
	admin        *models.User
	editor       *models.User
	viewer       *models.User
	sharesReader *models.User // custom role with Shares Read only
	sharesWriter *models.User // custom role with Shares Read and Write
	configReader *models.User // custom role with Config Read only
}

func newSharingSettingsFixture(t *testing.T, demoMode bool) *sharingSettingsFixture {
	t.Helper()
	ctx := context.Background()

	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	cfg.DemoMode = demoMode
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil, nil, nil), nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	f := &sharingSettingsFixture{
		db:      db,
		handler: srv.Handler,
		authSvc: auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
	}
	f.admin = insertPermissionTestUser(ctx, t, db, "admin", models.RoleAdmin, nil)
	f.editor = insertPermissionTestUser(ctx, t, db, "editor", models.RoleEditor, nil)
	f.viewer = insertPermissionTestUser(ctx, t, db, "viewer", models.RoleViewer, nil)
	f.sharesReader = f.insertCustomUser(ctx, t, "shares-reader", []*models.Permission{
		{Resource: models.ResourceShares, Operation: models.OperationRead},
	})
	f.sharesWriter = f.insertCustomUser(ctx, t, "shares-writer", []*models.Permission{
		{Resource: models.ResourceShares, Operation: models.OperationRead},
		{Resource: models.ResourceShares, Operation: models.OperationWrite},
	})
	f.configReader = f.insertCustomUser(ctx, t, "config-reader", []*models.Permission{
		{Resource: models.ResourceConfig, Operation: models.OperationRead},
	})
	return f
}

// insertCustomUser creates a role with exactly the given permissions and a
// user holding it.
func (f *sharingSettingsFixture) insertCustomUser(ctx context.Context, t *testing.T, name string, perms []*models.Permission) *models.User {
	t.Helper()
	role := &models.Role{Name: name}
	_, err := f.db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	for _, p := range perms {
		p.RoleID = role.ID
		_, err = f.db.NewInsert().Model(p).Exec(ctx)
		require.NoError(t, err)
	}
	return insertPermissionTestUser(ctx, t, f.db, name, role.Name, nil)
}

func (f *sharingSettingsFixture) do(t *testing.T, user *models.User, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := f.authSvc.GenerateToken(user)
	require.NoError(t, err)
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func TestSharingSettings_FreshDatabaseDefaults(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	rec := f.do(t, f.admin, http.MethodGet, "/api/settings/sharing", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"enabled":false,"require_expiration":false}`, rec.Body.String())
}

func TestSharingSettings_ReadPermissions(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	tests := []struct {
		name   string
		user   *models.User
		status int
	}{
		{"admin", f.admin, http.StatusOK},
		{"shares read only", f.sharesReader, http.StatusOK},
		{"config read only", f.configReader, http.StatusOK},
		{"editor has neither", f.editor, http.StatusForbidden},
		{"viewer has neither", f.viewer, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := f.do(t, tt.user, http.MethodGet, "/api/settings/sharing", "")
			assert.Equal(t, tt.status, rec.Code, rec.Body.String())
		})
	}
}

func TestSharingSettings_WriteRequiresConfigWrite(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	for _, user := range []*models.User{f.configReader, f.sharesWriter, f.editor, f.viewer} {
		rec := f.do(t, user, http.MethodPut, "/api/settings/sharing", `{"enabled":true}`)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s: %s", user.Username, rec.Body.String())
	}

	rec := f.do(t, f.admin, http.MethodGet, "/api/settings/sharing", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"enabled":false,"require_expiration":false}`, rec.Body.String(), "rejected writes must not persist")
}

func TestSharingSettings_WritePersistsPartialUpdates(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	rec := f.do(t, f.admin, http.MethodPut, "/api/settings/sharing", `{"enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"enabled":true,"require_expiration":false}`, rec.Body.String())

	rec = f.do(t, f.admin, http.MethodPut, "/api/settings/sharing", `{"require_expiration":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"enabled":true,"require_expiration":true}`, rec.Body.String(), "omitted fields keep their saved value")

	rec = f.do(t, f.sharesReader, http.MethodGet, "/api/settings/sharing", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"enabled":true,"require_expiration":true}`, rec.Body.String())

	rec = f.do(t, f.admin, http.MethodPut, "/api/settings/sharing", `{"enabled":false}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"enabled":false,"require_expiration":true}`, rec.Body.String())
}

func TestSharingSettings_WriteRejectedInDemoMode(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, true)

	rec := f.do(t, f.admin, http.MethodPut, "/api/settings/sharing", `{"enabled":true}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), `"code":"demo_mode"`)

	rec = f.do(t, f.admin, http.MethodGet, "/api/settings/sharing", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"enabled":false,"require_expiration":false}`, rec.Body.String())
}

func TestSharesPermission_SeededOnAdminOnly(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	permissions := func(user *models.User) []string {
		t.Helper()
		rec := f.do(t, user, http.MethodGet, "/api/auth/me", "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var me auth.MeResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
		return me.Permissions
	}

	adminPerms := permissions(f.admin)
	assert.Contains(t, adminPerms, "shares:read")
	assert.Contains(t, adminPerms, "shares:write")
	for _, user := range []*models.User{f.editor, f.viewer} {
		perms := permissions(user)
		assert.NotContains(t, perms, "shares:read", user.Username)
		assert.NotContains(t, perms, "shares:write", user.Username)
	}
}

func TestSharesPermission_GrantableToCustomRole(t *testing.T) {
	t.Parallel()
	f := newSharingSettingsFixture(t, false)

	rec := f.do(t, f.admin, http.MethodPost, "/api/roles", `{"name":"sharer","permissions":[{"resource":"shares","operation":"read"},{"resource":"shares","operation":"write"}]}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var role models.Role
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &role))
	var got []string
	for _, p := range role.Permissions {
		got = append(got, p.Resource+":"+p.Operation)
	}
	assert.ElementsMatch(t, []string{"shares:read", "shares:write"}, got)
}
