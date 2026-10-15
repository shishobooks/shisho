package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN default_library_id INTEGER REFERENCES libraries (id) ON DELETE SET NULL
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_user_settings_default_library_id ON user_settings (default_library_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP INDEX IF EXISTS ix_user_settings_default_library_id")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("ALTER TABLE user_settings DROP COLUMN default_library_id")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
