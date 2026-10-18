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

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				`DROP INDEX IF EXISTS ix_jobs_type_library_created`,
				`ALTER TABLE jobs DROP COLUMN library_id`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}
