package kobo

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// ShishoID returns the id an entry recorded before entries had ids of
// their own goes by: its File's Shisho id.
func ShishoID(fileID int) string {
	return fmt.Sprintf("shisho-%d", fileID)
}

// testDeviceID is the Device newSyncPointTestDB adds, which withDeviceOwner
// stores in a request.
const testDeviceID = "api-key-1"

// insertEntry records an entry on deviceID for fileID, delivered as
// deliveredType, under the File's Shisho id as entries from before the
// upgrade are, and returns that id.
func insertEntry(t *testing.T, db *bun.DB, deviceID string, fileID int, deliveredType string) string {
	t.Helper()
	id := ShishoID(fileID)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO kobo_sync_entries (device_id, entry_id, file_id, book_id, delivered_file_type, revision_id)
			VALUES (?, ?, ?, (SELECT book_id FROM files WHERE id = ?), ?, 'rev-' || ?)`,
		deviceID, id, fileID, fileID, deliveredType, id)
	require.NoError(t, err)
	return id
}

// entryIDOf returns the id of the entry that holds fileID on deviceID.
func entryIDOf(t *testing.T, db *bun.DB, deviceID string, fileID int) string {
	t.Helper()
	var id string
	require.NoError(t, db.NewRaw(`SELECT entry_id FROM kobo_sync_entries WHERE device_id = ? AND file_id = ?`, deviceID, fileID).Scan(context.Background(), &id))
	return id
}

// entryID returns the id of the entry that holds fileID on the fixture's
// Device.
func (f *koboScopeFixture) entryID(t *testing.T, fileID int) string {
	t.Helper()
	return entryIDOf(t, f.db, f.device.ID, fileID)
}
