package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`CREATE INDEX ix_files_file_type_book_id ON files (file_type, book_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_files_file_type_book_id`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
