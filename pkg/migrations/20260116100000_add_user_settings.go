package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE user_settings (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
				viewer_preload_count INTEGER NOT NULL DEFAULT 3,
				viewer_fit_mode TEXT NOT NULL DEFAULT 'fit-height'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for fast lookup by user
		_, err = db.Exec(`CREATE INDEX ix_user_settings_user_id ON user_settings(user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS user_settings")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
