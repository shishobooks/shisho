package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Read-throughs are the reading record, and Reading Status is derived
		// from them and user_books on every read (ADR 0013). Days are stored
		// as YYYY-MM-DD text: they are calendar days in the user's time zone,
		// not instants. The partial unique index allows at most one
		// Read-through underway per user and Book.
		_, err := db.Exec(`
			CREATE TABLE read_throughs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
				begin_day TEXT,
				end_day TEXT,
				outcome TEXT NOT NULL CHECK (outcome IN ('underway', 'finished', 'abandoned')),
				paused BOOLEAN NOT NULL DEFAULT FALSE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_read_throughs_user_id_book_id ON read_throughs (user_id, book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_read_throughs_book_id ON read_throughs (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_read_throughs_one_underway ON read_throughs (user_id, book_id) WHERE outcome = 'underway'`)
		if err != nil {
			return errors.WithStack(err)
		}

		// One row per user and Book for what isn't tied to a Read-through:
		// the Rating, in quarter stars from 1 to 20 (NULL is unrated), and
		// the To Read position (NULL is not on To Read).
		_, err = db.Exec(`
			CREATE TABLE user_books (
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				rating INTEGER CHECK (rating BETWEEN 1 AND 20),
				to_read_position INTEGER,
				PRIMARY KEY (user_id, book_id)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_user_books_book_id ON user_books (book_id)`)
		return errors.WithStack(err)
	}

	down := func(_ context.Context, db *bun.DB) error {
		if _, err := db.Exec("DROP TABLE IF EXISTS user_books"); err != nil {
			return errors.WithStack(err)
		}
		_, err := db.Exec("DROP TABLE IF EXISTS read_throughs")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
