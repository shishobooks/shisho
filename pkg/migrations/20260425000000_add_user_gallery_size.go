package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN gallery_size TEXT NOT NULL DEFAULT 'm'
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE user_settings DROP COLUMN gallery_size")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
