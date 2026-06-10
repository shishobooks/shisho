package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			ALTER TABLE user_settings ADD COLUMN viewer_playback_speed REAL NOT NULL DEFAULT 1.0
		`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("ALTER TABLE user_settings DROP COLUMN viewer_playback_speed")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
