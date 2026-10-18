package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE kobo_sync_point_books ADD COLUMN cover_modified_at TIMESTAMPTZ`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE kobo_sync_point_books DROP COLUMN cover_modified_at`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
