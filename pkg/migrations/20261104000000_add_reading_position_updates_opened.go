package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Marks an update that saved a place a link opened, which isn't
		// reading, so it doesn't count toward when a Book was last read.
		_, err := db.Exec(`ALTER TABLE reading_position_updates ADD COLUMN opened BOOLEAN NOT NULL DEFAULT FALSE`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE reading_position_updates DROP COLUMN opened`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
