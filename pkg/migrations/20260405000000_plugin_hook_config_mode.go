package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// 1. Rename plugin_order -> plugin_hook_config
		_, err := db.Exec(`ALTER TABLE plugin_order RENAME TO plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 2. Add mode column to plugin_hook_config
		_, err = db.Exec(`ALTER TABLE plugin_hook_config ADD COLUMN mode TEXT NOT NULL DEFAULT 'enabled'`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 3. Rename library_plugins -> library_plugin_hook_config
		_, err = db.Exec(`ALTER TABLE library_plugins RENAME TO library_plugin_hook_config`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 4. Add mode column to library_plugin_hook_config
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config ADD COLUMN mode TEXT NOT NULL DEFAULT 'enabled'`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 5. Migrate enabled -> mode
		_, err = db.Exec(`UPDATE library_plugin_hook_config SET mode = CASE WHEN enabled THEN 'enabled' ELSE 'disabled' END`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 6. Drop enabled column
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config DROP COLUMN enabled`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 7. Drop old index, create new one
		_, err = db.Exec(`DROP INDEX IF EXISTS idx_library_plugins_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX idx_library_plugin_hook_config_order ON library_plugin_hook_config(library_id, hook_type, position)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Reverse: drop new index
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_library_plugin_hook_config_order`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Add enabled column back
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Migrate mode -> enabled
		_, err = db.Exec(`UPDATE library_plugin_hook_config SET enabled = CASE WHEN mode = 'enabled' THEN true ELSE false END`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Drop mode column
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config DROP COLUMN mode`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename library_plugin_hook_config -> library_plugins
		_, err = db.Exec(`ALTER TABLE library_plugin_hook_config RENAME TO library_plugins`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recreate old index
		_, err = db.Exec(`CREATE INDEX idx_library_plugins_order ON library_plugins(library_id, hook_type, position)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Drop mode column from plugin_hook_config
		_, err = db.Exec(`ALTER TABLE plugin_hook_config DROP COLUMN mode`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename plugin_hook_config -> plugin_order
		_, err = db.Exec(`ALTER TABLE plugin_hook_config RENAME TO plugin_order`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
