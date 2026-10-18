package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// SQLite cannot drop a column without rebuilding the table.
		// rebuildTableInTx needs matching column sets, so the copy is spelled
		// out here, inside the same pinned, foreign-keys-off transaction.
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// Create replacement table without primary_file_id. author_source
			// is nullable for the reason given on fkCascadeRebuilds.
			for _, q := range []string{
				`CREATE TABLE books_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					library_id INTEGER NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
					filepath TEXT NOT NULL,
					title TEXT NOT NULL,
					title_source TEXT NOT NULL,
					sort_title TEXT NOT NULL,
					sort_title_source TEXT NOT NULL,
					subtitle TEXT,
					subtitle_source TEXT,
					description TEXT,
					description_source TEXT,
					author_source TEXT,
					genre_source TEXT,
					tag_source TEXT
				)`,
				// Copy data (all columns except primary_file_id).
				`INSERT INTO books_new (
					id, created_at, updated_at, library_id, filepath, title, title_source,
					sort_title, sort_title_source, subtitle, subtitle_source,
					description, description_source, author_source, genre_source, tag_source
				) SELECT
					id, created_at, updated_at, library_id, filepath, title, title_source,
					sort_title, sort_title_source, subtitle, subtitle_source,
					description, description_source, author_source, genre_source, tag_source
				FROM books`,
				`DROP TABLE books`,
				`ALTER TABLE books_new RENAME TO books`,
				// Recreate all indexes that existed on books.
				`CREATE INDEX ix_books_library_id ON books (library_id)`,
				`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`,
			} {
				if _, err := tx.ExecContext(ctx, q); err != nil {
					return errors.Wrap(err, q)
				}
			}
			// No checkForeignKeys, for the reason given in 20260406100000.
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, q := range []string{
				// Recreate the primary_file_id column.
				`ALTER TABLE books ADD COLUMN primary_file_id INTEGER REFERENCES files(id)`,
				// Backfill: set primary_file_id to the oldest main file per book,
				// falling back to the oldest supplement if no main files exist.
				`UPDATE books SET primary_file_id = (
					SELECT id FROM files
					WHERE files.book_id = books.id
					ORDER BY
						CASE WHEN file_role = 'main' THEN 0 ELSE 1 END,
						created_at ASC
					LIMIT 1
				)`,
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
