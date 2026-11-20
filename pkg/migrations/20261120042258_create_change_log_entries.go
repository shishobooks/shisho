package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// The change log the Shisho app syncs its copy of the catalog from (ADR
	// 0023). Triggers keep one entry per record the app sees, so a change
	// moves the record's entry to the next sequence number: the old entry is
	// deleted and a new one inserted, and AUTOINCREMENT never reuses a seq.
	// Each entry records the Library (or, for user records, the user) it
	// belongs to when written, because a deleted row, or a cascade's parent,
	// can't be looked up afterwards.
	//
	// Update triggers fire only when a compared column differs, so a Scan
	// that rewrites rows with the values they hold records nothing. A
	// child's change moves its Book only while the Book still exists, so a
	// cascade from a Book's delete never overwrites the Book's deletion
	// entry. The trigger invariant test in this package checks that every
	// table and column is either compared or left out with a reason.
	const now = `CURRENT_TIMESTAMP`

	// record moves kind's entry for id, which belongs to library.
	record := func(kind, id, library string, deleted bool) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = '%[1]s' AND record_id = %[2]s;
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			VALUES ('%[1]s', %[2]s, %[3]s, %[4]t, %[5]s);`, kind, id, library, deleted, now)
	}
	// moveLibraries moves the entries of the Libraries whose ids the
	// expressions list, skipping any that no longer exist.
	moveLibraries := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'library' AND record_id IN (SELECT id FROM libraries WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'library', id AS record_id, id AS library_id, FALSE, %[2]s FROM libraries WHERE id IN (%[1]s);`, ids, now)
	}
	// moveBooks moves the entries of the Books whose ids the expressions
	// list, skipping any that no longer exist.
	moveBooks := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'book' AND record_id IN (SELECT id FROM books WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'book', id, library_id, FALSE, %[2]s FROM books WHERE id IN (%[1]s);`, ids, now)
	}
	// differs is a WHEN clause that holds when any of columns changed.
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

	statements := []string{
		`CREATE TABLE change_log_entries (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			record_id INTEGER NOT NULL,
			library_id INTEGER,
			user_id INTEGER,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX ux_change_log_entries_kind_record_id ON change_log_entries (kind, record_id)`,
		`CREATE INDEX ix_change_log_entries_deleted_changed_at ON change_log_entries (changed_at) WHERE deleted`,
		// The highest seq whose deletion entries have expired. An
		// installation whose cursor is below it may have missed a deletion.
		`CREATE TABLE change_log_expiries (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			expired_through INTEGER NOT NULL,
			expired_at TIMESTAMP NOT NULL
		)`,

		// Library: its name and cover aspect ratio. The ratio decides which
		// File's cover each of its Books shows, so it moves them too.
		trigger("libraries_insert", "INSERT", "libraries", "", record("library", "NEW.id", "NEW.id", false)),
		trigger("libraries_update", "UPDATE", "libraries", differs("name", "cover_aspect_ratio"), record("library", "NEW.id", "NEW.id", false)),
		trigger("libraries_update_covers", "UPDATE", "libraries", differs("cover_aspect_ratio"), moveBooks("SELECT id FROM books WHERE library_id = NEW.id")),
		trigger("libraries_delete", "DELETE", "libraries", "", record("library", "OLD.id", "OLD.id", true)),

		// A user's saved sort for a Library is part of the Library's
		// document, so every user reads the Library again, each with their
		// own sort. A Library's delete cascades to its saved sorts with the
		// Library's row already gone, and before the Library's own delete
		// trigger records the deletion, so the cascade writes nothing.
		trigger("user_library_settings_insert", "INSERT", "user_library_settings", "", moveLibraries("NEW.library_id")),
		trigger("user_library_settings_update", "UPDATE", "user_library_settings", differs("user_id", "library_id", "sort_spec"),
			moveLibraries("OLD.library_id, NEW.library_id")),
		trigger("user_library_settings_delete", "DELETE", "user_library_settings", "", moveLibraries("OLD.library_id")),

		// Book: its own columns, then each part of its document.
		trigger("books_insert", "INSERT", "books", "", record("book", "NEW.id", "NEW.library_id", false)),
		trigger("books_update", "UPDATE", "books", differs("title", "subtitle", "sort_title", "library_id"), record("book", "NEW.id", "NEW.library_id", false)),
		trigger("books_delete", "DELETE", "books", "", record("book", "OLD.id", "OLD.library_id", true)),

		// Files: File Type, role, and size, what the Library sorts by
		// (release date, page count, duration), and what the cover version is
		// made of (which File's cover is chosen, and that File's updated_at).
		trigger("files_insert", "INSERT", "files", "", moveBooks("NEW.book_id")),
		trigger("files_update", "UPDATE", "files",
			differs("book_id", "file_type", "file_role", "filesize_bytes", "release_date", "page_count", "audiobook_duration_seconds",
				"cover_image_filename", "is_preferred_cover", "updated_at"),
			moveBooks("OLD.book_id, NEW.book_id")),
		trigger("files_delete", "DELETE", "files", "", moveBooks("OLD.book_id")),

		trigger("authors_insert", "INSERT", "authors", "", moveBooks("NEW.book_id")),
		trigger("authors_update", "UPDATE", "authors", differs("book_id", "person_id", "sort_order", "role"), moveBooks("OLD.book_id, NEW.book_id")),
		trigger("authors_delete", "DELETE", "authors", "", moveBooks("OLD.book_id")),

		trigger("book_series_insert", "INSERT", "book_series", "", moveBooks("NEW.book_id")),
		trigger("book_series_update", "UPDATE", "book_series",
			differs("book_id", "series_id", "series_number", "series_number_end", "series_number_unit", "sort_order"),
			moveBooks("OLD.book_id, NEW.book_id")),
		trigger("book_series_delete", "DELETE", "book_series", "", moveBooks("OLD.book_id")),

		// A Person's or Series' name is part of each Book that names it.
		// Adding or deleting one reaches the Books through authors and
		// book_series.
		trigger("persons_update", "UPDATE", "persons", differs("name", "sort_name"),
			moveBooks("SELECT book_id FROM authors WHERE person_id = NEW.id")),
		trigger("series_update", "UPDATE", "series", differs("name", "sort_name"),
			moveBooks("SELECT book_id FROM book_series WHERE series_id = NEW.id")),

		// Every Library and Book there is now has an entry, so an
		// installation syncing from the start gets the whole catalog.
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'library', id AS record_id, id AS library_id, FALSE, CURRENT_TIMESTAMP FROM libraries ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'book', id, library_id, FALSE, CURRENT_TIMESTAMP FROM books ORDER BY id`,
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
			var triggers []string
			if err := tx.NewRaw(`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'change\_log\_%' ESCAPE '\'`).Scan(ctx, &triggers); err != nil {
				return errors.WithStack(err)
			}
			for _, name := range triggers {
				if _, err := tx.ExecContext(ctx, "DROP TRIGGER "+name); err != nil {
					return errors.WithStack(err)
				}
			}
			for _, statement := range []string{
				`DROP TABLE IF EXISTS change_log_expiries`,
				`DROP TABLE IF EXISTS change_log_entries`,
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
