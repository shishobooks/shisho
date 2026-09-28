package ereader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// eReaderAccessFixture wires the real eReader routes against a database with
// two libraries and a viewer who owns an eReader API key and can access
// every library.
type eReaderAccessFixture struct {
	db   *bun.DB
	e    *echo.Echo
	user *models.User
	key  *apikeys.APIKey
	libA *models.Library
	libB *models.Library
}

func newEReaderAccessFixture(t *testing.T) *eReaderAccessFixture {
	t.Helper()
	ctx := context.Background()
	db := setupEReaderDB(t)
	f := &eReaderAccessFixture{db: db}

	for _, lib := range []**models.Library{&f.libA, &f.libB} {
		*lib = &models.Library{
			Name:                     "Library",
			CoverAspectRatio:         "book",
			DownloadFormatPreference: models.DownloadFormatOriginal,
			CreatedAt:                time.Now(),
			UpdatedAt:                time.Now(),
		}
		_, err := db.NewInsert().Model(*lib).Exec(ctx)
		require.NoError(t, err)
	}

	var viewerRoleID int
	require.NoError(t, db.QueryRow("SELECT id FROM roles WHERE name = ?", models.RoleViewer).Scan(&viewerRoleID))
	f.user = &models.User{Username: "reader", PasswordHash: "x", RoleID: viewerRoleID, IsActive: true}
	_, err := db.NewInsert().Model(f.user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: f.user.ID}).Exec(ctx)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	key, err := apiKeyService.Create(ctx, f.user.ID, "eReader")
	require.NoError(t, err)
	f.key, err = apiKeyService.AddPermission(ctx, f.user.ID, key.ID, apikeys.PermissionEReaderBrowser)
	require.NoError(t, err)

	f.e = echo.New()
	f.e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(f.e, db, nil, books.NewService(db))
	return f
}

// keyContext returns ctx with apiKey and its owner, loaded from the database,
// as APIKeyAuth stores them. Tests that call a handler directly use it.
func keyContext(ctx context.Context, t *testing.T, db *bun.DB, apiKey *apikeys.APIKey) context.Context {
	t.Helper()
	owner, err := apikeys.NewService(db).AuthenticateOwner(ctx, apiKey)
	require.NoError(t, err)
	ctx = context.WithValue(ctx, contextKeyAPIKey, apiKey)
	return context.WithValue(ctx, contextKeyUser, owner)
}

func (f *eReaderAccessFixture) serve(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ereader/key/"+f.key.Key+path, nil))
	return rec
}

// A deactivated user's eReader key stops working.
func TestEReaderAPIKeyAuth_InactiveUser_Returns401(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	_, err := f.db.NewUpdate().Model((*models.User)(nil)).Set("is_active = ?", false).Where("id = ?", f.user.ID).Exec(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, f.serve("/").Code)
}

// A key whose owner's role lacks books:read cannot browse.
func TestEReaderAPIKeyAuth_RoleWithoutBooksRead_Returns403(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	ctx := context.Background()
	role := &models.Role{Name: "no-books", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := f.db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewUpdate().Model((*models.User)(nil)).Set("role_id = ?", role.ID).Where("id = ?", f.user.ID).Exec(ctx)
	require.NoError(t, err)

	assert.Equal(t, http.StatusForbidden, f.serve("/").Code)
}

// A series from another library is not found under this library's path, so
// its name does not leak into the page title.
func TestEReaderSeriesBooks_SeriesFromOtherLibrary_Returns404(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	s := &models.Series{
		LibraryID:      f.libB.ID,
		Name:           "Secret Series",
		NameSource:     models.DataSourceManual,
		SortName:       "Secret Series",
		SortNameSource: models.DataSourceManual,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	_, err := f.db.NewInsert().Model(s).Exec(context.Background())
	require.NoError(t, err)

	rec := f.serve(fmt.Sprintf("/libraries/%d/series/%d", f.libA.ID, s.ID))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Secret Series")

	// Positive control: the series renders under its own library.
	rec = f.serve(fmt.Sprintf("/libraries/%d/series/%d", f.libB.ID, s.ID))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Secret Series")
}

// A short URL for a deactivated user's key does not redirect to the key URL.
func TestResolveShortURL_InactiveOwner_Returns401(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	ctx := context.Background()
	short, err := apikeys.NewService(f.db).GenerateShortURL(ctx, f.user.ID, f.key.ID)
	require.NoError(t, err)

	serveShort := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/e/"+short.ShortCode, nil))
		return rec
	}

	// Positive control: an active owner's short URL redirects.
	assert.Equal(t, http.StatusFound, serveShort().Code)

	_, err = f.db.NewUpdate().Model((*models.User)(nil)).Set("is_active = ?", false).Where("id = ?", f.user.ID).Exec(ctx)
	require.NoError(t, err)

	rec := serveShort()
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Header().Get(echo.HeaderLocation), "the key URL must not be revealed")
}
