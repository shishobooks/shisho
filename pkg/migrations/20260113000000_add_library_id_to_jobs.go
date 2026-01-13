package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Add library_id column to jobs table
		_, err := db.Exec(`ALTER TABLE jobs ADD COLUMN library_id INTEGER REFERENCES libraries(id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for filtering scans by library (includes global scans where library_id IS NULL)
		_, err = db.Exec(`CREATE INDEX ix_jobs_type_library_created ON jobs(type, library_id, created_at DESC)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_jobs_type_library_created`)
		if err != nil {
			return errors.WithStack(err)
		}
		// SQLite doesn't support DROP COLUMN, so we'd need to recreate the table
		// For simplicity, we'll leave the column in place on rollback
		return nil
	}

	Migrations.MustRegister(up, down)
}
