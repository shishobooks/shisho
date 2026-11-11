package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

// readThroughsCreateSQL builds the read_throughs table with the Book
// reference bookIDColumn and any extra columns, so up and down rebuild the
// same table apart from what this migration changes.
func readThroughsCreateSQL(bookIDColumn, extraColumns string) string {
	return `CREATE TABLE read_throughs_new (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
		` + bookIDColumn + `,
		begin_day TEXT,
		end_day TEXT,
		outcome TEXT NOT NULL CHECK (outcome IN ('underway', 'finished', 'abandoned')),
		paused BOOLEAN NOT NULL DEFAULT FALSE,
		left_to_read_position INTEGER` + extraColumns + `
	)`
}

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			// Read-throughs outlive their Book (ADR 0015): deleting a Book
			// clears book_id, and the books service copies the Book's title
			// and author names (a JSON array) first, so the user can still
			// recognise it. Their Sessions stay with them.
			statements := []string{
				`ALTER TABLE read_throughs ADD COLUMN book_title TEXT`,
				`ALTER TABLE read_throughs ADD COLUMN book_authors TEXT`,
			}
			for _, s := range statements {
				if _, err := tx.ExecContext(ctx, s); err != nil {
					return errors.WithStack(err)
				}
			}
			createSQL := readThroughsCreateSQL(
				"book_id INTEGER REFERENCES books (id) ON DELETE SET NULL",
				",\n\t\tbook_title TEXT,\n\t\tbook_authors TEXT",
			)
			if err := rebuildTableInTx(ctx, tx, "read_throughs", createSQL); err != nil {
				return err
			}
			if err := checkForeignKeys(ctx, tx, []string{"read_throughs", "reading_sessions", "reading_position_updates"}); err != nil {
				return err
			}

			// Whether a Session was reading or listening is recorded when it's
			// written, so it survives its File. Existing ones take it from
			// their File, and one whose File is gone counts as reading.
			// duration is the seconds spent, which a source that knows about
			// pauses can set below the span; every Session so far is its span.
			statements = []string{
				`ALTER TABLE reading_sessions ADD COLUMN kind TEXT NOT NULL DEFAULT 'reading' CHECK (kind IN ('reading', 'listening'))`,
				`UPDATE reading_sessions SET kind = 'listening' WHERE file_id IN (SELECT id FROM files WHERE file_type = 'm4b')`,
				`ALTER TABLE reading_sessions ADD COLUMN duration INTEGER NOT NULL DEFAULT 0 CHECK (duration >= 0)`,
				`UPDATE reading_sessions SET duration = MAX(0, CAST(ROUND((julianday(ended_at) - julianday(started_at)) * 86400) AS INTEGER))`,
			}
			for _, s := range statements {
				if _, err := tx.ExecContext(ctx, s); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return withForeignKeysOff(ctx, db, func(ctx context.Context, tx bun.Tx) error {
			statements := []string{
				`ALTER TABLE reading_sessions DROP COLUMN duration`,
				`ALTER TABLE reading_sessions DROP COLUMN kind`,
				// The old table can't hold a Read-through without a Book. Foreign
				// keys are off, so its history is deleted here, not by cascade.
				`DELETE FROM reading_sessions WHERE read_through_id IN (SELECT id FROM read_throughs WHERE book_id IS NULL)`,
				`DELETE FROM reading_position_updates WHERE read_through_id IN (SELECT id FROM read_throughs WHERE book_id IS NULL)`,
				`DELETE FROM read_throughs WHERE book_id IS NULL`,
			}
			for _, s := range statements {
				if _, err := tx.ExecContext(ctx, s); err != nil {
					return errors.WithStack(err)
				}
			}
			createSQL := readThroughsCreateSQL(
				"book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL",
				",\n\t\tbook_title TEXT,\n\t\tbook_authors TEXT",
			)
			if err := rebuildTableInTx(ctx, tx, "read_throughs", createSQL); err != nil {
				return err
			}
			for _, s := range []string{
				`ALTER TABLE read_throughs DROP COLUMN book_authors`,
				`ALTER TABLE read_throughs DROP COLUMN book_title`,
			} {
				if _, err := tx.ExecContext(ctx, s); err != nil {
					return errors.WithStack(err)
				}
			}
			return checkForeignKeys(ctx, tx, []string{"read_throughs", "reading_sessions", "reading_position_updates"})
		})
	}

	Migrations.MustRegister(up, down)
}
