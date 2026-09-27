package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// revoked_at and the counters are unused until revocation and usage
		// counts land, but they ship here so Share Links need one migration.
		// IF NOT EXISTS lets a retry finish after a failed index statement.
		_, err := db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS share_links (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				token TEXT NOT NULL,
				book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE,
				created_by_user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
				label TEXT,
				expires_at TIMESTAMPTZ,
				revoked_at TIMESTAMPTZ,
				open_count INTEGER NOT NULL DEFAULT 0,
				download_count INTEGER NOT NULL DEFAULT 0,
				last_accessed_at TIMESTAMPTZ
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, stmt := range []string{
			`CREATE UNIQUE INDEX IF NOT EXISTS ux_share_links_token ON share_links (token)`,
			`CREATE INDEX IF NOT EXISTS ix_share_links_book_id ON share_links (book_id)`,
			`CREATE INDEX IF NOT EXISTS ix_share_links_created_by_user_id ON share_links (created_by_user_id)`,
		} {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS share_links`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
