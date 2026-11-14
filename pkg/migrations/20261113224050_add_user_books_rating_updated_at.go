package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// When a user's Rating of a Book last changed, set or cleared, apart
		// from the row's other changes. The KOReader plugin judges a Rating
		// changed on both sides by it, and sends a cleared Rating on to the
		// device (ADR 0016). Ratings set before it existed take the row's
		// updated_at.
		statements := []string{
			`ALTER TABLE user_books ADD COLUMN rating_updated_at TIMESTAMP`,
			`UPDATE user_books SET rating_updated_at = updated_at WHERE rating IS NOT NULL`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `ALTER TABLE user_books DROP COLUMN rating_updated_at`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
