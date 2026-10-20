package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const addKoboSyncEntryIDsMigrationName = "20261022000000"

// Entries already on a Kobo keep their shisho-{fileID} ids and RevisionIds
// as KePub entries, and snapshots record the entry each File went out under.
// Deleting a File leaves its entry for the next sync to hand over, and the
// migration rolls back.
func TestAddKoboSyncEntryIDs_KeepsEntriesAndRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, addKoboSyncEntryIDsMigrationName)

	for _, query := range []string{
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (1, 'u', 'x', 1, 1)`,
		`INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at)
			VALUES ('d1', 1, 'Kobo', 'kobo', 'cred', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (7, 1, '/one/book', 'Book', 'filepath', 'Book', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 7, '/one/book/a.epub', 'epub', 'main'),
			       (2, 1, 7, '/one/book/b.cbz', 'cbz', 'main')`,
		`INSERT INTO kobo_sync_entries (device_id, file_id, revision_id) VALUES ('d1', 1, 'rev-1'), ('d1', 2, 'rev-2')`,
		`INSERT INTO kobo_sync_points (id, device_id, created_at, completed_at) VALUES ('sp', 'd1', '2026-01-01', '2026-01-01')`,
		`INSERT INTO kobo_sync_point_books (id, sync_point_id, file_id, file_hash, file_size, metadata_hash)
			VALUES ('b1', 'sp', 1, 'h', 1, 'm'), ('b2', 'sp', 3, 'h', 1, 'm')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	type entry struct {
		EntryID           string `bun:"entry_id"`
		FileID            *int   `bun:"file_id"`
		BookID            *int   `bun:"book_id"`
		DeliveredFileType string `bun:"delivered_file_type"`
		RevisionID        string `bun:"revision_id"`
	}
	var entries []entry
	require.NoError(t, db.NewRaw(`SELECT entry_id, file_id, book_id, delivered_file_type, revision_id FROM kobo_sync_entries ORDER BY entry_id`).Scan(ctx, &entries))
	one, two, seven := 1, 2, 7
	assert.Equal(t, []entry{
		{EntryID: "shisho-1", FileID: &one, BookID: &seven, DeliveredFileType: "kepub", RevisionID: "rev-1"},
		{EntryID: "shisho-2", FileID: &two, BookID: &seven, DeliveredFileType: "kepub", RevisionID: "rev-2"},
	}, entries)

	type snapshotRow struct {
		FileID            int    `bun:"file_id"`
		EntryID           string `bun:"entry_id"`
		DeliveredFileType string `bun:"delivered_file_type"`
		RevisionID        string `bun:"revision_id"`
	}
	var snapshot []snapshotRow
	require.NoError(t, db.NewRaw(`SELECT file_id, entry_id, delivered_file_type, revision_id FROM kobo_sync_point_books ORDER BY file_id`).Scan(ctx, &snapshot))
	assert.Equal(t, []snapshotRow{
		{FileID: 1, EntryID: "shisho-1", DeliveredFileType: "kepub", RevisionID: "rev-1"},
		// File 3 is gone and has no entry; the snapshot still names the
		// entry it went out under, so the next sync can remove it.
		{FileID: 3, EntryID: "shisho-3", DeliveredFileType: "kepub", RevisionID: ""},
	}, snapshot)

	_, err = db.ExecContext(ctx, `DELETE FROM files WHERE id = 2`)
	require.NoError(t, err)
	var orphan entry
	require.NoError(t, db.NewRaw(`SELECT entry_id, file_id, book_id, delivered_file_type, revision_id FROM kobo_sync_entries WHERE entry_id = 'shisho-2'`).Scan(ctx, &orphan))
	assert.Nil(t, orphan.FileID, "a deleted File's entry stays for the next sync to hand over")
	assert.Equal(t, &seven, orphan.BookID)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"device_id", "file_id", "revision_id", "created_at"}, columnNames(ctx, t, db, "kobo_sync_entries"))
	var count int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM kobo_sync_entries`).Scan(ctx, &count))
	assert.Equal(t, 1, count, "only the entry holding a File fits the old table")
	assert.NotContains(t, columnNames(ctx, t, db, "kobo_sync_point_books"), "entry_id")

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
