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
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// serveOPDSAs serves one OPDS GET signed in with username and password,
// which is either the account password or a Device credential.
func serveOPDSAs(t *testing.T, db *bun.DB, username, password, path string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = errcodes.NewHandler().Handle
	authMw := auth.NewMiddleware(auth.NewService(db, "test-secret", time.Hour))
	RegisterRoutes(e, db, authMw, downloadcache.NewCache(t.TempDir(), 1<<30), books.NewService(db, appsettings.NewService(db)))

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// getAs serves a GET signed in as "reader" with password, which is either
// the account password or a Device credential.
func (f *deliveryFixture) getAs(password, path string) (int, string) {
	f.t.Helper()
	rec := serveOPDSAs(f.t, f.db, "reader", password, path)
	return rec.Code, rec.Body.String()
}

// addDevice adds a Device of deviceType for "reader".
func (f *deliveryFixture) addDevice(deviceType string) *devices.Device {
	f.t.Helper()
	device, err := devices.NewService(f.db).Create(context.Background(), userID(f.t, f.db, "reader"), "Reader", deviceType)
	require.NoError(f.t, err)
	return device
}

func (f *deliveryFixture) allBooksPath() string {
	return fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID)
}

// An OPDS app Device follows the user's default until customized, so
// KOReader signed in with its credential sees what the password sees.
func TestOPDS_DeviceCredential_FollowingTheDefault(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	novel := f.addBook("Novel", "", models.FileTypeEPUB)
	comic := f.addBook("Comic", "", models.FileTypeCBZ)
	device := f.addDevice(devices.DeviceTypeOPDS)

	code, withPassword := f.getAs("secret", f.allBooksPath())
	require.Equal(t, http.StatusOK, code, withPassword)
	code, withDevice := f.getAs(device.Credential, f.allBooksPath())
	require.Equal(t, http.StatusOK, code, withDevice)
	for _, body := range []string{withPassword, withDevice} {
		assert.Contains(t, body, acquisitionHref(novel[models.FileTypeEPUB], "epub"))
		assert.Contains(t, body, acquisitionHref(comic[models.FileTypeCBZ], "cbz"))
	}
}

// Signed in with a Device credential, feeds and downloads follow the
// Device's rules; the account password keeps following the user's default.
func TestOPDS_DeviceCredential_AppliesTheDeviceRules(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	novel := f.addBook("Novel", "", models.FileTypeEPUB)
	comic := f.addBook("Comic", "", models.FileTypeCBZ)
	device := f.addDevice(devices.DeviceTypeOPDS)
	_, err := devices.NewService(f.db).SetRules(context.Background(), userID(t, f.db, "reader"), device.ID, devices.RulesModeCustom,
		[]delivery.Rule{{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeCBZ}})
	require.NoError(t, err)

	code, body := f.getAs(device.Credential, f.allBooksPath())
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(comic[models.FileTypeCBZ], "cbz"))
	assert.NotContains(t, body, "Novel", "a book with nothing to deliver is hidden")

	code, body = f.getAs("secret", f.allBooksPath())
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(novel[models.FileTypeEPUB], "epub"))
	assert.Contains(t, body, acquisitionHref(comic[models.FileTypeCBZ], "cbz"))
}

func TestOPDS_DeviceCredential_Refused(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	newOPDSUser(t, f.db, "other", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	opdsApp := f.addDevice(devices.DeviceTypeOPDS)
	kobo := f.addDevice(devices.DeviceTypeKobo)

	for name, creds := range map[string][2]string{
		"a wrong credential":                 {"reader", "abcdefghjkmnpqrs"},
		"a credential with another username": {"other", opdsApp.Credential},
		"a credential in capitals":           {"reader", toUpper(opdsApp.Credential)},
		"a Kobo Device's credential":         {"reader", kobo.Credential},
	} {
		t.Run(name, func(t *testing.T) {
			rec := serveOPDSAs(t, f.db, creds[0], creds[1], "/opds/v1/catalog")
			assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			assert.Equal(t, `Basic realm="Shisho OPDS"`, rec.Header().Get("WWW-Authenticate"))
		})
	}

	// Usernames match without regard to case, as the account password does.
	rec := serveOPDSAs(t, f.db, "READER", opdsApp.Credential, "/opds/v1/catalog")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// A Device whose owner is deactivated gets the same Basic challenge as a
// wrong password, so a reading app prompts for credentials again.
func TestOPDS_DeviceCredential_DeactivatedOwner(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	device := f.addDevice(devices.DeviceTypeOPDS)
	_, err := f.db.NewUpdate().Model((*models.User)(nil)).Set("is_active = ?", false).Where("username = ?", "reader").Exec(context.Background())
	require.NoError(t, err)

	rec := serveOPDSAs(t, f.db, "reader", device.Credential, "/opds/v1/catalog")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	assert.Equal(t, `Basic realm="Shisho OPDS"`, rec.Header().Get("WWW-Authenticate"))
}

func toUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// A Device's scope applies to OPDS as to its other connections. A library
// scope shows only that library; a list scope shows only that list. Anything
// outside it is not found, downloads and covers included.
func TestOPDS_DeviceCredential_AppliesTheDeviceScope(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	inScope := f.addBook("In Scope", "", models.FileTypeEPUB)
	inScopeBook := f.lastBook
	ctx := context.Background()
	other := &models.Library{Name: "Other Lib", CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)
	otherFile := &models.File{LibraryID: other.ID, BookID: 0, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/other/book.epub", FilesizeBytes: 1}
	otherBook := &models.Book{LibraryID: other.ID, Title: "Out Of Scope", TitleSource: models.DataSourceFilepath, SortTitle: "Out Of Scope", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/other"}
	_, err = f.db.NewInsert().Model(otherBook).Exec(ctx)
	require.NoError(t, err)
	otherFile.BookID = otherBook.ID
	_, err = f.db.NewInsert().Model(otherFile).Exec(ctx)
	require.NoError(t, err)

	owner := userID(t, f.db, "reader")
	deviceService := devices.NewService(f.db)

	t.Run("library", func(t *testing.T) {
		device := f.addDevice(devices.DeviceTypeOPDS)
		_, err := deviceService.Update(ctx, owner, device.ID, devices.UpdateDeviceOptions{Scope: &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.lib.ID}})
		require.NoError(t, err)

		code, body := f.getAs(device.Credential, "/opds/v1/catalog")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, fmt.Sprintf("/opds/v1/libraries/%d", f.lib.ID))
		assert.NotContains(t, body, "Other Lib")

		for _, path := range []string{
			fmt.Sprintf("/opds/v1/libraries/%d/all", other.ID),
			fmt.Sprintf("/opds/download/%d.epub", otherFile.ID),
			fmt.Sprintf("/opds/v1/books/%d/cover", otherBook.ID),
		} {
			code, body := f.getAs(device.Credential, path)
			assert.Equal(t, http.StatusNotFound, code, path+": "+body)
		}
		code, body = f.getAs(device.Credential, f.allBooksPath())
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, acquisitionHref(inScope[models.FileTypeEPUB], "epub"))
	})

	t.Run("list", func(t *testing.T) {
		listService := lists.NewService(f.db)
		list, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: owner, Name: "Shelf"})
		require.NoError(t, err)
		err = listService.AddBooks(ctx, lists.AddBooksOptions{ListID: list.ID, BookIDs: []int{inScopeBook.ID}, AddedByUserID: owner})
		require.NoError(t, err)
		otherList, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: owner, Name: "Elsewhere"})
		require.NoError(t, err)
		err = listService.AddBooks(ctx, lists.AddBooksOptions{ListID: otherList.ID, BookIDs: []int{otherBook.ID}, AddedByUserID: owner})
		require.NoError(t, err)

		device := f.addDevice(devices.DeviceTypeOPDS)
		_, err = deviceService.Update(ctx, owner, device.ID, devices.UpdateDeviceOptions{Scope: &devices.Scope{Type: devices.ScopeList, ListID: &list.ID}})
		require.NoError(t, err)

		code, body := f.getAs(device.Credential, "/opds/v1/catalog")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, fmt.Sprintf("/opds/v1/lists/%d", list.ID))
		assert.NotContains(t, body, "/opds/v1/libraries/")

		code, body = f.getAs(device.Credential, fmt.Sprintf("/opds/v1/lists/%d", list.ID))
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, acquisitionHref(inScope[models.FileTypeEPUB], "epub"))

		for _, path := range []string{
			f.allBooksPath(),
			fmt.Sprintf("/opds/v1/lists/%d", otherList.ID),
			fmt.Sprintf("/opds/download/%d.epub", otherFile.ID),
			fmt.Sprintf("/opds/v1/books/%d/cover", otherBook.ID),
		} {
			code, body := f.getAs(device.Credential, path)
			assert.Equal(t, http.StatusNotFound, code, path+": "+body)
		}
		code, body = f.getAs(device.Credential, "/opds/v1/lists")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, "Shelf")
		assert.NotContains(t, body, "Elsewhere")
	})
}
