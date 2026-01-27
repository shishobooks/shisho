package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Rename cover_image_path to cover_image_filename in files table
		_, err := db.Exec("ALTER TABLE files RENAME COLUMN cover_image_path TO cover_image_filename")
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename cover_image_path to cover_image_filename in series table
		_, err = db.Exec("ALTER TABLE series RENAME COLUMN cover_image_path TO cover_image_filename")
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Rename back to cover_image_path in files table
		_, err := db.Exec("ALTER TABLE files RENAME COLUMN cover_image_filename TO cover_image_path")
		if err != nil {
			return errors.WithStack(err)
		}

		// Rename back to cover_image_path in series table
		_, err = db.Exec("ALTER TABLE series RENAME COLUMN cover_image_filename TO cover_image_path")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
