package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE chapters (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
				parent_id INTEGER REFERENCES chapters(id) ON DELETE CASCADE,
				sort_order INTEGER NOT NULL,
				title TEXT NOT NULL,
				start_page INTEGER,
				start_timestamp_ms INTEGER,
				href TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for fast lookup by file
		_, err = db.Exec(`CREATE INDEX ix_chapters_file_id ON chapters(file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for parent lookups (nested chapters)
		_, err = db.Exec(`CREATE INDEX ix_chapters_parent_id ON chapters(parent_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Unique constraint: sort_order is unique within siblings
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_chapters_file_sort ON chapters(file_id, parent_id, sort_order)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS chapters")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
