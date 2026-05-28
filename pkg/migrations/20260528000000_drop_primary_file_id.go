package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// SQLite cannot drop a column without rebuilding the table.
		// Disable FK enforcement during the rebuild to avoid ordering issues.
		if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
			return errors.WithStack(err)
		}
		defer db.Exec("PRAGMA foreign_keys = ON") //nolint:errcheck

		// Create replacement table without primary_file_id.
		if _, err := db.Exec(`CREATE TABLE books_new (
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
			author_source TEXT NOT NULL,
			genre_source TEXT,
			tag_source TEXT
		)`); err != nil {
			return errors.WithStack(err)
		}

		// Copy data (all columns except primary_file_id).
		if _, err := db.Exec(`INSERT INTO books_new (
			id, created_at, updated_at, library_id, filepath, title, title_source,
			sort_title, sort_title_source, subtitle, subtitle_source,
			description, description_source, author_source, genre_source, tag_source
		) SELECT
			id, created_at, updated_at, library_id, filepath, title, title_source,
			sort_title, sort_title_source, subtitle, subtitle_source,
			description, description_source, author_source, genre_source, tag_source
		FROM books`); err != nil {
			return errors.WithStack(err)
		}

		if _, err := db.Exec("DROP TABLE books"); err != nil {
			return errors.WithStack(err)
		}

		if _, err := db.Exec("ALTER TABLE books_new RENAME TO books"); err != nil {
			return errors.WithStack(err)
		}

		// Recreate all indexes that existed on books.
		indexes := []string{
			`CREATE INDEX ix_books_library_id ON books (library_id)`,
			`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`,
		}
		for _, idx := range indexes {
			if _, err := db.Exec(idx); err != nil {
				return errors.WithStack(err)
			}
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		// Recreate the primary_file_id column and backfill with the oldest main
		// file per book (preferring main over supplement).
		if _, err := db.Exec(`ALTER TABLE books ADD COLUMN primary_file_id INTEGER REFERENCES files(id)`); err != nil {
			return errors.WithStack(err)
		}

		// Backfill: set primary_file_id to the oldest main file per book,
		// falling back to the oldest supplement if no main files exist.
		if _, err := db.Exec(`
			UPDATE books SET primary_file_id = (
				SELECT id FROM files
				WHERE files.book_id = books.id
				ORDER BY
					CASE WHEN file_role = 'main' THEN 0 ELSE 1 END,
					created_at ASC
				LIMIT 1
			)
		`); err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	Migrations.MustRegister(up, down)
}
