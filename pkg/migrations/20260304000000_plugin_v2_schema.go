package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// 1. Add status column (int), convert enabled -> status
		_, err := db.Exec(`ALTER TABLE plugins ADD COLUMN status INTEGER NOT NULL DEFAULT 0`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Convert: enabled=true -> status=0 (Active), enabled=false -> status=-1 (Disabled)
		_, err = db.Exec(`UPDATE plugins SET status = CASE WHEN enabled = true THEN 0 ELSE -1 END`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 2. Add auto_update column
		_, err = db.Exec(`ALTER TABLE plugins ADD COLUMN auto_update BOOLEAN NOT NULL DEFAULT true`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 3. Add repository_scope and repository_url columns
		_, err = db.Exec(`ALTER TABLE plugins ADD COLUMN repository_scope TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`ALTER TABLE plugins ADD COLUMN repository_url TEXT`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 4. Drop enabled column by recreating the table (SQLite limitation)
		// Note: SQLite doesn't support DROP COLUMN in older versions, so we use
		// the standard recreate approach. However, modern SQLite (3.35+) supports it.
		_, err = db.Exec(`ALTER TABLE plugins DROP COLUMN enabled`)
		if err != nil {
			return errors.WithStack(err)
		}

		// 5. Add index on status for filtering
		_, err = db.Exec(`CREATE INDEX ix_plugins_status ON plugins(status)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				`DROP INDEX IF EXISTS ix_plugins_status`,
				`ALTER TABLE plugins ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true`,
				`UPDATE plugins SET enabled = CASE WHEN status = 0 THEN true ELSE false END`,
				`ALTER TABLE plugins DROP COLUMN status`,
				`ALTER TABLE plugins DROP COLUMN auto_update`,
				`ALTER TABLE plugins DROP COLUMN repository_scope`,
				`ALTER TABLE plugins DROP COLUMN repository_url`,
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
