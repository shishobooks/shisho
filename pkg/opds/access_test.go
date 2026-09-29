package opds

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// newOPDSUser inserts an active user with access to every library, a role
// holding only the given permissions, and the password "secret".
func newOPDSUser(t *testing.T, db *bun.DB, username string, permissions ...models.Permission) {
	t.Helper()
	ctx := context.Background()
	role := &models.Role{Name: username + "-role", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	for _, p := range permissions {
		p.RoleID = role.ID
		_, err = db.NewInsert().Model(&p).Exec(ctx)
		require.NoError(t, err)
	}
	hash, err := auth.HashPassword("secret")
	require.NoError(t, err)
	user := &models.User{Username: username, PasswordHash: hash, RoleID: role.ID, IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID}).Exec(ctx)
	require.NoError(t, err)
}

func serveOPDS(t *testing.T, db *bun.DB, username, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	authMw := auth.NewMiddleware(auth.NewService(db, "test-secret", time.Hour))
	RegisterRoutes(e, db, authMw, downloadcache.NewCache(t.TempDir(), 1<<30), books.NewService(db))

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":secret")))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// A role without books:read cannot browse or download over OPDS, the same
// rule Kobo and eReader keys follow.
func TestOPDS_RoleWithoutBooksRead_Returns403(t *testing.T) {
	t.Parallel()
	db := setupOPDSDB(t)
	newOPDSUser(t, db, "nobooks")

	// Every route family, including each download route and method. File 1
	// does not exist, so a download that skipped the check would be a 404.
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/opds/v1/epub/catalog"},
		{http.MethodGet, "/opds/v1/kepub/epub/catalog"},
		{http.MethodGet, "/opds/download/1"},
		{http.MethodHead, "/opds/download/1"},
		{http.MethodGet, "/opds/download/1/kepub"},
		{http.MethodHead, "/opds/download/1/kepub"},
	} {
		assert.Equal(t, http.StatusForbidden, serveOPDS(t, db, "nobooks", r.method, r.path).Code, r.method+" "+r.path)
	}
}

func TestOPDS_RoleWithBooksRead_Returns200(t *testing.T) {
	t.Parallel()
	db := setupOPDSDB(t)
	newOPDSUser(t, db, "reader", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})

	assert.Equal(t, http.StatusOK, serveOPDS(t, db, "reader", http.MethodGet, "/opds/v1/epub/catalog").Code)
}

// An OPDS handler reached with no user in context rejects the request with
// 401 instead of treating the caller as having access to every library.
// The routes are registered without BasicAuth.
func TestOPDSHandlers_NoUserInContext_Returns401(t *testing.T) {
	t.Parallel()
	db := setupOPDSDB(t)
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	bookService := books.NewService(db)
	h := &handler{
		opdsService:     NewService(db, bookService),
		bookService:     bookService,
		libraryService:  libraries.NewService(db),
		settingsService: settings.NewService(db),
	}
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	e.GET("/opds/v1/:types/catalog", h.catalog)
	e.GET("/opds/v1/:types/libraries/:libraryID/all", h.libraryAllBooks)

	for _, path := range []string{"/opds/v1/epub/catalog", fmt.Sprintf("/opds/v1/epub/libraries/%d/all", lib.ID)} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
}
