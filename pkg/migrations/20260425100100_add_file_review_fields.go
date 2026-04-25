package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`ALTER TABLE files ADD COLUMN review_override TEXT
				CHECK (review_override IS NULL OR review_override IN ('reviewed','unreviewed'))`,
			`ALTER TABLE files ADD COLUMN review_overridden_at TIMESTAMP`,
			`ALTER TABLE files ADD COLUMN reviewed BOOLEAN`,
			`CREATE INDEX idx_files_book_reviewed
				ON files(book_id, reviewed)
				WHERE file_role = 'main'`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`DROP INDEX IF EXISTS idx_files_book_reviewed`,
			`ALTER TABLE files DROP COLUMN reviewed`,
			`ALTER TABLE files DROP COLUMN review_overridden_at`,
			`ALTER TABLE files DROP COLUMN review_override`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
