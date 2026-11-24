package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// The Page flow a user chose for a Book joins their own record of it in
	// the change feed (ADR 0023), so the Shisho app reads a Book in the flow
	// chosen on the web, and its own choice reaches the web. When it was
	// chosen orders the app's late journal events against it, as a Rating's
	// time does.
	updateTrigger := func(columns string) string {
		return `CREATE TRIGGER change_log_user_books_update AFTER UPDATE ON user_books WHEN ` + columns + ` BEGIN
			DELETE FROM change_log_entries WHERE kind = 'user_book' AND record_id = NEW.book_id AND user_id = NEW.user_id;
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			VALUES ('user_book', NEW.book_id, NEW.user_id, FALSE, CURRENT_TIMESTAMP);
		END`
	}
	const before = `OLD.user_id IS NOT NEW.user_id OR OLD.book_id IS NOT NEW.book_id OR OLD.rating IS NOT NEW.rating ` +
		`OR OLD.to_read_position IS NOT NEW.to_read_position OR OLD.reference IS NOT NEW.reference`

	statements := []string{
		`ALTER TABLE user_books ADD COLUMN page_flow_updated_at TIMESTAMP`,
		// A flow chosen before this was chosen no later than its record last
		// changed, so a choice an app made earlier and sends late doesn't
		// replace it.
		`UPDATE user_books SET page_flow_updated_at = updated_at WHERE page_flow IS NOT NULL`,
		`DROP TRIGGER IF EXISTS change_log_user_books_update`,
		updateTrigger(before + ` OR OLD.page_flow IS NOT NEW.page_flow`),
		// Every record with a flow moves, so an installation that already
		// synced gets it too.
		`DELETE FROM change_log_entries WHERE kind = 'user_book' AND EXISTS (
			SELECT 1 FROM user_books ub WHERE ub.page_flow IS NOT NULL
			AND ub.book_id = change_log_entries.record_id AND ub.user_id = change_log_entries.user_id)`,
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'user_book', book_id, user_id, FALSE, CURRENT_TIMESTAMP
			FROM user_books WHERE page_flow IS NOT NULL ORDER BY user_id, book_id`,
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
			`DROP TRIGGER IF EXISTS change_log_user_books_update`,
			updateTrigger(before),
			`ALTER TABLE user_books DROP COLUMN page_flow_updated_at`,
		})
	}

	Migrations.MustRegister(up, down)
}
