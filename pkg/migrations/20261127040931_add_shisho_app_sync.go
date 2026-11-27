package migrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	// What the Shisho app syncs through: its Devices, the change log it keeps
	// its copy of the catalog and the user's own records from (ADR 0023), and
	// the journal it sends its changes through (ADR 0024).
	//
	// The change log keeps one entry per record the app sees, so a change
	// moves the record's entry to the next sequence number: the old entry is
	// deleted and a new one inserted, and AUTOINCREMENT never reuses a seq.
	// Each entry records the Library it belongs to, or for the user's own
	// records the user, when written, because a deleted row, or a cascade's
	// parent, can't be looked up afterwards.
	//
	// Update triggers fire only when a compared column differs, so a Scan
	// that rewrites rows with the values they hold records nothing. A child's
	// change moves its parent only while the parent still exists, so a
	// cascade from the parent's delete never overwrites the parent's deletion
	// entry. change_log_invariants_test.go checks that every table and column
	// is either compared or left out with a reason.
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
	// moveLibraries moves the entries of the Libraries whose ids the
	// expressions list, skipping any that no longer exist.
	moveLibraries := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'library' AND record_id IN (SELECT id FROM libraries WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'library', id AS record_id, id AS library_id, FALSE, %[2]s FROM libraries WHERE id IN (%[1]s);`, ids, now)
	}
	// moveLists moves the entries of the Lists whose ids the expressions
	// list, each under its owner, skipping any that no longer exist.
	moveLists := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'list' AND record_id IN (SELECT id FROM lists WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'list', id, user_id, FALSE, %[2]s FROM lists WHERE id IN (%[1]s);`, ids, now)
	}
	// moveReadThroughs moves the entries of the Read-throughs whose ids the
	// expressions list, each under its user, skipping any that no longer
	// exist or are null.
	moveReadThroughs := func(ids string) string {
		return fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'read_through' AND record_id IN (SELECT id FROM read_throughs WHERE id IN (%[1]s));
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT 'read_through', id, user_id, FALSE, %[2]s FROM read_throughs WHERE id IN (%[1]s);`, ids, now)
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
	// fileBook is the Book of the File a row names.
	fileBook := func(row string) string {
		return fmt.Sprintf("SELECT book_id FROM files WHERE id = %s.file_id", row)
	}
	// bookPart is a table whose rows are part of a Book's document: book
	// names the Book of a row, and join joins two of them.
	bookPart := func(table string, book func(row string) string, join string, compared ...string) []string {
		return []string{
			trigger(table+"_insert", "INSERT", table, "", moveBooks(book("NEW"))),
			trigger(table+"_update", "UPDATE", table, differs(compared...), moveBooks(book("OLD")+join+book("NEW"))),
			trigger(table+"_delete", "DELETE", table, "", moveBooks(book("OLD"))),
		}
	}
	bookColumn := func(row string) string { return row + ".book_id" }
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
	// listPart is a table whose rows are part of a List's document.
	listPart := func(table string, compared ...string) []string {
		return []string{
			trigger(table+"_insert", "INSERT", table, "", moveLists("NEW.list_id")),
			trigger(table+"_update", "UPDATE", table, differs(compared...), moveLists("OLD.list_id, NEW.list_id")),
			trigger(table+"_delete", "DELETE", table, "", moveLists("OLD.list_id")),
		}
	}
	// own is one of the user's own records, keyed by key.
	own := func(kind, table, key string, compared ...string) []string {
		return []string{
			trigger(table+"_insert", "INSERT", table, "", recordOwn(kind, "NEW."+key, "NEW.user_id", false)),
			trigger(table+"_update", "UPDATE", table, differs(compared...), recordOwn(kind, "NEW."+key, "NEW.user_id", false)),
			trigger(table+"_delete", "DELETE", table, "", recordOwn(kind, "OLD."+key, "OLD.user_id", true)),
		}
	}

	statements := []string{
		// A Shisho app Device records the installation that signed in and
		// the app version it last reported. The installation id is made when
		// the app is installed and kept across signing out and back in, so
		// the journal can recognize events an installation resends after
		// signing in again as a new Device. Other Device types leave both
		// NULL.
		`ALTER TABLE devices ADD COLUMN installation_id TEXT`,
		`ALTER TABLE devices ADD COLUMN app_version TEXT`,

		// What the journal needs to place the app's changes by their own
		// time, against changes made elsewhere:
		//
		//   - user_books.page_flow_updated_at is when the Page flow was last
		//     chosen, as a Rating's time is for a Rating.
		//   - user_books.to_read_changed_at is when the Book last joined,
		//     left, or moved on To Read, so the latest change of each Book
		//     wins.
		//   - list_books.moved_at is when the Book was last placed in its
		//     List, by adding or moving it, for the same rule there. NULL is
		//     when it was added.
		//   - read_throughs.began_at and state_changed_at are when the pass
		//     began, and when it last paused, resumed, or ended, by when that
		//     happened rather than when the server heard: a place the app
		//     sends late starts a pass dated by the reading. updated_at stays
		//     the server's clock, which KOReader sync finds changes by. NULL
		//     is created_at and updated_at.
		//   - read_throughs.history_only marks a finish or abandon made
		//     offline before a later Reference choice (ADR 0028): an entry in
		//     Reading History that never takes Reading Sessions.
		`ALTER TABLE user_books ADD COLUMN page_flow_updated_at TIMESTAMP`,
		`ALTER TABLE user_books ADD COLUMN to_read_changed_at TIMESTAMP`,
		// A flow chosen on the web before this was chosen no later than its
		// record last changed, so a choice the app made earlier and sends
		// late doesn't replace it.
		`UPDATE user_books SET page_flow_updated_at = updated_at WHERE page_flow IS NOT NULL`,
		`ALTER TABLE list_books ADD COLUMN moved_at TIMESTAMP`,
		`ALTER TABLE read_throughs ADD COLUMN history_only BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE read_throughs ADD COLUMN began_at TIMESTAMP`,
		`ALTER TABLE read_throughs ADD COLUMN state_changed_at TIMESTAMP`,

		// A user's own records are entries with user_id set and no
		// library_id. A Reading Position has no id of its own, so its entry
		// is keyed by its File, and two users' positions in one File share a
		// record_id: the unique index covers the user too. A Book moved to
		// another Library leaves a book_left deletion in the Library it left,
		// so a user who can't access the new one drops it. Each Library a
		// Book left keeps its own, so a Book moved twice between an
		// installation's syncs still leaves the first: the unique index takes
		// in the Library for book_left.
		`CREATE TABLE change_log_entries (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			record_id INTEGER NOT NULL,
			library_id INTEGER,
			user_id INTEGER,
			deleted BOOLEAN NOT NULL DEFAULT FALSE,
			changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX ix_change_log_entries_deleted_changed_at ON change_log_entries (changed_at) WHERE deleted`,
		`CREATE UNIQUE INDEX ux_change_log_entries_record ON change_log_entries
			(kind, record_id, IFNULL(user_id, 0), CASE WHEN kind = 'book_left' THEN library_id ELSE 0 END)`,
		`CREATE INDEX ix_change_log_entries_user_id ON change_log_entries (user_id) WHERE user_id IS NOT NULL`,
		// Each page says which Libraries have records behind its cursor.
		`CREATE INDEX ix_change_log_entries_library_id_seq ON change_log_entries (library_id, seq)`,
		// The highest seq whose deletion entries have expired. An
		// installation whose cursor is below it may have missed a deletion.
		`CREATE TABLE change_log_expiries (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			expired_through INTEGER NOT NULL,
			expired_at TIMESTAMP NOT NULL
		)`,

		// app_journal_events records each event the server took in, by user,
		// installation, and the event's own id, so a resent event is applied
		// once even after the user deleted what it produced, and even after
		// the installation signed in again as a new Device.
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
		// These link the id an installation made a Bookmark or a List under
		// to it, so its later events can name it before the feed brings the
		// server's id, and the feed can tell the installation it's its own.
		// A deleted one keeps its row, with no Bookmark or List, so a create
		// sent again doesn't bring it back. A row is written in the
		// transaction that makes its record and never changes after, so the
		// record's own entry carries it.
		`CREATE TABLE app_journal_bookmarks (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			local_id TEXT NOT NULL,
			bookmark_id INTEGER REFERENCES bookmarks (id) ON DELETE SET NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, local_id)
		)`,
		`CREATE INDEX ix_app_journal_bookmarks_bookmark_id ON app_journal_bookmarks (bookmark_id)`,
		`CREATE TABLE app_journal_lists (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			local_id TEXT NOT NULL,
			list_id INTEGER REFERENCES lists (id) ON DELETE SET NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, local_id)
		)`,
		`CREATE INDEX ix_app_journal_lists_list_id ON app_journal_lists (list_id)`,

		// Library: its name and cover aspect ratio. The ratio decides which
		// File's cover each of its Books shows, so it moves them too.
		trigger("libraries_insert", "INSERT", "libraries", "", record("library", "NEW.id", "NEW.id", false)),
		trigger("libraries_update", "UPDATE", "libraries", differs("name", "cover_aspect_ratio", "tag_namespaces"), record("library", "NEW.id", "NEW.id", false)),
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
		trigger("books_update", "UPDATE", "books",
			differs("title", "subtitle", "sort_title", "created_at", "library_id", "description"),
			record("book", "NEW.id", "NEW.library_id", false)),
		trigger("books_update_library", "UPDATE", "books", differs("library_id"),
			fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'book_left' AND record_id = OLD.id AND library_id = OLD.library_id;
				INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
				VALUES ('book_left', OLD.id, OLD.library_id, TRUE, %s);`, now)),
		trigger("books_delete", "DELETE", "books", "", record("book", "OLD.id", "OLD.library_id", true)),

		// Files: everything the app shows of a File, or sorts or picks a
		// cover by, including updated_at, which the cover version is made
		// of.
		trigger("files_insert", "INSERT", "files", "", moveBooks("NEW.book_id")),
		trigger("files_update", "UPDATE", "files",
			differs("book_id", "file_type", "file_role", "filesize_bytes", "name", "name_source", "filepath",
				"release_date", "page_count", "audiobook_duration_seconds",
				"cover_image_filename", "is_preferred_cover", "updated_at", "publisher_id",
				"audiobook_codec", "audiobook_bitrate_bps", "language", "abridged", "url", "reading_direction",
				"file_modified_at", "created_at", "word_count"),
			moveBooks("OLD.book_id, NEW.book_id")),
		trigger("files_delete", "DELETE", "files", "", moveBooks("OLD.book_id")),
	}
	statements = append(statements, bookPart("authors", bookColumn, ", ", "book_id", "person_id", "sort_order", "role")...)
	statements = append(statements, bookPart("book_series", bookColumn, ", ",
		"book_id", "series_id", "series_number", "series_number_end", "series_number_unit", "sort_order")...)
	statements = append(statements, bookPart("book_genres", bookColumn, ", ", "book_id", "genre_id")...)
	statements = append(statements, bookPart("book_tags", bookColumn, ", ", "book_id", "tag_id")...)
	// A File's chapters, narrators, and identifiers are part of its Book. A
	// File's delete cascades to them after the File's row is gone, so they
	// find no Book to move; the File's own trigger moved it.
	statements = append(statements, bookPart("chapters", fileBook, " UNION ",
		"file_id", "parent_id", "sort_order", "title", "start_page", "start_timestamp_ms", "href")...)
	statements = append(statements, bookPart("narrators", fileBook, " UNION ", "file_id", "person_id", "sort_order")...)
	statements = append(statements, bookPart("file_identifiers", fileBook, " UNION ", "file_id", "type", "value")...)

	// Series, Persons, Genres, Tags, and Publishers are catalog records of
	// their own, with their aliases.
	statements = append(statements, resource("series", "series", "series_aliases", "series_id", "name", "sort_name", "description")...)
	statements = append(statements, resource("person", "persons", "person_aliases", "person_id", "name", "sort_name")...)
	statements = append(statements, resource("genre", "genres", "genre_aliases", "genre_id", "name")...)
	statements = append(statements, resource("tag", "tags", "tag_aliases", "tag_id", "name")...)
	statements = append(statements, resource("publisher", "publishers", "publisher_aliases", "publisher_id", "name", "parent_id")...)
	statements = append(statements,
		// Each one's name is also part of each Book that names it. Adding or
		// deleting one reaches the Books through their join tables.
		trigger("persons_update", "UPDATE", "persons", differs("name", "sort_name"),
			moveBooks("SELECT book_id FROM authors WHERE person_id = NEW.id")),
		trigger("persons_update_narrators", "UPDATE", "persons", differs("name", "sort_name"),
			moveBooks("SELECT f.book_id FROM narrators n JOIN files f ON f.id = n.file_id WHERE n.person_id = NEW.id")),
		trigger("series_update", "UPDATE", "series", differs("name", "sort_name"),
			moveBooks("SELECT book_id FROM book_series WHERE series_id = NEW.id")),
		trigger("genres_update", "UPDATE", "genres", differs("name"),
			moveBooks("SELECT book_id FROM book_genres WHERE genre_id = NEW.id")),
		trigger("tags_update", "UPDATE", "tags", differs("name"),
			moveBooks("SELECT book_id FROM book_tags WHERE tag_id = NEW.id")),
		trigger("publishers_update", "UPDATE", "publishers", differs("name"),
			moveBooks("SELECT book_id FROM files WHERE publisher_id = NEW.id")),

		// A List's entry carries its owner as user_id, so the owner sees it
		// as one of their own records. Everyone it's shared with sees it
		// through the feed's List Shares filter, and learns of a List deleted
		// or unshared from the access each page lists, since its List Shares
		// are gone. Its Books and List Shares are part of its document, since
		// the document says what each user may do with it. A List's delete
		// cascades to them with the List already gone, so the cascade writes
		// nothing.
		trigger("lists_insert", "INSERT", "lists", "", moveLists("NEW.id")),
		trigger("lists_update", "UPDATE", "lists", differs("user_id", "name", "description", "is_ordered", "default_sort"),
			moveLists("NEW.id")),
		trigger("lists_delete", "DELETE", "lists", "",
			fmt.Sprintf(`DELETE FROM change_log_entries WHERE kind = 'list' AND record_id = OLD.id;
				INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
				VALUES ('list', OLD.id, OLD.user_id, TRUE, %s);`, now)),
	)
	statements = append(statements, listPart("list_books", "list_id", "book_id", "added_at", "sort_order")...)
	statements = append(statements, listPart("list_shares", "list_id", "user_id", "permission")...)

	// The user's own records: the record for each Book, holding Rating, To
	// Read, Reference, and Page flow, keyed by the Book; Reading Positions,
	// keyed by their File; Read-throughs; and Bookmarks.
	statements = append(statements, own("user_book", "user_books", "book_id",
		"user_id", "book_id", "rating", "to_read_position", "reference", "page_flow")...)
	statements = append(statements, own("reading_position", "reading_positions", "file_id",
		"user_id", "file_id", "cfi", "xpointer", "page", "seconds", "kobo_span", "percentage",
		"source", "source_device_id", "occurred_at")...)
	statements = append(statements, own("read_through", "read_throughs", "id",
		"user_id", "book_id", "begin_day", "end_day", "outcome", "paused")...)
	statements = append(statements, own("bookmark", "bookmarks", "id", "user_id", "file_id", "seconds", "note", "created_at")...)
	statements = append(statements,
		// A Read-through's document carries when it was last read, from its
		// position updates, so an update moves it.
		trigger("reading_position_updates_insert", "INSERT", "reading_position_updates", "NEW.read_through_id IS NOT NULL",
			moveReadThroughs("NEW.read_through_id")),
		trigger("reading_position_updates_update", "UPDATE", "reading_position_updates",
			differs("read_through_id", "file_id", "occurred_at", "opened"),
			moveReadThroughs("OLD.read_through_id, NEW.read_through_id")),
		trigger("reading_position_updates_delete", "DELETE", "reading_position_updates", "OLD.read_through_id IS NOT NULL",
			moveReadThroughs("OLD.read_through_id")),

		// A Reading Position names the Device that sent it, so the app can
		// say where another device's place came from. Renaming a Device moves
		// the entries of its positions, so the app gets the new name.
		// Deleting one clears the positions' source_device_id, which their
		// own update trigger compares.
		`CREATE TRIGGER change_log_devices_update AFTER UPDATE ON devices WHEN OLD.name IS NOT NEW.name BEGIN
			DELETE FROM change_log_entries WHERE kind = 'reading_position' AND EXISTS (
				SELECT 1 FROM reading_positions rp WHERE rp.source_device_id = NEW.id
				AND rp.file_id = change_log_entries.record_id AND rp.user_id = change_log_entries.user_id);
			INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
				SELECT 'reading_position', file_id, user_id, FALSE, CURRENT_TIMESTAMP
				FROM reading_positions WHERE source_device_id = NEW.id ORDER BY occurred_at;
		END`,

		// Everything there is now has an entry, so an installation syncing
		// from the start gets the whole catalog and the user's own records.
		`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT 'library', id AS record_id, id AS library_id, FALSE, CURRENT_TIMESTAMP FROM libraries ORDER BY id`,
	)
	for _, catalog := range []struct{ kind, table string }{
		{"series", "series"}, {"person", "persons"}, {"genre", "genres"}, {"tag", "tags"},
		{"publisher", "publishers"}, {"book", "books"},
	} {
		statements = append(statements, fmt.Sprintf(`INSERT INTO change_log_entries (kind, record_id, library_id, deleted, changed_at)
			SELECT '%s', id, library_id, FALSE, CURRENT_TIMESTAMP FROM %s ORDER BY id`, catalog.kind, catalog.table))
	}
	for _, mine := range []struct{ kind, key, table, order string }{
		{"list", "id", "lists", "id"},
		{"user_book", "book_id", "user_books", "user_id, book_id"},
		{"read_through", "id", "read_throughs", "id"},
		{"reading_position", "file_id", "reading_positions", "occurred_at"},
		{"bookmark", "id", "bookmarks", "id"},
	} {
		statements = append(statements, fmt.Sprintf(`INSERT INTO change_log_entries (kind, record_id, user_id, deleted, changed_at)
			SELECT '%s', %s, user_id, FALSE, CURRENT_TIMESTAMP FROM %s ORDER BY %s`, mine.kind, mine.key, mine.table, mine.order))
	}

	run := func(ctx context.Context, tx bun.Tx, statements ...string) error {
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return errors.Wrapf(err, "run %s", statement)
			}
		}
		return nil
	}

	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// Bun records a migration after it commits, so a server stopped
			// between the two runs it again on its next start. It's all one
			// transaction, so the change log being there means it all ran.
			var n int
			err := tx.NewRaw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'change_log_entries'").Scan(ctx, &n)
			if err != nil {
				return errors.WithStack(err)
			}
			if n > 0 {
				return nil
			}
			return run(ctx, tx, statements...)
		})
	}

	down := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			// The triggers go first: SQLite won't drop a column a trigger
			// reads.
			var triggers []string
			if err := tx.NewRaw(`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'change\_log\_%' ESCAPE '\'`).Scan(ctx, &triggers); err != nil {
				return errors.WithStack(err)
			}
			for _, name := range triggers {
				if err := run(ctx, tx, "DROP TRIGGER "+name); err != nil {
					return err
				}
			}
			return run(ctx, tx,
				`DROP TABLE IF EXISTS app_journal_lists`,
				`DROP TABLE IF EXISTS app_journal_bookmarks`,
				`DROP TABLE IF EXISTS app_journal_sessions`,
				`DROP TABLE IF EXISTS app_journal_events`,
				`DROP TABLE IF EXISTS change_log_expiries`,
				`DROP TABLE IF EXISTS change_log_entries`,
				`ALTER TABLE read_throughs DROP COLUMN state_changed_at`,
				`ALTER TABLE read_throughs DROP COLUMN began_at`,
				`ALTER TABLE read_throughs DROP COLUMN history_only`,
				`ALTER TABLE list_books DROP COLUMN moved_at`,
				`ALTER TABLE user_books DROP COLUMN to_read_changed_at`,
				`ALTER TABLE user_books DROP COLUMN page_flow_updated_at`,
				`DELETE FROM devices WHERE device_type = 'shisho_app'`,
				`ALTER TABLE devices DROP COLUMN app_version`,
				`ALTER TABLE devices DROP COLUMN installation_id`,
			)
		})
	}

	Migrations.MustRegister(up, down)
}
