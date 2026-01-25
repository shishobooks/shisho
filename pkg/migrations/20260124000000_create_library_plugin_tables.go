package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE library_plugin_customizations (
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				hook_type TEXT NOT NULL,
				PRIMARY KEY (library_id, hook_type)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`
			CREATE TABLE library_plugins (
				library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				scope TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				hook_type TEXT NOT NULL,
				enabled BOOLEAN NOT NULL DEFAULT true,
				position INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY (library_id, hook_type, scope, plugin_id),
				FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX idx_library_plugins_order ON library_plugins(library_id, hook_type, position)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS idx_library_plugins_order`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS library_plugins`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS library_plugin_customizations`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
