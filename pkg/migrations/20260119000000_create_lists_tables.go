package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create lists table
		_, err := db.Exec(`
			CREATE TABLE lists (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				name TEXT NOT NULL,
				description TEXT,
				is_ordered BOOLEAN NOT NULL DEFAULT FALSE,
				default_sort TEXT NOT NULL DEFAULT 'added_at_desc'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_lists_user_name ON lists (user_id, name COLLATE NOCASE)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_lists_user_id ON lists (user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create list_books table
		_, err = db.Exec(`
			CREATE TABLE list_books (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
				book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
				added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				added_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
				sort_order INTEGER
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_books ON list_books (list_id, book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_books_book_id ON list_books (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_books_list_sort ON list_books (list_id, sort_order)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create list_shares table
		_, err = db.Exec(`
			CREATE TABLE list_shares (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				permission TEXT NOT NULL CHECK (permission IN ('viewer', 'editor', 'manager')),
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				shared_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_shares ON list_shares (list_id, user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_shares_user_id ON list_shares (user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP TABLE IF EXISTS list_shares`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS list_books`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS lists`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
