package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Clean up orphaned rows in all plugin child tables.
		// Foreign keys were not enforced prior to this release, so uninstalling
		// plugins left behind orphaned rows that would now violate FK constraints.
		orphanCleanup := []string{
			`DELETE FROM plugin_configs WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			`DELETE FROM plugin_identifier_types WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			`DELETE FROM plugin_hook_config WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			`DELETE FROM library_plugin_hook_config WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			`DELETE FROM plugin_field_settings WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
			`DELETE FROM library_plugin_field_settings WHERE (scope, plugin_id) NOT IN (SELECT scope, id FROM plugins)`,
		}
		for _, q := range orphanCleanup {
			if _, err := db.Exec(q); err != nil {
				return errors.WithStack(err)
			}
		}

		// Recreate plugin_identifier_types with composite PK (id, scope, plugin_id)
		// instead of just (id). This allows multiple plugins to register the same
		// identifier type (e.g., local and published versions of the same plugin).
		_, err := db.Exec(`
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
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			INSERT INTO plugin_identifier_types_new (id, scope, plugin_id, name, url_template, pattern)
			SELECT id, scope, plugin_id, name, url_template, pattern
			FROM plugin_identifier_types
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`DROP TABLE plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE plugin_identifier_types_new RENAME TO plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_plugin_identifier_types_plugin ON plugin_identifier_types(scope, plugin_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename plugin_hook_config -> plugin_hook_configs for naming consistency
		// (all other tables use plural names).
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_plugin_order_hook_position`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE plugin_hook_config RENAME TO plugin_hook_configs`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_plugin_hook_configs_hook_position ON plugin_hook_configs(hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename library_plugin_hook_config -> library_plugin_hook_configs
		_, err = db.Exec(`DROP INDEX IF EXISTS idx_library_plugin_hook_config_order`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config RENAME TO library_plugin_hook_configs`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_library_plugin_hook_configs_order ON library_plugin_hook_configs(library_id, hook_type, position)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Rename library_plugin_hook_configs back to library_plugin_hook_config
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_library_plugin_hook_configs_order`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE library_plugin_hook_configs RENAME TO library_plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_library_plugin_hook_config_order ON library_plugin_hook_config(library_id, hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename plugin_hook_configs back to plugin_hook_config
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_plugin_hook_configs_hook_position`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE plugin_hook_configs RENAME TO plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_plugin_order_hook_position ON plugin_hook_config(hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recreate plugin_identifier_types with single-column PK
		_, err = db.Exec(`
			CREATE TABLE plugin_identifier_types_old (
				id TEXT PRIMARY KEY,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				name TEXT NOT NULL,
				url_template TEXT,
				pattern TEXT,
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			INSERT INTO plugin_identifier_types_old (id, scope, plugin_id, name, url_template, pattern)
			SELECT id, scope, plugin_id, name, url_template, pattern
			FROM plugin_identifier_types
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`DROP TABLE plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`ALTER TABLE plugin_identifier_types_old RENAME TO plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_plugin_identifier_types_plugin ON plugin_identifier_types(scope, plugin_id)`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
