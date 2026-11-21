package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Whether the user marked the Book Reference (ADR 0028), and when they
		// last chose a status for it explicitly, of any kind, which orders the
		// Shisho app's late journal events against it. Books before this have
		// no such choice recorded.
		statements := []string{
			`ALTER TABLE user_books ADD COLUMN reference BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE user_books ADD COLUMN status_chosen_at TIMESTAMP`,
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
			`ALTER TABLE user_books DROP COLUMN status_chosen_at`,
			`ALTER TABLE user_books DROP COLUMN reference`,
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
