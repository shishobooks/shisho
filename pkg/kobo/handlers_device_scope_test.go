package kobo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncedFileIDs runs a fresh sync at path and returns the file ids of the
// books it sends.
func (f *koboScopeFixture) syncedFileIDs(t *testing.T, path string) []string {
	t.Helper()
	rec := f.serve(http.MethodGet, path+"/v1/library/sync")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var items []syncItem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &items))
	ids := []string{}
	for _, item := range items {
		require.NotNil(t, item.NewEntitlement)
		id, _ := item.NewEntitlement.BookEntitlement["Id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// resetSync forgets what the Device has synced, so the next sync sends
// everything in scope.
func (f *koboScopeFixture) resetSync(t *testing.T) {
	t.Helper()
	require.NoError(t, devices.NewService(f.db).ClearKoboSyncHistory(context.Background(), f.user.ID, f.device.ID))
}

// markMigrated makes the Device look like one migrated from an API key,
// whose scope still comes from its old sync URL.
func (f *koboScopeFixture) markMigrated(t *testing.T) {
	t.Helper()
	_, err := f.db.ExecContext(context.Background(), `UPDATE devices SET url_scope_pending = TRUE WHERE id = ?`, f.device.ID)
	require.NoError(t, err)
}

func (f *koboScopeFixture) storedScope(t *testing.T) devices.Scope {
	t.Helper()
	device, err := devices.NewService(f.db).Retrieve(context.Background(), f.user.ID, f.device.ID)
	require.NoError(t, err)
	return device.Scope()
}

// A scope set on the Device decides what its sync URL, which has no scope
// segment, sends.
func TestHandleSync_UsesTheDeviceScope(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	assert.ElementsMatch(t, []string{ShishoID(f.fileA.ID), ShishoID(f.fileB.ID)}, f.syncedFileIDs(t, f.prefix()))

	f.setScope(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libB.ID})
	f.resetSync(t)
	assert.Equal(t, []string{ShishoID(f.fileB.ID)}, f.syncedFileIDs(t, f.prefix()))

	list := f.insertList(t, f.bookA.ID)
	f.setScope(t, devices.Scope{Type: devices.ScopeList, ListID: &list.ID})
	f.resetSync(t)
	assert.Equal(t, []string{ShishoID(f.fileA.ID)}, f.syncedFileIDs(t, f.prefix()))
}

// A migrated Kobo's first request after upgrading sets the Device's scope
// from its old sync URL. After that the URL's scope segment is ignored.
func TestHandleSync_MigratedURLScopeAppliesOnce(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.markMigrated(t)

	legacyA := f.legacyPrefix(fmt.Sprintf("library/%d", f.libA.ID))
	assert.Equal(t, []string{ShishoID(f.fileA.ID)}, f.syncedFileIDs(t, legacyA))
	assert.Equal(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libA.ID}, f.storedScope(t))

	f.resetSync(t)
	legacyB := f.legacyPrefix(fmt.Sprintf("library/%d", f.libB.ID))
	assert.Equal(t, []string{ShishoID(f.fileA.ID)}, f.syncedFileIDs(t, legacyB), "a later URL scope is ignored")
	f.resetSync(t)
	assert.Equal(t, []string{ShishoID(f.fileA.ID)}, f.syncedFileIDs(t, f.legacyPrefix("all")))
	assert.Equal(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libA.ID}, f.storedScope(t))
}

// A migrated list URL carries its list onto the Device.
func TestHandleSync_MigratedListURLSetsListScope(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	list := f.insertList(t, f.bookB.ID)
	f.markMigrated(t)

	assert.Equal(t, []string{ShishoID(f.fileB.ID)}, f.syncedFileIDs(t, f.legacyPrefix(fmt.Sprintf("list/%d", list.ID))))
	assert.Equal(t, devices.Scope{Type: devices.ScopeList, ListID: &list.ID}, f.storedScope(t))
}

// A Device that was never migrated ignores a scope segment in its URL.
func TestHandleSync_LegacyURLScopeIgnoredForNewDevices(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.setScope(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libB.ID})

	legacyA := f.legacyPrefix(fmt.Sprintf("library/%d", f.libA.ID))
	assert.Equal(t, []string{ShishoID(f.fileB.ID)}, f.syncedFileIDs(t, legacyA))
	assert.Equal(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libB.ID}, f.storedScope(t))
}

// Download URLs come back through the route the Kobo used, so a migrated
// Kobo keeps using its old URL and a new one never gets a scope segment.
func TestHandleSync_DownloadURLsFollowTheRouteUsed(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)

	for _, prefix := range []string{f.prefix(), f.legacyPrefix("all"), f.legacyPrefix(fmt.Sprintf("library/%d", f.libA.ID))} {
		f.resetSync(t)
		rec := f.serve(http.MethodGet, prefix+"/v1/library/sync")
		require.Equal(t, http.StatusOK, rec.Code)
		var items []syncItem
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &items))
		item := itemFor(t, items, f.fileA.ID)
		assertDownloadURLsUnder(t, item.NewEntitlement.BookMetadata["DownloadUrls"], "http://example.com"+prefix)
	}
}

// A Device without the Kobo sync connection cannot sync.
func TestKoboDeviceAuth_WithoutKoboSyncConnection_Returns403(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	_, err := devices.NewService(f.db).Update(context.Background(), f.user.ID, f.device.ID, devices.UpdateDeviceOptions{
		Connections: []string{devices.ConnectionEReaderBrowser}, SetConnections: true,
	})
	require.NoError(t, err)

	rec := f.serve(http.MethodGet, f.prefix()+"/v1/library/sync")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "This Device does not have Kobo sync turned on.")
}

// A migrated URL naming a library its owner cannot access synced nothing,
// and the Device keeps syncing nothing.
func TestHandleSync_MigratedURLForInaccessibleLibraryStaysClosed(t *testing.T) {
	t.Parallel()
	f := newKoboScopeFixture(t)
	f.restrictToLibrary(t, f.libA.ID)
	f.markMigrated(t)

	assert.Empty(t, f.syncedFileIDs(t, f.legacyPrefix(fmt.Sprintf("library/%d", f.libB.ID))))
	assert.Equal(t, devices.Scope{Type: devices.ScopeLibrary}, f.storedScope(t))
}
