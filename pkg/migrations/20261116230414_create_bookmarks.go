package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// A Bookmark is one user's mark at a moment in an audiobook File, with
	// an optional note. It goes with its File and its user.
	up := func(_ context.Context, db *bun.DB) error {
		statements := []string{
			`CREATE TABLE bookmarks (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				file_id INTEGER REFERENCES files (id) ON DELETE CASCADE NOT NULL,
				seconds REAL NOT NULL CHECK (seconds >= 0),
				note TEXT
			)`,
			`CREATE INDEX ix_bookmarks_user_id_file_id ON bookmarks (user_id, file_id)`,
			`CREATE INDEX ix_bookmarks_file_id ON bookmarks (file_id)`,
		}
		for _, statement := range statements {
			if _, err := db.Exec(statement); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS bookmarks")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
