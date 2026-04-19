package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE user_library_settings (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id     INTEGER NOT NULL REFERENCES users     (id) ON DELETE CASCADE,
				library_id  INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
				sort_spec   TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_user_library_settings ON user_library_settings (user_id, library_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP INDEX IF EXISTS ux_user_library_settings")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS user_library_settings")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
