package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_hide_chrome BOOLEAN NOT NULL DEFAULT FALSE
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			UPDATE user_settings SET viewer_fit_mode = 'fit-width' WHERE viewer_fit_mode = 'original'
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			UPDATE user_settings SET viewer_fit_mode = 'original' WHERE viewer_fit_mode = 'fit-width'
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("ALTER TABLE user_settings DROP COLUMN viewer_hide_chrome")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
