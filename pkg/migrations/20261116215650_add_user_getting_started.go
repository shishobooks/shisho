package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Each user's Getting Started state. Existing users get it only while
		// a forced password reset is pending, since they never finished
		// signing in; an upgrade doesn't push first steps at everyone else.
		statements := []string{
			`ALTER TABLE users ADD COLUMN getting_started_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE users ADD COLUMN getting_started_auto_open BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE users ADD COLUMN getting_started_dismissed_at TIMESTAMP`,
			`ALTER TABLE users ADD COLUMN getting_started_not_for_me TEXT NOT NULL DEFAULT '[]'`,
			// The steps that applied when the list was dismissed, so a step
			// that applies later can bring it back.
			`ALTER TABLE users ADD COLUMN getting_started_dismissed_steps TEXT NOT NULL DEFAULT '[]'`,
			`UPDATE users SET getting_started_enabled = TRUE, getting_started_auto_open = TRUE WHERE must_change_password`,
		}
		for _, statement := range statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		for _, column := range []string{
			"getting_started_dismissed_steps",
			"getting_started_not_for_me",
			"getting_started_dismissed_at",
			"getting_started_auto_open",
			"getting_started_enabled",
		} {
			if _, err := db.ExecContext(ctx, `ALTER TABLE users DROP COLUMN `+column); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
