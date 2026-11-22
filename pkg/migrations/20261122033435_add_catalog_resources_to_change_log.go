package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// What the change feed (ADR 0023) gains for browsing and searching the
	// whole catalog offline: Series, Persons, Genres, Tags, and Publishers as
	// records of their own, with their aliases; each Book's description,
	// genres, and tags, and each File's publisher and identifiers; Lists,
	// with their Books and List Shares; and the user's own record for each
	// Book, holding Rating and To Read.
	//
	// A List's entry carries its owner as user_id, so the owner sees it as
	// one of their own records. Everyone it's shared with sees it through the
	// feed's List Shares filter, and learns of a List deleted or unshared
	// from the access each page lists, since its List Shares are gone.
	//
	// A Book moved to another Library leaves a book_left deletion in the
	// Library it left, so a user who can't access the new one drops it. Each
	// Library a Book left keeps its own, so a Book moved twice between an
	// installation's syncs still leaves the first: the unique index takes in
	// the Library for book_left.
	const now = `CURRENT_TIMESTAMP`

	// record moves kind's entry for id, which belongs to library.
	record := func(kind, id, library string, deleted bool) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = '%[1]s' AND record_id = %[2]s;
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			VALUES ('%[1]s', %[2]s, %[3]s, %[4]t, %[5]s);`, kind, id, library, deleted, now)
	}
	// recordOwn moves kind's entry for id, which belongs to user.
	recordOwn := func(kind, id, user string, deleted bool) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = '%[1]s' AND record_id = %[2]s AND user_id = %[3]s;
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			VALUES ('%[1]s', %[2]s, %[3]s, %[4]t, %[5]s);`, kind, id, user, deleted, now)
	}
	// moveRecords moves the entries of kind's rows in table whose ids the
	// expressions list, skipping any that no longer exist.
	moveRecords := func(kind, table, ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = '%[1]s' AND record_id IN (SELECT id FROM %[2]s WHERE id IN (%[3]s));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT '%[1]s', id, library_id, FALSE, %[4]s FROM %[2]s WHERE id IN (%[3]s);`, kind, table, ids, now)
	}
	moveBooks := func(ids string) string { return moveRecords("book", "books", ids) }
	// moveLists moves the entries of the Lists whose ids the expressions
	// list, each under its owner, skipping any that no longer exist. A List's
	// delete cascades to its Books and List Shares with the List already
	// gone, so the cascade writes nothing.
	moveLists := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'list' AND record_id IN (SELECT id FROM lists WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'list', id, user_id, FALSE, %[2]s FROM lists WHERE id IN (%[1]s);`, ids, now)
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
	// fileBook is the Book of the File a row names.
	fileBook := func(row string) string {
		return fmt.Sprintf("SELECT book_id FROM files WHERE id = %s.file_id", row)
	}
	// resource is a catalog record of its own: its own triggers, and the
	// triggers of its aliases, which are part of its document.
	resource := func(kind, table, aliases, parent string, compared ...string) []string {
		return []string{
			trigger(table+"_insert", "INSERT", table, "", record(kind, "NEW.id", "NEW.library_id", false)),
			trigger(table+"_update_document", "UPDATE", table, differs(compared...), record(kind, "NEW.id", "NEW.library_id", false)),
			trigger(table+"_delete", "DELETE", table, "", record(kind, "OLD.id", "OLD.library_id", true)),
			trigger(aliases+"_insert", "INSERT", aliases, "", moveRecords(kind, table, "NEW."+parent)),
			trigger(aliases+"_update", "UPDATE", aliases, differs(parent, "name"), moveRecords(kind, table, "OLD."+parent+", NEW."+parent)),
			trigger(aliases+"_delete", "DELETE", aliases, "", moveRecords(kind, table, "OLD."+parent)),
		}
	}
	// bookJoin is a join table whose rows are part of a Book's document.
	bookJoin := func(table string, compared ...string) []string {
		return []string{
			trigger(table+"_insert", "INSERT", table, "", moveBooks("NEW.book_id")),
			trigger(table+"_update", "UPDATE", table, differs(compared...), moveBooks("OLD.book_id, NEW.book_id")),
			trigger(table+"_delete", "DELETE", table, "", moveBooks("OLD.book_id")),
		}
	}

	statements := []string{
		`DROP INDEX ux_change_log_entries_kind_record_id_user_id`,
		`CREATE UNIQUE INDEX ux_change_log_entries_record ON change_log_entries
			(kind, record_id, IFNULL(user_id, 0), CASE WHEN kind = 'book_left' THEN library_id ELSE 0 END)`,
		// Each page says which Libraries have records behind its cursor.
		`CREATE INDEX ix_change_log_entries_library_id_seq ON change_log_entries (library_id, seq)`,
	}
	statements = append(statements, resource("series", "series", "series_aliases", "series_id", "name", "sort_name", "description")...)
	statements = append(statements, resource("person", "persons", "person_aliases", "person_id", "name", "sort_name")...)
	statements = append(statements, resource("genre", "genres", "genre_aliases", "genre_id", "name")...)
	statements = append(statements, resource("tag", "tags", "tag_aliases", "tag_id", "name")...)
	statements = append(statements, resource("publisher", "publishers", "publisher_aliases", "publisher_id", "name", "parent_id")...)
	statements = append(statements, bookJoin("book_genres", "book_id", "genre_id")...)
	statements = append(statements, bookJoin("book_tags", "book_id", "tag_id")...)
	statements = append(statements,
		// A Genre's, Tag's, or Publisher's name is part of each Book that
		// names it.
		trigger("genres_update", "UPDATE", "genres", differs("name"),
			moveBooks("SELECT book_id FROM book_genres WHERE genre_id = NEW.id")),
		trigger("tags_update", "UPDATE", "tags", differs("name"),
			moveBooks("SELECT book_id FROM book_tags WHERE tag_id = NEW.id")),
		trigger("publishers_update", "UPDATE", "publishers", differs("name"),
			moveBooks("SELECT book_id FROM files WHERE publisher_id = NEW.id")),

		// A File's identifiers are part of its Book.
		trigger("file_identifiers_insert", "INSERT", "file_identifiers", "", moveBooks(fileBook("NEW"))),
		trigger("file_identifiers_update", "UPDATE", "file_identifiers", differs("file_id", "type", "value"),
			moveBooks(fileBook("OLD")+" UNION "+fileBook("NEW"))),
		trigger("file_identifiers_delete", "DELETE", "file_identifiers", "", moveBooks(fileBook("OLD"))),

		// The Book's description and each File's publisher and details join
		// their documents, so their update triggers compare them too.
		`DROP TRIGGER change_log_books_update`,
		trigger("books_update", "UPDATE", "books",
			differs("title", "subtitle", "sort_title", "created_at", "library_id", "description"),
			record("book", "NEW.id", "NEW.library_id", false)),
		trigger("books_update_library", "UPDATE", "books", differs("library_id"),
			fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'book_left' AND record_id = OLD.id AND library_id = OLD.library_id;
				INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
				VALUES ('book_left', OLD.id, OLD.library_id, TRUE, %s);`, now)),
		`DROP TRIGGER change_log_files_update`,
		trigger("files_update", "UPDATE", "files",
			differs("book_id", "file_type", "file_role", "filesize_bytes", "name", "name_source", "filepath",
				"release_date", "page_count", "audiobook_duration_seconds",
				"cover_image_filename", "is_preferred_cover", "updated_at", "publisher_id",
				"audiobook_codec", "audiobook_bitrate_bps", "language", "abridged", "url", "reading_direction",
				"file_modified_at", "created_at"),
			moveBooks("OLD.book_id, NEW.book_id")),

		// Lists, their Books, and their List Shares, since the document says
		// what each user may do with it.
		trigger("lists_insert", "INSERT", "lists", "", moveLists("NEW.id")),
		trigger("lists_update", "UPDATE", "lists", differs("user_id", "name", "description", "is_ordered", "default_sort"),
			moveLists("NEW.id")),
		trigger("lists_delete", "DELETE", "lists", "",
			fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'list' AND record_id = OLD.id;
				INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
				VALUES ('list', OLD.id, OLD.user_id, TRUE, %s);`, now)),
		trigger("list_books_insert", "INSERT", "list_books", "", moveLists("NEW.list_id")),
		trigger("list_books_update", "UPDATE", "list_books", differs("list_id", "book_id", "added_at", "sort_order"),
			moveLists("OLD.list_id, NEW.list_id")),
		trigger("list_books_delete", "DELETE", "list_books", "", moveLists("OLD.list_id")),
		trigger("list_shares_insert", "INSERT", "list_shares", "", moveLists("NEW.list_id")),
		trigger("list_shares_update", "UPDATE", "list_shares", differs("list_id", "user_id", "permission"),
			moveLists("OLD.list_id, NEW.list_id")),
		trigger("list_shares_delete", "DELETE", "list_shares", "", moveLists("OLD.list_id")),

		// The user's own record for each Book, keyed by the Book.
		trigger("user_books_insert", "INSERT", "user_books", "",
			recordOwn("user_book", "NEW.book_id", "NEW.user_id", false)),
		trigger("user_books_update", "UPDATE", "user_books",
			differs("user_id", "book_id", "rating", "to_read_position", "reference"),
			recordOwn("user_book", "NEW.book_id", "NEW.user_id", false)),
		trigger("user_books_delete", "DELETE", "user_books", "",
			recordOwn("user_book", "OLD.book_id", "OLD.user_id", true)),

		// Everything there is now has an entry, and every Book moves for what
		// its document gained, so an installation that already synced gets
		// them too.
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'series', id, library_id, FALSE, CURRENT_TIMESTAMP FROM series ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'person', id, library_id, FALSE, CURRENT_TIMESTAMP FROM persons ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'genre', id, library_id, FALSE, CURRENT_TIMESTAMP FROM genres ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'tag', id, library_id, FALSE, CURRENT_TIMESTAMP FROM tags ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'publisher', id, library_id, FALSE, CURRENT_TIMESTAMP FROM publishers ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'list', id, user_id, FALSE, CURRENT_TIMESTAMP FROM lists ORDER BY id`,
		`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'user_book', book_id, user_id, FALSE, CURRENT_TIMESTAMP FROM user_books ORDER BY user_id, book_id`,
		moveBooks("SELECT id FROM books"),
	)

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
			var names []string
			for _, statement := range statements {
				name, ok := strings.CutPrefix(statement, "CREATE TRIGGER ")
				if !ok {
					continue
				}
				name, _, _ = strings.Cut(name, " ")
				names = append(names, name)
			}
			for _, name := range names {
				if _, err := tx.ExecContext(ctx, "DROP TRIGGER IF EXISTS "+name); err != nil {
					return errors.WithStack(err)
				}
			}
			// The earlier migration's update triggers, as it made them.
			for _, statement := range []string{
				trigger("books_update", "UPDATE", "books", differs("title", "subtitle", "sort_title", "created_at", "library_id"),
					record("book", "NEW.id", "NEW.library_id", false)),
				trigger("files_update", "UPDATE", "files",
					differs("book_id", "file_type", "file_role", "filesize_bytes", "name", "name_source", "filepath",
						"release_date", "page_count", "audiobook_duration_seconds",
						"cover_image_filename", "is_preferred_cover", "updated_at"),
					moveBooks("OLD.book_id, NEW.book_id")),
				`DELETE FROM change_log_entries WHERE kind IN ('series', 'person', 'genre', 'tag', 'publisher', 'list', 'user_book', 'book_left')`,
				`DROP INDEX IF EXISTS ix_change_log_entries_library_id_seq`,
				`DROP INDEX IF EXISTS ux_change_log_entries_record`,
				`CREATE UNIQUE INDEX ux_change_log_entries_kind_record_id_user_id ON change_log_entries (kind, record_id, IFNULL(user_id, 0))`,
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
