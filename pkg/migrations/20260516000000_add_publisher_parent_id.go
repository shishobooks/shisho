package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE publishers ADD COLUMN parent_id INTEGER REFERENCES publishers(id) ON DELETE SET NULL`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_publishers_parent_id ON publishers(parent_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_publishers_parent_id`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE publishers DROP COLUMN parent_id`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
