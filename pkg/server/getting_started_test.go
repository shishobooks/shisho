package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every signed-in user has a Getting Started list of their own, whatever
// their role; showing it to another user takes Users Write.
func TestGettingStartedRoutes_Permissions(t *testing.T) {
	t.Parallel()
	f := newRoutePermissionsFixture(t)
	nobody := f.user("nobody", f.role("no-permissions"))
	viewer := f.user("reader", models.RoleViewer)
	userReader := f.user("user-reader", f.role("user-reader", "users:read"))

	f.expect(nil, http.MethodGet, "/api/getting-started", "", http.StatusUnauthorized)
	f.expect(nobody, http.MethodGet, "/api/getting-started", "", http.StatusOK)
	f.expect(nobody, http.MethodPost, "/api/getting-started/dismiss", "", http.StatusOK)
	f.expect(nobody, http.MethodPost, "/api/getting-started/steps/device/not-for-me", "", http.StatusOK)

	show := fmt.Sprintf("/api/users/%d/show-getting-started", viewer.ID)
	f.expect(nil, http.MethodPost, show, "", http.StatusUnauthorized)
	f.expect(viewer, http.MethodPost, show, "", http.StatusForbidden)
	f.expect(userReader, http.MethodPost, show, "", http.StatusForbidden)
	f.expect(f.admin, http.MethodPost, show, "", http.StatusNoContent)
}

// In Demo Mode the list is reported off even for a user it is on for.
func TestGettingStartedRoutes_DemoModeReportsItOff(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	cfg := config.NewForTest()
	cfg.DemoMode = true
	cfg.CacheDir = t.TempDir()
	srv, err := New(cfg, db, &worker.Worker{}, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	user := insertPermissionTestUser(t.Context(), t, db, "visitor", models.RoleViewer, nil)
	_, err = db.NewUpdate().Model((*models.User)(nil)).
		Set("getting_started_enabled = TRUE").
		Set("getting_started_auto_open = TRUE").
		Where("id = ?", user.ID).Exec(t.Context())
	require.NoError(t, err)

	token, err := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()).GenerateToken(user)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/api/getting-started", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"enabled":false,"dismissed":false,"auto_open":false,"steps":[]}`, rec.Body.String())
}
