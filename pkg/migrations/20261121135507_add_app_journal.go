package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// The Shisho app's journal (ADR 0024) and what the change feed (ADR 0023)
	// gains with it: chapters on each File, and the user's own Reading
	// Positions and Read-throughs.
	//
	// app_journal_events records each event the server took in, by user,
	// installation, and the event's own id, so a resent event is applied once
	// even after the user deleted what it produced, and even after the
	// installation signed in again as a new Device.
	//
	// A user's own records are entries with user_id set and no library_id.
	// A Reading Position has no id of its own, so its entry is keyed by its
	// File, and two users' positions in one File share a record_id: the
	// unique index now covers the user too.
	const now = `CURRENT_TIMESTAMP`

	// recordOwn moves kind's entry for id, which belongs to user.
	recordOwn := func(kind, id, user string, deleted bool) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = '%[1]s' AND record_id = %[2]s AND user_id = %[3]s;
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			VALUES ('%[1]s', %[2]s, %[3]s, %[4]t, %[5]s);`, kind, id, user, deleted, now)
	}
	// moveBooks moves the entries of the Books whose ids the expressions
	// list, skipping any that no longer exist, as the change log's first
	// migration does.
	moveBooks := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'book' AND record_id IN (SELECT id FROM books WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'book', id, library_id, FALSE, %[2]s FROM books WHERE id IN (%[1]s);`, ids, now)
	}
	differs := func(columns ...string) string {
		terms := make([]string, len(columns))
		for i, column := range columns {
			terms[i] = fmt.Sprintf("OLD.%[1]s IS NOT NEW.%[1]s", column)
		}
		return strings.Join(terms, " OR ")
	}
	trigger := func(name, event, table, when, body string) string {
		if when != "" {
			when = "WHEN " + when
		}
		return fmt.Sprintf("CREATE TRIGGER change_log_%s AFTER %s ON %s %s BEGIN %s END", name, event, table, when, body)
	}
	// fileBook is the Book of the File a chapter row names.
	fileBook := func(row string) string {
		return fmt.Sprintf("SELECT book_id FROM files WHERE id = %s.file_id", row)
	}

	statements := []string{
		`CREATE TABLE app_journal_events (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			installation_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, installation_id, event_id)
		)`,

		// The Reading Session each sitting's snapshots update: the app sends
		// the session so far on every pause, under the sitting's id. A
		// session the user deleted keeps its row, with no session, so later
		// snapshots don't bring it back.
		`CREATE TABLE app_journal_sessions (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			installation_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			reading_session_id INTEGER REFERENCES reading_sessions (id) ON DELETE SET NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, installation_id, session_id)
		)`,
		`CREATE INDEX ix_app_journal_sessions_reading_session_id ON app_journal_sessions (reading_session_id)`,

		`DROP INDEX ux_change_log_entries_kind_record_id`,
		`CREATE UNIQUE INDEX ux_change_log_entries_kind_record_id_user_id ON change_log_entries (kind, record_id, IFNULL(user_id, 0))`,
		`CREATE INDEX ix_change_log_entries_user_id ON change_log_entries (user_id) WHERE user_id IS NOT NULL`,

		// Chapters are part of their File, so of its Book. A File's delete
		// cascades to its chapters after the File's row is gone, so they
		// find no Book to move; the File's own trigger moved it.
		trigger("chapters_insert", "INSERT", "chapters", "", moveBooks(fileBook("NEW"))),
		trigger("chapters_update", "UPDATE", "chapters",
			differs("file_id", "parent_id", "sort_order", "title", "start_page", "start_timestamp_ms", "href"),
			moveBooks(fileBook("OLD")+" UNION "+fileBook("NEW"))),
		trigger("chapters_delete", "DELETE", "chapters", "", moveBooks(fileBook("OLD"))),

		trigger("reading_positions_insert", "INSERT", "reading_positions", "",
			recordOwn("reading_position", "NEW.file_id", "NEW.user_id", false)),
		trigger("reading_positions_update", "UPDATE", "reading_positions",
			differs("user_id", "file_id", "cfi", "xpointer", "page", "seconds", "kobo_span", "percentage",
				"source", "source_device_id", "occurred_at"),
			recordOwn("reading_position", "NEW.file_id", "NEW.user_id", false)),
		trigger("reading_positions_delete", "DELETE", "reading_positions", "",
			recordOwn("reading_position", "OLD.file_id", "OLD.user_id", true)),

		trigger("read_throughs_insert", "INSERT", "read_throughs", "",
			recordOwn("read_through", "NEW.id", "NEW.user_id", false)),
		trigger("read_throughs_update", "UPDATE", "read_throughs",
			differs("user_id", "book_id", "begin_day", "end_day", "outcome", "paused"),
			recordOwn("read_through", "NEW.id", "NEW.user_id", false)),
		trigger("read_throughs_delete", "DELETE", "read_throughs", "",
			recordOwn("read_through", "OLD.id", "OLD.user_id", true)),

		// Every position and Read-through there is now has an entry, and
		// every Book with chapters moves, so an installation that already
		// synced gets them too.
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'reading_position', file_id, user_id, FALSE, CURRENT_TIMESTAMP FROM reading_positions ORDER BY occurred_at`,
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'read_through', id, user_id, FALSE, CURRENT_TIMESTAMP FROM read_throughs ORDER BY id`,
		moveBooks("SELECT f.book_id FROM files f WHERE EXISTS (SELECT 1 FROM chapters c WHERE c.file_id = f.id)"),
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
				`DROP TRIGGER IF EXISTS change_log_chapters_insert`,
				`DROP TRIGGER IF EXISTS change_log_chapters_update`,
				`DROP TRIGGER IF EXISTS change_log_chapters_delete`,
				`DROP TRIGGER IF EXISTS change_log_reading_positions_insert`,
				`DROP TRIGGER IF EXISTS change_log_reading_positions_update`,
				`DROP TRIGGER IF EXISTS change_log_reading_positions_delete`,
				`DROP TRIGGER IF EXISTS change_log_read_throughs_insert`,
				`DROP TRIGGER IF EXISTS change_log_read_throughs_update`,
				`DROP TRIGGER IF EXISTS change_log_read_throughs_delete`,
				`DELETE FROM change_log_entries WHERE user_id IS NOT NULL`,
				`DROP INDEX IF EXISTS ix_change_log_entries_user_id`,
				`DROP INDEX IF EXISTS ux_change_log_entries_kind_record_id_user_id`,
				`CREATE UNIQUE INDEX ux_change_log_entries_kind_record_id ON change_log_entries (kind, record_id)`,
				`DROP TABLE IF EXISTS app_journal_sessions`,
				`DROP TABLE IF EXISTS app_journal_events`,
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
