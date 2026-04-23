package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE libraries DROP COLUMN deleted_at`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE libraries ADD COLUMN deleted_at TIMESTAMPTZ`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
