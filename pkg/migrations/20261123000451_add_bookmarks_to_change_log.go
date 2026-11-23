package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// Bookmarks join the change feed (ADR 0023) as the user's own records,
	// and the Shisho app's journal (ADR 0024) can make, edit, and delete
	// them.
	//
	// app_journal_bookmarks links the id an installation made a Bookmark
	// under to the Bookmark, so its later events can name it before the feed
	// brings the server's id, and the feed can tell the installation it's
	// its own. A deleted Bookmark keeps its row, with no Bookmark, so a
	// create sent again doesn't bring it back. The row is written in the
	// transaction that makes the Bookmark and never changes after, so the
	// Bookmark's own entry carries it.
	recordOwn := func(id, user string, deleted bool) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'bookmark' AND record_id = %[1]s AND user_id = %[2]s;
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			VALUES ('bookmark', %[1]s, %[2]s, %[3]t, CURRENT_TIMESTAMP);`, id, user, deleted)
	}
	differs := func(columns ...string) string {
		terms := make([]string, len(columns))
		for i, column := range columns {
			terms[i] = fmt.Sprintf("OLD.%[1]s IS NOT NEW.%[1]s", column)
		}
		return strings.Join(terms, " OR ")
	}

	statements := []string{
		`CREATE TABLE app_journal_bookmarks (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			local_id TEXT NOT NULL,
			bookmark_id INTEGER REFERENCES bookmarks (id) ON DELETE SET NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, local_id)
		)`,
		`CREATE INDEX ix_app_journal_bookmarks_bookmark_id ON app_journal_bookmarks (bookmark_id)`,

		`CREATE TRIGGER change_log_bookmarks_insert AFTER INSERT ON bookmarks BEGIN ` +
			recordOwn("NEW.id", "NEW.user_id", false) + ` END`,
		`CREATE TRIGGER change_log_bookmarks_update AFTER UPDATE ON bookmarks WHEN ` +
			differs("user_id", "file_id", "seconds", "note", "created_at") + ` BEGIN ` +
			recordOwn("NEW.id", "NEW.user_id", false) + ` END`,
		`CREATE TRIGGER change_log_bookmarks_delete AFTER DELETE ON bookmarks BEGIN ` +
			recordOwn("OLD.id", "OLD.user_id", true) + ` END`,

		// Every Bookmark there is now has an entry, so an installation that
		// already synced gets them too.
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'bookmark', id, user_id, FALSE, CURRENT_TIMESTAMP FROM bookmarks ORDER BY id`,
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, statement := range statements {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.Wrapf(err, "run %s", statement)
				}
			}
			return nil
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, statement := range []string{
				`DROP TRIGGER IF EXISTS change_log_bookmarks_insert`,
				`DROP TRIGGER IF EXISTS change_log_bookmarks_update`,
				`DROP TRIGGER IF EXISTS change_log_bookmarks_delete`,
				`DELETE FROM change_log_entries WHERE kind = 'bookmark'`,
				`DROP TABLE IF EXISTS app_journal_bookmarks`,
			} {
				if _, err := tx.ExecContext(ctx, statement); err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		})
	}

	Migrations.MustRegister(up, down)
}
