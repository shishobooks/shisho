package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// Both statements run in one transaction, so a failure between them
	// doesn't leave one column behind for the retry to trip over.
	run := func(ctx context.Context, db *bun.DB, statements ...string) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		})
	}

	up := func(ctx context.Context, db *bun.DB) error {
		// Whether the user marked the Book Reference (ADR 0028), and when they
		// last chose a status for it explicitly, of any kind, which orders the
		// Shisho app's late journal events against it. Books before this have
		// no such choice recorded.
		return run(ctx, db,
			`ALTER TABLE user_books ADD COLUMN reference BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE user_books ADD COLUMN status_chosen_at TIMESTAMP`,
		)
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return run(ctx, db,
			`ALTER TABLE user_books DROP COLUMN status_chosen_at`,
			`ALTER TABLE user_books DROP COLUMN reference`,
		)
	}

	Migrations.MustRegister(up, down)
}
