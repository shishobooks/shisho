package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE files ADD COLUMN is_preferred_cover BOOLEAN NOT NULL DEFAULT FALSE`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		// SQLite cannot drop a column before 3.35.0; rebuild if needed.
		// Since the app targets recent SQLite, ALTER TABLE DROP COLUMN works.
		_, err := db.Exec(`ALTER TABLE files DROP COLUMN is_preferred_cover`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
