package ereader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// eReaderAccessFixture wires the real eReader routes against a database with
// two libraries and a viewer who owns a Browser Device and can access every
// library.
type eReaderAccessFixture struct {
	db     *bun.DB
	e      *echo.Echo
	user   *models.User
	device *devices.Device
	libA   *models.Library
	libB   *models.Library
}

func newEReaderAccessFixture(t *testing.T) *eReaderAccessFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
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

	f.device, err = devices.NewService(db).Create(ctx, f.user.ID, "eReader", devices.DeviceTypeBrowser)
	require.NoError(t, err)

	f.e = echo.New()
	f.e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(f.e, db, nil, books.NewService(db, appsettings.NewService(db)))
	return f
}

// withDevice stores device and its owner, loaded from the database, in c as
// DeviceAuth does. Tests that call a handler directly use it.
func withDevice(c echo.Context, t *testing.T, db *bun.DB, device *devices.Device) {
	t.Helper()
	owner, err := devices.NewService(db).AuthenticateOwner(context.Background(), device)
	require.NoError(t, err)
	devices.SetDevice(c, device)
	auth.SetUser(c, owner)
}

func (f *eReaderAccessFixture) serve(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ereader/key/"+f.device.Credential+path, nil))
	return rec
}

// A deactivated user's Device stops working.
func TestEReaderDeviceAuth_InactiveUser_Returns401(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	_, err := f.db.NewUpdate().Model((*models.User)(nil)).Set("is_active = ?", false).Where("id = ?", f.user.ID).Exec(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnauthorized, f.serve("/").Code)
}

// A Device whose owner's role lacks books:read cannot browse.
func TestEReaderDeviceAuth_RoleWithoutBooksRead_Returns403(t *testing.T) {
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

// A short URL for a deactivated user's Device does not redirect to its URL.
func TestResolveShortURL_InactiveOwner_Returns401(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	ctx := context.Background()
	short, err := devices.NewService(f.db).GenerateShortURL(ctx, f.user.ID, f.device.ID)
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
	assert.Empty(t, rec.Header().Get(echo.HeaderLocation), "the Device URL must not be revealed")
}

// A file, or a book, in a library the Device's owner cannot access is not found,
// the same rule Kobo applies to its per-file routes, so the id does not leak
// whether it exists.
func TestEReaderDownload_FileOutsideAccess_Returns404(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	ctx := context.Background()

	// Narrow the owner to library A.
	_, err := f.db.NewUpdate().Model((*models.UserLibraryAccess)(nil)).Set("library_id = ?", f.libA.ID).Where("user_id = ?", f.user.ID).Exec(ctx)
	require.NoError(t, err)

	dir := t.TempDir()
	book := &models.Book{
		LibraryID:       f.libB.ID,
		Title:           "Secret Book",
		Filepath:        dir,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Secret Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := &models.File{
		LibraryID:     f.libB.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(dir, "secret.epub"),
		FilesizeBytes: 1,
	}
	_, err = f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	for _, path := range []string{
		fmt.Sprintf("/file/%d", file.ID),
		fmt.Sprintf("/file/%d/kepub", file.ID),
		fmt.Sprintf("/file/%d/kindle/secret.mobi", file.ID),
		fmt.Sprintf("/download/%d", book.ID),
		fmt.Sprintf("/cover/%d", book.ID),
	} {
		rec := f.serve(path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "Secret Book", path)
	}

	// A library path outside the owner's access stays a 403.
	assert.Equal(t, http.StatusForbidden, f.serve(fmt.Sprintf("/libraries/%d/all", f.libB.ID)).Code)
}
