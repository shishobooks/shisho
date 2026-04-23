package libraries

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/binder"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newDeleteTestServer wires up the libraries DELETE route against a real DB
// and a stubbed auth middleware that injects the provided user into the
// request context. Returns the Echo instance plus a counter that increments
// each time onLibraryChanged fires.
func newDeleteTestServer(t *testing.T, db *bun.DB, user *models.User) (*echo.Echo, *int) {
	t.Helper()

	e := echo.New()
	b, err := binder.New()
	require.NoError(t, err)
	e.Binder = b
	e.HTTPErrorHandler = errcodes.NewHandler().Handle

	// Inject user context middleware that mirrors the real auth middleware.
	stubAuth := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if user != nil {
				c.Set("user", user)
				c.Set("user_id", user.ID)
			}
			return next(c)
		}
	}

	cfg := config.NewForTest()
	authService := auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration())
	authMiddleware := auth.NewMiddleware(authService)

	callbacks := 0
	g := e.Group("/libraries")
	g.Use(stubAuth)
	RegisterRoutesWithGroup(g, db, authMiddleware, RegisterRoutesOptions{
		OnLibraryChanged: func() { callbacks++ },
	})
	return e, &callbacks
}

func seedUser(ctx context.Context, t *testing.T, db *bun.DB, roleName string, allLibraryAccess bool) *models.User {
	t.Helper()

	role := &models.Role{}
	err := db.NewSelect().Model(role).Where("name = ?", roleName).Scan(ctx)
	require.NoError(t, err)

	u := &models.User{
		Username:     roleName + "-user",
		PasswordHash: "unused",
		RoleID:       role.ID,
		IsActive:     true,
		Role:         role,
	}
	_, err = db.NewInsert().Model(u).Returning("*").Exec(ctx)
	require.NoError(t, err)

	// Load permissions so user.HasPermission works inside middleware.
	err = db.NewSelect().Model(u).Relation("Role").Relation("Role.Permissions").Where("u.id = ?", u.ID).Scan(ctx)
	require.NoError(t, err)

	if allLibraryAccess {
		// nil LibraryID means access to all libraries.
		access := &models.UserLibraryAccess{UserID: u.ID, LibraryID: nil}
		_, err = db.NewInsert().Model(access).Returning("*").Exec(ctx)
		require.NoError(t, err)
		u.LibraryAccess = []*models.UserLibraryAccess{access}
	}

	return u
}

func TestDeleteLibraryHandler_HappyPath(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	admin := seedUser(ctx, t, db, models.RoleAdmin, true)
	e, callbacks := newDeleteTestServer(t, db, admin)

	seeded := seedLibraryWithContent(ctx, t, db, "Doomed")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)

	count, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "library should be gone")

	assert.Equal(t, 1, *callbacks, "onLibraryChanged should fire once on success")
}

func TestDeleteLibraryHandler_NotFound(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	admin := seedUser(ctx, t, db, models.RoleAdmin, true)
	e, _ := newDeleteTestServer(t, db, admin)

	req := httptest.NewRequest(http.MethodDelete, "/libraries/99999", nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestDeleteLibraryHandler_RequiresWritePermission(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	viewer := seedUser(ctx, t, db, models.RoleViewer, true)
	e, callbacks := newDeleteTestServer(t, db, viewer)

	seeded := seedLibraryWithContent(ctx, t, db, "Protected")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)

	count, err := db.NewSelect().Model((*models.Library)(nil)).Where("id = ?", seeded.LibraryID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "library must survive")

	assert.Zero(t, *callbacks, "onLibraryChanged must not fire on 403")
}

func TestDeleteLibraryHandler_RequiresLibraryAccess(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	ctx := context.Background()

	editor := seedUser(ctx, t, db, models.RoleEditor, false) // no library access
	e, _ := newDeleteTestServer(t, db, editor)

	seeded := seedLibraryWithContent(ctx, t, db, "NotYours")

	req := httptest.NewRequest(http.MethodDelete, "/libraries/"+strconv.Itoa(seeded.LibraryID), nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}
