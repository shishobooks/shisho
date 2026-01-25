package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE job_logs ADD COLUMN plugin TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for filtering logs by plugin
		_, err = db.Exec(`CREATE INDEX ix_job_logs_plugin ON job_logs(plugin)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP INDEX IF EXISTS ix_job_logs_plugin`)
		if err != nil {
			return errors.WithStack(err)
		}
		// SQLite doesn't support DROP COLUMN before 3.35, but bun handles this
		_, err = db.Exec(`ALTER TABLE job_logs DROP COLUMN plugin`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
