package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// What the Shisho app's journal (ADR 0024) needs to place its reading
	// changes by their own time:
	//
	//   - user_books.to_read_changed_at is when the Book last joined, left,
	//     or moved on To Read, so the latest change of each Book wins.
	//   - list_books.moved_at is when the Book was last placed in its List,
	//     by adding or moving it, for the same rule there. NULL is when it
	//     was added.
	//   - read_throughs.history_only marks a finish or abandon made offline
	//     before a later Reference choice (ADR 0028): an entry in Reading
	//     History that never takes Reading Sessions.
	//   - app_journal_lists links the id an installation made a List under to
	//     the List, so its later events can name it before the feed brings
	//     the server's id. A deleted List keeps its row, with no List, so a
	//     create sent again doesn't bring it back.
	statements := []string{
		`ALTER TABLE user_books ADD COLUMN to_read_changed_at TIMESTAMP`,
		`ALTER TABLE list_books ADD COLUMN moved_at TIMESTAMP`,
		`ALTER TABLE read_throughs ADD COLUMN history_only BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE app_journal_lists (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			local_id TEXT NOT NULL,
			list_id INTEGER REFERENCES lists (id) ON DELETE SET NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, local_id)
		)`,
		`CREATE INDEX ix_app_journal_lists_list_id ON app_journal_lists (list_id)`,
	}

	run := func(ctx context.Context, db *bun.DB, statements []string) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.Wrapf(err, "run %s", statement)
				}
			}
			return nil
		})
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return run(ctx, db, statements)
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return run(ctx, db, []string{
			`DROP TABLE IF EXISTS app_journal_lists`,
			`ALTER TABLE read_throughs DROP COLUMN history_only`,
			`ALTER TABLE list_books DROP COLUMN moved_at`,
			`ALTER TABLE user_books DROP COLUMN to_read_changed_at`,
		})
	}

	Migrations.MustRegister(up, down)
}
