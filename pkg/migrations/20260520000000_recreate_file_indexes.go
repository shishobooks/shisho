package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`CREATE INDEX IF NOT EXISTS ix_files_file_type_book_id ON files (file_type, book_id)`,
			`CREATE INDEX IF NOT EXISTS idx_files_language ON files(language COLLATE NOCASE) WHERE language IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_files_book_reviewed ON files(book_id, reviewed) WHERE file_role = 'main'`,
		}
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(ctx context.Context, db *bun.DB) error {
		stmts := []string{
			`DROP INDEX IF EXISTS idx_files_book_reviewed`,
			`DROP INDEX IF EXISTS idx_files_language`,
			`DROP INDEX IF EXISTS ix_files_file_type_book_id`,
		}
		for _, stmt := range stmts {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
