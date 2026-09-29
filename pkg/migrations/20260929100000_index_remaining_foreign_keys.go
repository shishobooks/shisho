package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// Index the foreign keys that 20260928100000 left out, so every foreign key
// has an index whose leading columns are the key. Without one, the ON DELETE
// action on each library, API key, or plugin delete scans the child table:
// the alias tables' unique index leads with name, and the plugin tables'
// primary keys lead with library_id or hook_type rather than (scope, plugin_id).
func init() {
	indexes := []struct{ name, table, columns string }{
		{"ix_api_key_short_urls_api_key_id", "api_key_short_urls", "api_key_id"},
		{"ix_person_aliases_library_id", "person_aliases", "library_id"},
		{"ix_series_aliases_library_id", "series_aliases", "library_id"},
		{"ix_genre_aliases_library_id", "genre_aliases", "library_id"},
		{"ix_tag_aliases_library_id", "tag_aliases", "library_id"},
		{"ix_publisher_aliases_library_id", "publisher_aliases", "library_id"},
		{"ix_library_plugin_field_settings_scope_plugin_id", "library_plugin_field_settings", "scope, plugin_id"},
		{"ix_library_plugin_hook_configs_scope_plugin_id", "library_plugin_hook_configs", "scope, plugin_id"},
		{"ix_plugin_hook_configs_scope_plugin_id", "plugin_hook_configs", "scope, plugin_id"},
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, ix := range indexes {
				query := "CREATE INDEX " + ix.name + " ON " + ix.table + " (" + ix.columns + ")"
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrapf(err, "failed to index %s (%s)", ix.table, ix.columns)
				}
			}
			return nil
		})
	}
	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, ix := range indexes {
				if _, err := tx.ExecContext(ctx, "DROP INDEX IF EXISTS "+ix.name); err != nil {
					return errors.Wrapf(err, "failed to drop index %s", ix.name)
				}
			}
			return nil
		})
	}
	Migrations.MustRegister(up, down)
}
