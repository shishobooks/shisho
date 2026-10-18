package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// Clean up orphaned rows in all plugin child tables.
			// Foreign keys were not enforced prior to this release, so uninstalling
			// plugins left behind orphaned rows that would now violate FK constraints.
			for _, q := range []string{
				`DELETE FROM plugin_configs WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
				`DELETE FROM plugin_identifier_types WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
				`DELETE FROM plugin_hook_config WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
				`DELETE FROM library_plugin_hook_config WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
				`DELETE FROM plugin_field_settings WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
				`DELETE FROM library_plugin_field_settings WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}

			// Recreate plugin_identifier_types with composite PK (id, scope, plugin_id)
			// instead of just (id). This allows multiple plugins to register the same
			// identifier type (e.g., local and published versions of the same plugin).
			if err := rebuildTableInTx(ctx, tx, "plugin_identifier_types", `
				CREATE TABLE plugin_identifier_types_new (
					id TEXT NOT NULL,
					scope TEXT NOT NULL,
					plugin_id TEXT NOT NULL,
					name TEXT NOT NULL,
					url_template TEXT,
					pattern TEXT,
					PRIMARY KEY (id, scope, plugin_id),
					FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
				)
			`); err != nil {
				return err
			}

			for _, q := range []string{
				// Rename plugin_hook_config -> plugin_hook_configs for naming consistency
				// (all other tables use plural names).
				`DROP INDEX IF EXISTS ix_plugin_order_hook_position`,
				`ALTER TABLE plugin_hook_config RENAME TO plugin_hook_configs`,
				`CREATE INDEX ix_plugin_hook_configs_hook_position ON plugin_hook_configs(hook_type, position)`,
				// Rename library_plugin_hook_config -> library_plugin_hook_configs
				`DROP INDEX IF EXISTS idx_library_plugin_hook_config_order`,
				`ALTER TABLE library_plugin_hook_config RENAME TO library_plugin_hook_configs`,
				`CREATE INDEX ix_library_plugin_hook_configs_order ON library_plugin_hook_configs(library_id, hook_type, position)`,
				// Drop unused book_identifiers table. Identifiers are stored at the
				// file level (file_identifiers) and this table was never populated.
				// Its non-cascading FK to books would block book deletion.
				`DROP TABLE IF EXISTS book_identifiers`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}

			return checkForeignKeys(ctx, tx, []string{"plugin_identifier_types"})
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		// plugin_identifier_types keeps its composite primary key. Restoring the
		// single-column key would fail once two plugins share an identifier
		// type id, and the up rebuilds the table the same way either way.
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				// Rename library_plugin_hook_configs back to library_plugin_hook_config
				`DROP INDEX IF EXISTS ix_library_plugin_hook_configs_order`,
				`ALTER TABLE library_plugin_hook_configs RENAME TO library_plugin_hook_config`,
				`CREATE INDEX idx_library_plugin_hook_config_order ON library_plugin_hook_config(library_id, hook_type, position)`,
				// Rename plugin_hook_configs back to plugin_hook_config
				`DROP INDEX IF EXISTS ix_plugin_hook_configs_hook_position`,
				`ALTER TABLE plugin_hook_configs RENAME TO plugin_hook_config`,
				`CREATE INDEX ix_plugin_order_hook_position ON plugin_hook_config(hook_type, position)`,
				// Recreate book_identifiers table
				`CREATE TABLE book_identifiers (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					book_id TEXT REFERENCES books (id) NOT NULL,
					type TEXT NOT NULL,
					value TEXT NOT NULL
				)`,
				`CREATE INDEX ix_book_identifiers_book_id ON book_identifiers (book_id)`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}
