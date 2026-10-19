package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const replaceAPIKeysWithDevicesMigrationName = "20261019000000"

// Each API key becomes a Device with the same id and with its key as the
// credential, so saved Kobo and eReader URLs keep working. Its short URLs and
// Kobo sync state move with it.
func TestReplaceAPIKeysWithDevices_KeysBecomeDevices(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, replaceAPIKeysWithDevicesMigrationName)

	for _, query := range []string{
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (1, 'u', 'x', 1, 1)`,
		`INSERT INTO api_keys (id, user_id, name, key, created_at, updated_at, last_accessed_at)
			VALUES ('kobo', 1, 'Kobo only', 'ak_kobo', '2026-01-01 00:00:00', '2026-01-02 00:00:00', '2026-01-03 00:00:00'),
			       ('both', 1, 'Kobo and browser', 'ak_both', '2026-01-01 00:00:00', '2026-01-01 00:00:00', NULL),
			       ('browser', 1, 'Browser only', 'ak_browser', '2026-01-01 00:00:00', '2026-01-01 00:00:00', NULL),
			       ('none', 1, 'No permissions', 'ak_none', '2026-01-01 00:00:00', '2026-01-01 00:00:00', NULL)`,
		`INSERT INTO api_key_permissions (id, api_key_id, permission, created_at)
			VALUES ('p1', 'kobo', 'kobo_sync', CURRENT_TIMESTAMP),
			       ('p2', 'both', 'kobo_sync', CURRENT_TIMESTAMP),
			       ('p3', 'both', 'ereader_browser', CURRENT_TIMESTAMP),
			       ('p4', 'browser', 'ereader_browser', CURRENT_TIMESTAMP)`,
		`INSERT INTO api_key_short_urls (id, api_key_id, short_code, expires_at, created_at)
			VALUES ('s1', 'browser', 'abc123', '2099-01-01 00:00:00', CURRENT_TIMESTAMP)`,
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/book', 'Book', 'filepath', 'Book', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, file_role)
			VALUES (1, 1, 1, '/one/book/a.epub', 'epub', 'main')`,
		`INSERT INTO kobo_sync_points (id, api_key_id, created_at, completed_at)
			VALUES ('sp1', 'kobo', '2026-02-01 00:00:00', '2026-02-01 00:00:00')`,
		`INSERT INTO kobo_sync_point_books (id, sync_point_id, file_id, file_hash, file_size, metadata_hash, synced)
			VALUES ('b1', 'sp1', 1, 'hash', 10, 'meta', 1)`,
		`INSERT INTO kobo_sync_entries (api_key_id, file_id, revision_id) VALUES ('kobo', 1, 'shisho-1-hash')`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	type device struct {
		ID              string  `bun:"id"`
		UserID          int     `bun:"user_id"`
		Name            string  `bun:"name"`
		DeviceType      string  `bun:"device_type"`
		Credential      string  `bun:"credential"`
		ScopeType       string  `bun:"scope_type"`
		URLScopePending bool    `bun:"url_scope_pending"`
		LastAccessedAt  *string `bun:"last_accessed_at"`
	}
	var devices []device
	require.NoError(t, db.NewRaw(`SELECT id, user_id, name, device_type, credential, scope_type, url_scope_pending, last_accessed_at FROM devices ORDER BY id`).Scan(ctx, &devices))
	lastAccessed := "2026-01-03 00:00:00"
	assert.Equal(t, []device{
		{ID: "both", UserID: 1, Name: "Kobo and browser", DeviceType: "kobo", Credential: "ak_both", ScopeType: "all", URLScopePending: true},
		{ID: "browser", UserID: 1, Name: "Browser only", DeviceType: "browser", Credential: "ak_browser", ScopeType: "all"},
		{ID: "kobo", UserID: 1, Name: "Kobo only", DeviceType: "kobo", Credential: "ak_kobo", ScopeType: "all", URLScopePending: true, LastAccessedAt: &lastAccessed},
		{ID: "none", UserID: 1, Name: "No permissions", DeviceType: "browser", Credential: "ak_none", ScopeType: "all"},
	}, devices)

	var connections []string
	require.NoError(t, db.NewRaw(`SELECT device_id || '/' || connection FROM device_connections ORDER BY device_id, connection`).Scan(ctx, &connections))
	assert.Equal(t, []string{"both/ereader_browser", "both/kobo_sync", "browser/ereader_browser", "kobo/kobo_sync"}, connections)

	var shortURLs []string
	require.NoError(t, db.NewRaw(`SELECT device_id || '/' || short_code FROM device_short_urls`).Scan(ctx, &shortURLs))
	assert.Equal(t, []string{"browser/abc123"}, shortURLs)

	var syncPoints []string
	require.NoError(t, db.NewRaw(`SELECT device_id || '/' || id FROM kobo_sync_points`).Scan(ctx, &syncPoints))
	assert.Equal(t, []string{"kobo/sp1"}, syncPoints)
	var syncPointBooks int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM kobo_sync_point_books`).Scan(ctx, &syncPointBooks))
	assert.Equal(t, 1, syncPointBooks)
	var entries []string
	require.NoError(t, db.NewRaw(`SELECT device_id || '/' || file_id || '/' || revision_id FROM kobo_sync_entries`).Scan(ctx, &entries))
	assert.Equal(t, []string{"kobo/1/shisho-1-hash"}, entries)

	for _, table := range []string{"api_keys", "api_key_permissions", "api_key_short_urls"} {
		assert.Empty(t, columnNames(ctx, t, db, table), table)
	}

	// Deleting a Device cascades into its sync state, which now hangs off
	// devices instead of api_keys.
	_, err = db.ExecContext(ctx, `DELETE FROM devices WHERE id = 'kobo'`)
	require.NoError(t, err)
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM kobo_sync_point_books`).Scan(ctx, &syncPointBooks))
	assert.Equal(t, 0, syncPointBooks)
	var entryCount int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM kobo_sync_entries`).Scan(ctx, &entryCount))
	assert.Equal(t, 0, entryCount)
}

// Rolling back turns each Device back into an API key with the permissions
// its connections stand for, keeping its short URLs and Kobo sync state.
func TestReplaceAPIKeysWithDevices_RollbackRestoresAPIKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, replaceAPIKeysWithDevicesMigrationName)

	for _, query := range []string{
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (1, 'u', 'x', 1, 1)`,
		`INSERT INTO api_keys (id, user_id, name, key, created_at, updated_at)
			VALUES ('both', 1, 'Kobo and browser', 'ak_both', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		`INSERT INTO api_key_permissions (id, api_key_id, permission, created_at)
			VALUES ('p2', 'both', 'kobo_sync', CURRENT_TIMESTAMP),
			       ('p3', 'both', 'ereader_browser', CURRENT_TIMESTAMP)`,
		`INSERT INTO api_key_short_urls (id, api_key_id, short_code, expires_at, created_at)
			VALUES ('s1', 'both', 'abc123', '2099-01-01 00:00:00', CURRENT_TIMESTAMP)`,
		`INSERT INTO kobo_sync_points (id, api_key_id, created_at) VALUES ('sp1', 'both', CURRENT_TIMESTAMP)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)

	var keys []string
	require.NoError(t, db.NewRaw(`SELECT id || '/' || key FROM api_keys`).Scan(ctx, &keys))
	assert.Equal(t, []string{"both/ak_both"}, keys)
	var permissions []string
	require.NoError(t, db.NewRaw(`SELECT permission FROM api_key_permissions WHERE api_key_id = 'both' ORDER BY permission`).Scan(ctx, &permissions))
	assert.Equal(t, []string{"ereader_browser", "kobo_sync"}, permissions)
	var shortURLs []string
	require.NoError(t, db.NewRaw(`SELECT api_key_id || '/' || short_code FROM api_key_short_urls`).Scan(ctx, &shortURLs))
	assert.Equal(t, []string{"both/abc123"}, shortURLs)
	var syncPoints []string
	require.NoError(t, db.NewRaw(`SELECT api_key_id || '/' || id FROM kobo_sync_points`).Scan(ctx, &syncPoints))
	assert.Equal(t, []string{"both/sp1"}, syncPoints)
	assert.Empty(t, columnNames(ctx, t, db, "devices"))

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
