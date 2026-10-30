package opds

import (
	"context"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/lists"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queueToRead puts books on reader's To Read in this order.
func (f *deliveryFixture) queueToRead(books ...*models.Book) {
	f.t.Helper()
	svc := reading.NewService(f.db)
	for _, b := range books {
		require.NoError(f.t, svc.AddToRead(context.Background(), userID(f.t, f.db, "reader"), b.ID))
	}
}

const toReadHref = `href="http://example.com/opds/v1/to-read"`

// The root catalog has To Read when it holds something to deliver, and the
// feed lists it in queue order.
func TestOPDS_ToReadFeedInQueueOrder(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)

	code, body := f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, toReadHref, "an empty To Read is left out")

	f.addBook("Zebra", "", models.FileTypeEPUB)
	zebra := f.lastBook
	f.addBook("Apple", "", models.FileTypeEPUB)
	apple := f.lastBook
	f.addBook("Mango", "", models.FileTypeEPUB)
	mango := f.lastBook
	f.addBook("Not Queued", "", models.FileTypeEPUB)
	f.queueToRead(mango, zebra, apple)
	require.NoError(t, reading.NewService(f.db).ReorderToRead(context.Background(), userID(t, f.db, "reader"), []int{zebra.ID, mango.ID, apple.ID}))

	code, body = f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, toReadHref)

	code, body = f.get("/opds/v1/to-read")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Zebra", "Mango", "Apple"}, entryTitles(body))
}

func TestOPDS_ToReadFeedPaginates(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	var queued []*models.Book
	for _, title := range []string{"One", "Two", "Three"} {
		f.addBook(title, "", models.FileTypeEPUB)
		queued = append(queued, f.lastBook)
	}
	f.queueToRead(queued...)

	code, body := f.get("/opds/v1/to-read?limit=2&offset=2")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Three"}, entryTitles(body))
	assert.Contains(t, body, `rel="previous"`)
}

// Books the Delivery Rules have nothing to deliver of are left out, and a
// To Read with nothing to deliver is left out of the catalog.
func TestOPDS_ToReadFeedFollowsTheDeliveryRules(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("Comic", "", models.FileTypeCBZ)
	comic := f.lastBook
	f.addBook("Novel", "", models.FileTypeEPUB)
	novel := f.lastBook
	f.queueToRead(comic, novel)
	setRules(t, f.db, "reader", epubOnly)

	code, body := f.get("/opds/v1/to-read")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Novel"}, entryTitles(body))

	setRules(t, f.db, "reader", []delivery.Rule{{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeM4B}})
	code, body = f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, toReadHref)
}

// A Device sees only the To Read Books in its scope.
func TestOPDS_ToReadFeedFollowsTheDeviceScope(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("In Scope", "", models.FileTypeEPUB)
	inScope := f.lastBook
	ctx := context.Background()
	other := &models.Library{Name: "Other Lib", CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)
	otherBook := &models.Book{LibraryID: other.ID, Title: "Out Of Scope", TitleSource: models.DataSourceFilepath, SortTitle: "Out Of Scope", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/other"}
	_, err = f.db.NewInsert().Model(otherBook).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.File{LibraryID: other.ID, BookID: otherBook.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/other/book.epub", FilesizeBytes: 1}).Exec(ctx)
	require.NoError(t, err)
	f.queueToRead(otherBook, inScope)

	device := f.addDevice(devices.DeviceTypeOPDS)
	_, err = devices.NewService(f.db).Update(ctx, userID(t, f.db, "reader"), device.ID, devices.UpdateDeviceOptions{Scope: &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.lib.ID}})
	require.NoError(t, err)

	code, body := f.getAs(device.Credential, "/opds/v1/to-read")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"In Scope"}, entryTitles(body))

	code, body = f.get("/opds/v1/to-read")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Out Of Scope", "In Scope"}, entryTitles(body), "the account password reaches every library")

	// A list-scoped Device sees only the To Read Books on its list.
	owner := userID(t, f.db, "reader")
	listService := lists.NewService(f.db)
	list, err := listService.CreateList(ctx, lists.CreateListOptions{UserID: owner, Name: "Scope"})
	require.NoError(t, err)
	require.NoError(t, listService.AddBooks(ctx, lists.AddBooksOptions{ListID: list.ID, BookIDs: []int{otherBook.ID}, AddedByUserID: owner}))
	listDevice := f.addDevice(devices.DeviceTypeOPDS)
	_, err = devices.NewService(f.db).Update(ctx, owner, listDevice.ID, devices.UpdateDeviceOptions{Scope: &devices.Scope{Type: devices.ScopeList, ListID: &list.ID}})
	require.NoError(t, err)

	code, body = f.getAs(listDevice.Credential, "/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, toReadHref)
	code, body = f.getAs(listDevice.Credential, "/opds/v1/to-read")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Out Of Scope"}, entryTitles(body))
}
