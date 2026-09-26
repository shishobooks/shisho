package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// jobs.created_by_user_id records who requested a job through POST /api/jobs
// so the creator of a bulk download can poll and fetch it without Jobs Read.
// Jobs enqueued any other way, or before this column existed, stay NULL.
// Deleting the user keeps the job and clears the reference.
func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, query := range []string{
				"ALTER TABLE jobs ADD COLUMN created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL",
				"CREATE INDEX ix_jobs_created_by_user_id ON jobs(created_by_user_id)",
			} {
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrap(err, "failed to add jobs created_by_user_id")
				}
			}
			return nil
		})
	}
	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// SQLite refuses to drop an indexed column, so drop the index first.
			for _, query := range []string{
				"DROP INDEX IF EXISTS ix_jobs_created_by_user_id",
				"ALTER TABLE jobs DROP COLUMN created_by_user_id",
			} {
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return errors.Wrap(err, "failed to drop jobs created_by_user_id")
				}
			}
			return nil
		})
	}
	Migrations.MustRegister(up, down)
}
