package devices

import (
	"context"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func intPtr(i int) *int { return &i }

func insertTestLibrary(t *testing.T, db *bun.DB, id int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (?, 'Library', 'book')`, id)
	require.NoError(t, err)
}

func TestService_Create_GetsEveryConnectionOfItsType(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()

	for deviceType, connections := range map[string][]string{
		DeviceTypeKobo:    {ConnectionKoboSync, ConnectionEReaderBrowser},
		DeviceTypeKindle:  {ConnectionEReaderBrowser},
		DeviceTypeOPDS:    {ConnectionOPDS},
		DeviceTypeBrowser: {ConnectionEReaderBrowser},
	} {
		created, err := service.Create(ctx, 1, "Device", deviceType)
		require.NoError(t, err)
		device, err := service.Retrieve(ctx, 1, created.ID)
		require.NoError(t, err)
		assert.Equal(t, deviceType, device.DeviceType)
		assert.Equal(t, connections, device.ConnectionNames(), deviceType)
		assert.Equal(t, ScopeAll, device.ScopeType)
		assert.Equal(t, created.Credential, device.Credential)
		assert.False(t, device.URLScopePending)
	}
}

func TestService_RegenerateCredential_RetiresTheOldOne(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	device, err := service.Create(ctx, 1, "Browser", DeviceTypeBrowser)
	require.NoError(t, err)
	shortURL, err := service.GenerateShortURL(ctx, 1, device.ID)
	require.NoError(t, err)

	regenerated, err := service.RegenerateCredential(ctx, 1, device.ID)
	require.NoError(t, err)
	assert.NotEqual(t, device.Credential, regenerated.Credential)
	assert.Len(t, regenerated.Credential, credentialLength)

	_, err = runDeviceAuth(t, service, ConnectionEReaderBrowser, device.Credential)
	requireStatus(t, err, http.StatusUnauthorized)
	seen, err := runDeviceAuth(t, service, ConnectionEReaderBrowser, regenerated.Credential)
	require.NoError(t, err)
	assert.Equal(t, device.ID, seen.ID)

	// A short URL made for the old credential would hand out the new one.
	resolved, err := service.ResolveShortCode(ctx, shortURL.ShortCode)
	require.NoError(t, err)
	assert.Nil(t, resolved)
}

// A migrated Kobo takes its scope from its old sync URL on its first
// request after upgrading. Later requests keep the Device's scope, whatever
// their URL says.
func TestService_ApplyURLScope_OnlyOnTheFirstRequest(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	insertTestLibrary(t, db, 1)
	insertTestLibrary(t, db, 2)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, name, device_type, credential, url_scope_pending, created_at, updated_at)
		VALUES ('migrated', 1, 'Kobo', 'kobo', 'ak_old', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	device, err := service.Retrieve(ctx, 1, "migrated")
	require.NoError(t, err)
	require.True(t, device.URLScopePending)

	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeLibrary, LibraryID: intPtr(1)})
	require.NoError(t, err)
	assert.Equal(t, Scope{Type: ScopeLibrary, LibraryID: intPtr(1)}, device.Scope())
	assert.False(t, device.URLScopePending)

	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeLibrary, LibraryID: intPtr(2)})
	require.NoError(t, err)
	assert.Equal(t, Scope{Type: ScopeLibrary, LibraryID: intPtr(1)}, device.Scope())

	// A stale copy that still says pending cannot overwrite it either.
	device.URLScopePending = true
	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeAll})
	require.NoError(t, err)
	assert.Equal(t, Scope{Type: ScopeLibrary, LibraryID: intPtr(1)}, device.Scope())
}

// A scope saved on the Device wins over the one in a migrated Kobo's old
// sync URL.
func TestService_Update_ScopeStopsTheURLScope(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	insertTestLibrary(t, db, 1)
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, name, device_type, credential, url_scope_pending, created_at, updated_at)
		VALUES ('migrated', 1, 'Kobo', 'kobo', 'ak_old', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	device, err := service.Update(ctx, 1, "migrated", UpdateDeviceOptions{Scope: &Scope{Type: ScopeAll}})
	require.NoError(t, err)
	assert.False(t, device.URLScopePending)

	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeLibrary, LibraryID: intPtr(1)})
	require.NoError(t, err)
	assert.Equal(t, ScopeAll, device.ScopeType)
}

func TestService_OtherUsersDevicesAreNotFound(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "owner")
	insertTestUser(t, db, 2, "other")
	ctx := context.Background()
	device, err := service.Create(ctx, 1, "Kobo", DeviceTypeKobo)
	require.NoError(t, err)

	_, err = service.Retrieve(ctx, 2, device.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = service.Update(ctx, 2, device.ID, UpdateDeviceOptions{Name: new(string)})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = service.RegenerateCredential(ctx, 2, device.ID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = service.GenerateShortURL(ctx, 2, device.ID)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, service.ClearKoboSyncHistory(ctx, 2, device.ID), ErrNotFound)
	require.ErrorIs(t, service.Delete(ctx, 2, device.ID), ErrNotFound)

	list, err := service.List(ctx, 2)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// A URL naming a library that no longer exists synced nothing, and the
// Device keeps syncing nothing rather than every library.
func TestService_ApplyURLScope_MissingLibraryStaysClosed(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, name, device_type, credential, url_scope_pending, created_at, updated_at)
		VALUES ('migrated', 1, 'Kobo', 'kobo', 'ak_old', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	device, err := service.Retrieve(ctx, 1, "migrated")
	require.NoError(t, err)

	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeLibrary, LibraryID: intPtr(99)})
	require.NoError(t, err)
	assert.Equal(t, Scope{Type: ScopeLibrary}, device.Scope())
	assert.False(t, device.URLScopePending)

	_, err = db.ExecContext(ctx, `UPDATE devices SET url_scope_pending = TRUE WHERE id = 'migrated'`)
	require.NoError(t, err)
	device.URLScopePending = true
	device, err = service.ApplyURLScope(ctx, device, Scope{Type: ScopeList, ListID: intPtr(99)})
	require.NoError(t, err)
	assert.Equal(t, Scope{Type: ScopeList}, device.Scope())
}

// Regenerating the credential retires the old sync URL, so its scope can no
// longer arrive and the Device's own scope applies.
func TestService_RegenerateCredential_StopsTheURLScope(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		INSERT INTO devices (id, user_id, name, device_type, credential, url_scope_pending, created_at, updated_at)
		VALUES ('migrated', 1, 'Kobo', 'kobo', 'ak_old', TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	_, err = service.RegenerateCredential(ctx, 1, "migrated")
	require.NoError(t, err)
	device, err := service.Retrieve(ctx, 1, "migrated")
	require.NoError(t, err)
	assert.False(t, device.URLScopePending)
}

// A Device keeps one setup URL at a time, so making a new one retires the
// last.
func TestService_GenerateShortURL_ReplacesThePreviousOne(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	service := NewService(db)
	insertTestUser(t, db, 1, "reader")
	ctx := context.Background()
	device, err := service.Create(ctx, 1, "Browser", DeviceTypeBrowser)
	require.NoError(t, err)

	first, err := service.GenerateShortURL(ctx, 1, device.ID)
	require.NoError(t, err)
	second, err := service.GenerateShortURL(ctx, 1, device.ID)
	require.NoError(t, err)

	resolved, err := service.ResolveShortCode(ctx, first.ShortCode)
	require.NoError(t, err)
	assert.Nil(t, resolved)
	resolved, err = service.ResolveShortCode(ctx, second.ShortCode)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, device.ID, resolved.ID)
}
