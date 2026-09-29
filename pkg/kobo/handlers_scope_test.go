package kobo

import (
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/apikeys"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// koboScopeFixture wires the real Kobo routes (API key middleware, scope
// parser and handlers) against a database holding two libraries, each with
// one EPUB that exists on disk and has a cover. The key's owner is a viewer
// with access to every library unless a test narrows it.
type koboScopeFixture struct {
	db    *bun.DB
	e     *echo.Echo
	user  *models.User
	key   *apikeys.APIKey
	libA  *models.Library
	libB  *models.Library
	bookA *models.Book
	bookB *models.Book
	fileA *models.File
	fileB *models.File
}

func newKoboScopeFixture(t *testing.T) *koboScopeFixture {
	t.Helper()
	ctx := context.Background()
	db := newSyncPointTestDB(t)
	f := &koboScopeFixture{db: db}

	f.libA = insertScopeLibrary(ctx, t, db, "Library A")
	f.libB = insertScopeLibrary(ctx, t, db, "Library B")

	var viewerRoleID int
	require.NoError(t, db.QueryRow("SELECT id FROM roles WHERE name = ?", models.RoleViewer).Scan(&viewerRoleID))
	f.user = &models.User{
		Username:     "koboreader",
		PasswordHash: "x",
		RoleID:       viewerRoleID,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(f.user).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: f.user.ID}).Exec(ctx)
	require.NoError(t, err)

	apiKeyService := apikeys.NewService(db)
	key, err := apiKeyService.Create(ctx, f.user.ID, "Kobo")
	require.NoError(t, err)
	f.key, err = apiKeyService.AddPermission(ctx, f.user.ID, key.ID, apikeys.PermissionKoboSync)
	require.NoError(t, err)

	f.bookA, f.fileA = insertScopeBookWithFile(ctx, t, db, f.libA.ID, "Alpha", models.FileTypeEPUB, models.FileRoleMain)
	f.bookB, f.fileB = insertScopeBookWithFile(ctx, t, db, f.libB.ID, "Bravo", models.FileTypeEPUB, models.FileRoleMain)

	f.e = echo.New()
	f.e.HTTPErrorHandler = errcodes.NewHandler().Handle
	RegisterRoutes(f.e, db, downloadcache.NewCache(t.TempDir(), 1<<30), books.NewService(db))

	return f
}

// restrictToLibrary replaces the owner's all-libraries grant with a grant on
// a single library.
func (f *koboScopeFixture) restrictToLibrary(t *testing.T, libraryID int) {
	t.Helper()
	ctx := context.Background()
	_, err := f.db.NewDelete().Model((*models.UserLibraryAccess)(nil)).Where("user_id = ?", f.user.ID).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.UserLibraryAccess{UserID: f.user.ID, LibraryID: &libraryID}).Exec(ctx)
	require.NoError(t, err)
}

// insertList creates a list owned by the key's owner holding the given books.
func (f *koboScopeFixture) insertList(t *testing.T, bookIDs ...int) *models.List {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	list := &models.List{UserID: f.user.ID, Name: "Sync", DefaultSort: "added_at", CreatedAt: now, UpdatedAt: now}
	_, err := f.db.NewInsert().Model(list).Exec(ctx)
	require.NoError(t, err)
	for _, bookID := range bookIDs {
		_, err = f.db.NewInsert().Model(&models.ListBook{ListID: list.ID, BookID: bookID, AddedAt: now}).Exec(ctx)
		require.NoError(t, err)
	}
	return list
}

func (f *koboScopeFixture) serve(method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.e.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// prefix returns the route prefix for a scope segment such as "all",
// "library/3" or "list/7".
func (f *koboScopeFixture) prefix(scope string) string {
	return "/kobo/" + f.key.Key + "/" + scope
}

// downloadPaths returns both download routes for a file under a scope.
func (f *koboScopeFixture) downloadPaths(scope string, fileID int) []string {
	return []string{
		f.prefix(scope) + "/v1/books/" + ShishoID(fileID) + "/file/epub",
		f.prefix(scope) + "/download/" + ShishoID(fileID) + "/kepub",
	}
}

func insertScopeLibrary(ctx context.Context, t *testing.T, db *bun.DB, name string) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	return lib
}

// insertScopeBookWithFile creates a book with one file that exists on disk
// and has a cover image next to it.
func insertScopeBookWithFile(ctx context.Context, t *testing.T, db *bun.DB, libraryID int, title, fileType, fileRole string) (*models.Book, *models.File) {
	t.Helper()
	dir := t.TempDir()
	book := &models.Book{
		LibraryID:       libraryID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        dir,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	file := insertScopeFile(ctx, t, db, book, dir, title, fileType, fileRole)
	return book, file
}

func insertScopeFile(ctx context.Context, t *testing.T, db *bun.DB, book *models.Book, dir, name, fileType, fileRole string) *models.File {
	t.Helper()
	filePath := filepath.Join(dir, name+"."+fileType)
	require.NoError(t, os.WriteFile(filePath, []byte("not a real "+fileType), 0o600))

	coverFilename := name + "." + fileType + ".cover.jpg"
	coverFile, err := os.Create(filepath.Join(dir, coverFilename))
	require.NoError(t, err)
	require.NoError(t, jpeg.Encode(coverFile, image.NewRGBA(image.Rect(0, 0, 20, 30)), nil))
	require.NoError(t, coverFile.Close())

	mimeType := "image/jpeg"
	file := &models.File{
		LibraryID:          book.LibraryID,
		BookID:             book.ID,
		Filepath:           filePath,
		FileType:           fileType,
		FileRole:           fileRole,
		FilesizeBytes:      16,
		CoverImageFilename: &coverFilename,
		CoverMimeType:      &mimeType,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	return file
}

// withKeyOwner stores an all-libraries key owner in c, as APIKeyAuth does,
// for tests that call a handler directly. With no scope in context the
// handlers use the "all" scope.
func withKeyOwner(c echo.Context) {
	auth.SetUser(c, &models.User{ID: 1, IsActive: true, LibraryAccess: []*models.UserLibraryAccess{{}}})
}

func assertFileNotFound(t *testing.T, rec *httptest.ResponseRecorder, method string) {
	t.Helper()
	assert.Equal(t, http.StatusNotFound, rec.Code)
	if method != http.MethodHead {
		assert.Contains(t, rec.Body.String(), "File not found.")
	}
}

// A key scoped to library A cannot download a file from library B, even
// though its owner can access library B, on either download route, GET or HEAD.
func TestHandleDownload_FileOutsideLibraryScope_Returns404(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	scope := fmt.Sprintf("library/%d", f.libA.ID)

	for _, path := range f.downloadPaths(scope, f.fileB.ID) {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+" "+path, func(t *testing.T) {
				t.Parallel()
				assertFileNotFound(t, f.serve(method, path), method)
			})
		}
	}

	// Positive control: the in-scope file still downloads on both routes.
	for _, path := range f.downloadPaths(scope, f.fileA.ID) {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assert.Equal(t, http.StatusOK, f.serve(method, path).Code, "%s %s", method, path)
		}
	}
}

// An "all" scope key cannot download a file from a library its owner has no
// access to.
func TestHandleDownload_AllScope_FileInInaccessibleLibrary_Returns404(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.restrictToLibrary(t, f.libA.ID)

	for _, path := range f.downloadPaths("all", f.fileB.ID) {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assertFileNotFound(t, f.serve(method, path), method)
		}
	}
	for _, path := range f.downloadPaths("all", f.fileA.ID) {
		assert.Equal(t, http.StatusOK, f.serve(http.MethodGet, path).Code, path)
	}
}

// A list scope key only downloads files of books on the list.
func TestHandleDownload_ListScope_FileNotInList_Returns404(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	list := f.insertList(t, f.bookA.ID)
	scope := fmt.Sprintf("list/%d", list.ID)

	for _, path := range f.downloadPaths(scope, f.fileB.ID) {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			assertFileNotFound(t, f.serve(method, path), method)
		}
	}
	// Positive control: the listed book's file downloads.
	for _, path := range f.downloadPaths(scope, f.fileA.ID) {
		assert.Equal(t, http.StatusOK, f.serve(http.MethodGet, path).Code, path)
	}
}

func TestHandleCover_FileOutsideScope_Returns404(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	prefix := f.prefix(fmt.Sprintf("library/%d", f.libA.ID))

	rec := f.serve(http.MethodGet, prefix+"/v1/books/"+ShishoID(f.fileB.ID)+"/thumbnail/100/150/false/image.jpg")
	assertFileNotFound(t, rec, http.MethodGet)

	rec = f.serve(http.MethodGet, prefix+"/v1/books/"+ShishoID(f.fileA.ID)+"/thumbnail/100/150/false/image.jpg")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleMetadata_FileOutsideScope_Returns404(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	prefix := f.prefix(fmt.Sprintf("library/%d", f.libA.ID))

	rec := f.serve(http.MethodGet, prefix+"/v1/library/"+ShishoID(f.fileB.ID)+"/metadata")
	assertFileNotFound(t, rec, http.MethodGet)
	assert.NotContains(t, rec.Body.String(), "Bravo", "metadata of an out-of-scope file must not leak")

	rec = f.serve(http.MethodGet, prefix+"/v1/library/"+ShishoID(f.fileA.ID)+"/metadata")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Alpha")
}

// A deactivated user's key stops working on every Kobo route.
func TestKoboAPIKeyAuth_InactiveUser_Returns401(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	_, err := f.db.NewUpdate().Model((*models.User)(nil)).Set("is_active = ?", false).Where("id = ?", f.user.ID).Exec(context.Background())
	require.NoError(t, err)

	for _, path := range []string{
		f.prefix("all") + "/v1/library/sync",
		f.downloadPaths("all", f.fileA.ID)[0],
	} {
		assert.Equal(t, http.StatusUnauthorized, f.serve(http.MethodGet, path).Code, path)
	}
}

// A key whose owner's role lacks books:read cannot sync or download.
func TestKoboAPIKeyAuth_RoleWithoutBooksRead_Returns403(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	ctx := context.Background()
	role := &models.Role{Name: "no-books", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := f.db.NewInsert().Model(role).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewUpdate().Model((*models.User)(nil)).Set("role_id = ?", role.ID).Where("id = ?", f.user.ID).Exec(ctx)
	require.NoError(t, err)

	for _, path := range []string{
		f.prefix("all") + "/v1/library/sync",
		f.downloadPaths("all", f.fileA.ID)[0],
	} {
		assert.Equal(t, http.StatusForbidden, f.serve(http.MethodGet, path).Code, path)
	}
}
