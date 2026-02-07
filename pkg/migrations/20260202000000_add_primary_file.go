package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Add the column as nullable first
		_, err := db.Exec(`ALTER TABLE books ADD COLUMN primary_file_id INTEGER REFERENCES files(id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Populate primary_file_id for all existing books
		// Prefer main files over supplements, ordered by created_at
		_, err = db.Exec(`
			UPDATE books SET primary_file_id = (
				SELECT id FROM files
				WHERE files.book_id = books.id
				ORDER BY
					CASE WHEN file_role = 'main' THEN 0 ELSE 1 END,
					created_at ASC
				LIMIT 1
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE books DROP COLUMN primary_file_id`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
