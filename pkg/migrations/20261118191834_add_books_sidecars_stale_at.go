package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// When a rename or merge left a Book's sidecars naming a resource by
		// a name it no longer has. The sidecar rewrite job clears it, and a
		// scan reaching the Book first rewrites the sidecars before reading
		// them, so it never trusts a stale name (ADR 0027). The partial index
		// lets the job find the marked Books without scanning every row.
		statements := []string{
			`ALTER TABLE books ADD COLUMN sidecars_stale_at TIMESTAMP`,
			`CREATE INDEX ix_books_sidecars_stale_at ON books (sidecars_stale_at) WHERE sidecars_stale_at IS NOT NULL`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		statements := []string{
			`DROP INDEX IF EXISTS ix_books_sidecars_stale_at`,
			`ALTER TABLE books DROP COLUMN sidecars_stale_at`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
