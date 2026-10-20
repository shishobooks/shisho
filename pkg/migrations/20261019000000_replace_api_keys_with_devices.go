package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// API keys become Devices (ADR 0011). Each key keeps its id, and its key
// becomes the Device's credential, so saved Kobo sync URLs, eReader browser
// bookmarks, and Kobo sync state keep working. A key with kobo_sync becomes a
// Kobo Device and any other key a Browser Device. A Device's type decides how
// it connects, so permissions are not kept: every Kobo Device can also use the
// eReader browser, and a key with no permissions becomes a working Browser
// Device. A migrated Kobo takes its scope from the scope
// segment of its sync URL on its first request, so url_scope_pending marks
// the Devices still waiting for one.
//
// Kobo sync state cascades from its parent, so its tables are rebuilt with
// foreign keys off to point them at devices without deleting any rows.

var deviceSyncStateRebuilds = []struct {
	table, oldColumn, newColumn, toSQL, fromSQL string
}{
	{
		table:     "kobo_sync_points",
		oldColumn: "api_key_id",
		newColumn: "device_id",
		toSQL: `CREATE TABLE kobo_sync_points_new (
			id TEXT PRIMARY KEY,
			device_id TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL,
			completed_at DATETIME
		)`,
		fromSQL: `CREATE TABLE kobo_sync_points_new (
			id TEXT PRIMARY KEY,
			api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
			created_at DATETIME NOT NULL,
			completed_at DATETIME
		)`,
	},
	{
		table:     "kobo_sync_entries",
		oldColumn: "api_key_id",
		newColumn: "device_id",
		toSQL: `CREATE TABLE kobo_sync_entries_new (
			device_id TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
			file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
			revision_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (device_id, file_id)
		)`,
		fromSQL: `CREATE TABLE kobo_sync_entries_new (
			api_key_id TEXT NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
			file_id INTEGER NOT NULL REFERENCES files (id) ON DELETE CASCADE,
			revision_id TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (api_key_id, file_id)
		)`,
	},
}

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			for _, stmt := range []string{
				`CREATE TABLE devices (
					id TEXT PRIMARY KEY,
					user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					device_type TEXT NOT NULL,
					credential TEXT NOT NULL UNIQUE,
					scope_type TEXT NOT NULL DEFAULT 'all',
					scope_library_id INTEGER REFERENCES libraries (id) ON DELETE SET NULL,
					scope_list_id INTEGER REFERENCES lists (id) ON DELETE SET NULL,
					url_scope_pending BOOLEAN NOT NULL DEFAULT FALSE,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					last_accessed_at TIMESTAMPTZ
				)`,
				`CREATE INDEX ix_devices_user_id ON devices (user_id)`,
				`CREATE INDEX ix_devices_scope_library_id ON devices (scope_library_id)`,
				`CREATE INDEX ix_devices_scope_list_id ON devices (scope_list_id)`,
				`CREATE TABLE device_short_urls (
					id TEXT PRIMARY KEY,
					device_id TEXT NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
					short_code TEXT NOT NULL UNIQUE,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX ix_device_short_urls_device_id ON device_short_urls (device_id)`,
				`INSERT INTO devices (id, user_id, name, device_type, credential, url_scope_pending, created_at, updated_at, last_accessed_at)
					SELECT k.id, k.user_id, k.name,
						CASE WHEN kobo.api_key_id IS NULL THEN 'browser' ELSE 'kobo' END,
						k.key, kobo.api_key_id IS NOT NULL, k.created_at, k.updated_at, k.last_accessed_at
					FROM api_keys k
					LEFT JOIN api_key_permissions kobo ON kobo.api_key_id = k.id AND kobo.permission = 'kobo_sync'`,
				`INSERT INTO device_short_urls (id, device_id, short_code, expires_at, created_at)
					SELECT id, api_key_id, short_code, expires_at, created_at FROM api_key_short_urls`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}

			for _, r := range deviceSyncStateRebuilds {
				if err := renameColumnAndRebuild(ctx, tx, r.table, r.oldColumn, r.newColumn, r.toSQL); err != nil {
					return err
				}
			}

			for _, stmt := range []string{
				`DROP INDEX idx_kobo_sync_points_api_key`,
				`CREATE INDEX ix_kobo_sync_points_device_id ON kobo_sync_points (device_id)`,
				`DROP TABLE api_key_short_urls`,
				`DROP TABLE api_key_permissions`,
				`DROP TABLE api_keys`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}

			return checkForeignKeys(ctx, tx, []string{"devices", "device_short_urls", "kobo_sync_points", "kobo_sync_entries"})
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			for _, stmt := range []string{
				`CREATE TABLE api_keys (
					id TEXT PRIMARY KEY,
					user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					key TEXT NOT NULL UNIQUE,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					last_accessed_at DATETIME
				)`,
				`CREATE INDEX idx_api_keys_user_id ON api_keys(user_id)`,
				`CREATE INDEX idx_api_keys_key ON api_keys(key)`,
				`CREATE TABLE api_key_permissions (
					id TEXT PRIMARY KEY,
					api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
					permission TEXT NOT NULL,
					created_at DATETIME NOT NULL,
					UNIQUE(api_key_id, permission)
				)`,
				`CREATE INDEX idx_api_key_permissions_api_key_id ON api_key_permissions(api_key_id)`,
				`CREATE TABLE api_key_short_urls (
					id TEXT PRIMARY KEY,
					api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
					short_code TEXT NOT NULL UNIQUE,
					expires_at DATETIME NOT NULL,
					created_at DATETIME NOT NULL
				)`,
				`CREATE INDEX idx_short_urls_code ON api_key_short_urls(short_code)`,
				`CREATE INDEX ix_api_key_short_urls_api_key_id ON api_key_short_urls (api_key_id)`,
				`INSERT INTO api_keys (id, user_id, name, key, created_at, updated_at, last_accessed_at)
					SELECT id, user_id, name, credential, created_at, updated_at, last_accessed_at FROM devices`,
				`INSERT INTO api_key_permissions (id, api_key_id, permission, created_at)
					SELECT lower(hex(randomblob(16))), id, 'ereader_browser', created_at FROM devices`,
				`INSERT INTO api_key_permissions (id, api_key_id, permission, created_at)
					SELECT lower(hex(randomblob(16))), id, 'kobo_sync', created_at FROM devices WHERE device_type = 'kobo'`,
				`INSERT INTO api_key_short_urls (id, api_key_id, short_code, expires_at, created_at)
					SELECT id, device_id, short_code, expires_at, created_at FROM device_short_urls`,
				`DROP INDEX ix_kobo_sync_points_device_id`,
				`CREATE INDEX idx_kobo_sync_points_api_key ON kobo_sync_points (device_id)`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}

			for _, r := range deviceSyncStateRebuilds {
				if err := renameColumnAndRebuild(ctx, tx, r.table, r.newColumn, r.oldColumn, r.fromSQL); err != nil {
					return err
				}
			}

			for _, stmt := range []string{
				`DROP TABLE device_short_urls`,
				`DROP TABLE devices`,
			} {
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return errors.Wrap(err, stmt)
				}
			}

			return checkForeignKeys(ctx, tx, []string{"api_keys", "api_key_permissions", "api_key_short_urls", "kobo_sync_points", "kobo_sync_entries"})
		})
	}

	Migrations.MustRegister(up, down)
}

// renameColumnAndRebuild renames a column, then rebuilds the table with
// createSQL, which uses the new name and the new foreign key target.
// rebuildTableInTx copies by column name, and the rename updates the column
// in the table's indexes, so they are recreated on the new column.
func renameColumnAndRebuild(ctx context.Context, tx bun.Tx, table, oldColumn, newColumn, createSQL string) error {
	if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" RENAME COLUMN "+oldColumn+" TO "+newColumn); err != nil {
		return errors.Wrapf(err, "rename %s.%s", table, oldColumn)
	}
	return rebuildTableInTx(ctx, tx, table, createSQL)
}
