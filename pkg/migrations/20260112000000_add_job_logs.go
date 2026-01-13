package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create job_logs table
		_, err := db.Exec(`
			CREATE TABLE job_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
				level TEXT NOT NULL,
				message TEXT NOT NULL,
				data TEXT,
				stack_trace TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for fetching logs by job_id (most common query)
		_, err = db.Exec(`CREATE INDEX ix_job_logs_job_id ON job_logs(job_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for retention cleanup (delete old logs by created_at)
		_, err = db.Exec(`CREATE INDEX ix_job_logs_created_at ON job_logs(created_at)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for retention cleanup on jobs (delete old completed/failed jobs)
		_, err = db.Exec(`CREATE INDEX ix_jobs_status_created_at ON jobs(status, created_at)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_jobs_status_created_at`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_job_logs_created_at`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP INDEX IF EXISTS ix_job_logs_job_id`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS job_logs`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
