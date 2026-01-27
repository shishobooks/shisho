package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create plugin_field_settings table for global field enable/disable settings
		_, err := db.Exec(`
			CREATE TABLE plugin_field_settings (
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				field TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT true,
				PRIMARY KEY (scope, plugin_id, field),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create library_plugin_field_settings table for per-library overrides
		_, err = db.Exec(`
			CREATE TABLE library_plugin_field_settings (
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				field TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT true,
				PRIMARY KEY (library_id, scope, plugin_id, field),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for querying field settings by plugin
		_, err = db.Exec(`CREATE INDEX idx_plugin_field_settings_plugin ON plugin_field_settings(scope, plugin_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for querying library field settings by library and plugin
		_, err = db.Exec(`CREATE INDEX idx_library_plugin_field_settings_library ON library_plugin_field_settings(library_id, scope, plugin_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_library_plugin_field_settings_library`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP INDEX IF EXISTS idx_plugin_field_settings_plugin`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS library_plugin_field_settings`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS plugin_field_settings`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
