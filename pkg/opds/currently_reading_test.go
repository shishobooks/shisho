package opds

import (
	"context"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startReading starts a Read-through of book for reader on day, pausing it
// when paused.
func (f *deliveryFixture) startReading(book *models.Book, day string, paused bool) {
	f.t.Helper()
	svc := reading.NewService(f.db)
	reader := userID(f.t, f.db, "reader")
	require.NoError(f.t, svc.Start(context.Background(), reader, book.ID, day))
	if paused {
		require.NoError(f.t, svc.SetPaused(context.Background(), reader, book.ID, true))
	}
}

const currentlyReadingHref = `href="http://example.com/opds/v1/currently-reading"`

// The root catalog has Currently Reading when something underway has
// something to deliver, and the feed lists the Books being read before the
// paused ones, each most recently begun first.
func TestOPDS_CurrentlyReadingFeedPutsPausedLast(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)

	code, body := f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, currentlyReadingHref, "nothing underway is left out")

	f.addBook("Paused", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-03-01", true)
	f.addBook("Older", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-01-01", false)
	f.addBook("Newer", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-02-01", false)
	f.addBook("Finished", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-01-01", false)
	require.NoError(t, reading.NewService(f.db).End(context.Background(), userID(t, f.db, "reader"), f.lastBook.ID, models.ReadThroughOutcomeFinished, "2026-01-02"))
	f.addBook("Untouched", "", models.FileTypeEPUB)

	code, body = f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, currentlyReadingHref)

	code, body = f.get("/opds/v1/currently-reading")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Newer", "Older", "Paused"}, entryTitles(body))
}

// Books the Delivery Rules have nothing to deliver of are left out, and a
// Currently Reading with nothing to deliver is left out of the catalog.
func TestOPDS_CurrentlyReadingFeedFollowsTheDeliveryRules(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("Comic", "", models.FileTypeCBZ)
	f.startReading(f.lastBook, "2026-01-01", false)
	f.addBook("Novel", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-01-01", false)
	setRules(t, f.db, "reader", epubOnly)

	code, body := f.get("/opds/v1/currently-reading")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Novel"}, entryTitles(body))

	setRules(t, f.db, "reader", []delivery.Rule{{SourceFileType: models.FileTypeM4B, DeliveredFileType: models.FileTypeM4B}})
	code, body = f.get("/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, currentlyReadingHref)
}

// A Device sees only the Books underway in its scope.
func TestOPDS_CurrentlyReadingFeedFollowsTheDeviceScope(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("In Scope", "", models.FileTypeEPUB)
	f.startReading(f.lastBook, "2026-01-01", false)
	ctx := context.Background()
	other := &models.Library{Name: "Other Lib", CoverAspectRatio: "book"}
	_, err := f.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)
	otherBook := &models.Book{LibraryID: other.ID, Title: "Out Of Scope", TitleSource: models.DataSourceFilepath, SortTitle: "Out Of Scope", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/other"}
	_, err = f.db.NewInsert().Model(otherBook).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.File{LibraryID: other.ID, BookID: otherBook.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: "/other/book.epub", FilesizeBytes: 1}).Exec(ctx)
	require.NoError(t, err)
	f.startReading(otherBook, "2026-02-01", false)

	device := f.addDevice(devices.DeviceTypeOPDS)
	_, err = devices.NewService(f.db).Update(ctx, userID(t, f.db, "reader"), device.ID, devices.UpdateDeviceOptions{Scope: &devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.lib.ID}})
	require.NoError(t, err)

	code, body := f.getAs(device.Credential, "/opds/v1/currently-reading")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"In Scope"}, entryTitles(body))

	code, body = f.get("/opds/v1/currently-reading")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Out Of Scope", "In Scope"}, entryTitles(body), "the account password reaches every library")
}

// A library sort saved as the default on the web, here by rating, applies
// to the library's OPDS feed with the reader's own Ratings.
func TestOPDS_LibraryFeedSortsByTheReadersRating(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	reader := userID(t, f.db, "reader")
	svc := reading.NewService(f.db)
	for _, b := range []struct {
		title string
		units int
	}{{"Low", 4}, {"High", 20}, {"Mid", 12}} {
		f.addBook(b.title, "", models.FileTypeEPUB)
		units := b.units
		require.NoError(t, svc.SetRating(context.Background(), reader, f.lastBook.ID, &units))
	}
	stored := "rating:desc"
	_, err := settings.NewService(f.db).UpsertLibrarySort(context.Background(), reader, f.lib.ID, &stored)
	require.NoError(t, err)

	code, body := f.get(f.allBooksPath())
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"High", "Mid", "Low"}, entryTitles(body))
}
