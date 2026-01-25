package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create plugin_repositories table
		_, err := db.Exec(`
			CREATE TABLE plugin_repositories (
				url TEXT PRIMARY KEY,
				scope TEXT NOT NULL UNIQUE,
				name TEXT,
				is_official BOOLEAN NOT NULL DEFAULT false,
				enabled BOOLEAN NOT NULL DEFAULT true,
				last_fetched_at TIMESTAMP,
				fetch_error TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create plugins table
		_, err = db.Exec(`
			CREATE TABLE plugins (
				scope TEXT NOT NULL,
				id TEXT NOT NULL,
				name TEXT NOT NULL,
				version TEXT NOT NULL,
				description TEXT,
				author TEXT,
				homepage TEXT,
				enabled BOOLEAN NOT NULL DEFAULT true,
				installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP,
				load_error TEXT,
				update_available_version TEXT,
				PRIMARY KEY (scope, id)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create plugin_configs table
		_, err = db.Exec(`
			CREATE TABLE plugin_configs (
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				key TEXT NOT NULL,
				value TEXT,
				PRIMARY KEY (scope, plugin_id, key),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create plugin_identifier_types table
		_, err = db.Exec(`
			CREATE TABLE plugin_identifier_types (
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

		// Create plugin_order table
		_, err = db.Exec(`
			CREATE TABLE plugin_order (
				hook_type TEXT NOT NULL,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				position INTEGER NOT NULL,
				PRIMARY KEY (hook_type, scope, plugin_id),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for plugin_identifier_types by plugin
		_, err = db.Exec(`CREATE INDEX ix_plugin_identifier_types_plugin ON plugin_identifier_types(scope, plugin_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for plugin_order by hook_type and position
		_, err = db.Exec(`CREATE INDEX ix_plugin_order_hook_position ON plugin_order(hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Seed official repository
		_, err = db.Exec(`
			INSERT INTO plugin_repositories (url, scope, name, is_official, enabled)
			VALUES ('https://raw.githubusercontent.com/shishobooks/plugins/master/repository.json', 'shisho', 'Official Shisho Plugins', true, true)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_plugin_order_hook_position`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_plugin_identifier_types_plugin`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_identifier_types`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_configs`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugins`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_repositories`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
