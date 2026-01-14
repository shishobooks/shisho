package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create file_identifiers table
		_, err := db.Exec(`
			CREATE TABLE file_identifiers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				type TEXT NOT NULL,
				value TEXT NOT NULL,
				source TEXT NOT NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index on file_id for fast lookups by file
		_, err = db.Exec(`CREATE INDEX ix_file_identifiers_file_id ON file_identifiers (file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index on value for search functionality
		_, err = db.Exec(`CREATE INDEX ix_file_identifiers_value ON file_identifiers (value)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Unique constraint: one identifier of each type per file
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_file_identifiers_file_type ON file_identifiers (file_id, type)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Add identifier_source column to files table
		_, err = db.Exec(`ALTER TABLE files ADD COLUMN identifier_source TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// SQLite doesn't support DROP COLUMN in older versions, so we skip dropping identifier_source
		// The column will just be unused after rollback

		_, err := db.Exec("DROP TABLE IF EXISTS file_identifiers")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
