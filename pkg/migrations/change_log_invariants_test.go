package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// These tests keep the change log (ADR 0023) complete. Every table is either
// in feedTables, whose triggers record its changes, or in tablesNotInFeed
// with the reason the Shisho app doesn't need it. Every column of a table in
// feedTables is either compared by an update trigger or in
// columnsNotCompared with a reason, so a new column can't silently stay out
// of the app's copy. Update triggers compare only, so a write that changes
// nothing records nothing.

// feedTables are the tables with change log triggers. events lists the
// writes whose triggers each needs: a table whose rows are part of a
// document needs all three, and a table a document only names (a Person's
// name, say) needs its updates, since its rows reach the document through
// a join table.
var feedTables = map[string][]string{
	"libraries":   {"INSERT", "UPDATE", "DELETE"},
	"books":       {"INSERT", "UPDATE", "DELETE"},
	"files":       {"INSERT", "UPDATE", "DELETE"},
	"authors":     {"INSERT", "UPDATE", "DELETE"},
	"book_series": {"INSERT", "UPDATE", "DELETE"},
	"persons":     {"INSERT", "UPDATE", "DELETE"},
	"series":      {"INSERT", "UPDATE", "DELETE"},
	// A user's saved sort for a Library is part of the Library's document.
	"user_library_settings": {"INSERT", "UPDATE", "DELETE"},
	// A File's chapters and narrators are part of its Book's document.
	"chapters":  {"INSERT", "UPDATE", "DELETE"},
	"narrators": {"INSERT", "UPDATE", "DELETE"},
	// Catalog records of their own, and their aliases, which are part of
	// their documents.
	"genres":            {"INSERT", "UPDATE", "DELETE"},
	"tags":              {"INSERT", "UPDATE", "DELETE"},
	"publishers":        {"INSERT", "UPDATE", "DELETE"},
	"person_aliases":    {"INSERT", "UPDATE", "DELETE"},
	"series_aliases":    {"INSERT", "UPDATE", "DELETE"},
	"genre_aliases":     {"INSERT", "UPDATE", "DELETE"},
	"tag_aliases":       {"INSERT", "UPDATE", "DELETE"},
	"publisher_aliases": {"INSERT", "UPDATE", "DELETE"},
	// A Book's genres and tags, and its Files' identifiers.
	"book_genres":      {"INSERT", "UPDATE", "DELETE"},
	"book_tags":        {"INSERT", "UPDATE", "DELETE"},
	"file_identifiers": {"INSERT", "UPDATE", "DELETE"},
	// Lists, with their Books and the List Shares that say who sees them.
	"lists":       {"INSERT", "UPDATE", "DELETE"},
	"list_books":  {"INSERT", "UPDATE", "DELETE"},
	"list_shares": {"INSERT", "UPDATE", "DELETE"},
	// The user's own records.
	"reading_positions": {"INSERT", "UPDATE", "DELETE"},
	"read_throughs":     {"INSERT", "UPDATE", "DELETE"},
	// A Read-through's document carries when it was last read.
	"reading_position_updates": {"INSERT", "UPDATE", "DELETE"},
	"user_books":               {"INSERT", "UPDATE", "DELETE"},
	"bookmarks":                {"INSERT", "UPDATE", "DELETE"},
}

const (
	reasonPrimaryKey = "the primary key, which a row never changes"
	reasonBookkeep   = "bookkeeping the app doesn't show"
	reasonSource     = "provenance, which the app doesn't show"
	reasonNotYet     = "not in the app's documents yet"
	reasonLibrary    = "the Library is read from the Book"
	// An alias belongs to its parent's Library, which never changes.
	reasonAliasLibrary = "the Library is its parent's"
)

// columnsNotCompared are the columns of feedTables that no update trigger
// compares, as "table.column".
var columnsNotCompared = map[string]string{
	"libraries.id":                      reasonPrimaryKey,
	"libraries.created_at":              reasonBookkeep,
	"libraries.updated_at":              reasonBookkeep,
	"libraries.organize_file_structure": "a server-side setting",
	"libraries.tag_namespaces":          reasonNotYet,

	"books.id":                 reasonPrimaryKey,
	"books.updated_at":         reasonBookkeep,
	"books.filepath":           "where the server keeps the Book",
	"books.title_source":       reasonSource,
	"books.sort_title_source":  reasonSource,
	"books.subtitle_source":    reasonSource,
	"books.description_source": reasonSource,
	"books.genre_source":       reasonSource,
	"books.tag_source":         reasonSource,
	"books.author_source":      reasonSource,
	"books.series_source":      reasonSource,
	"books.sidecars_stale_at":  "server-side sidecar bookkeeping",

	"files.id":                       reasonPrimaryKey,
	"files.library_id":               reasonLibrary,
	"files.cover_mime_type":          "changes only with the cover, which updated_at covers",
	"files.cover_source":             reasonSource,
	"files.cover_page":               "changes only with the cover, which updated_at covers",
	"files.narrator_source":          reasonSource,
	"files.identifier_source":        reasonSource,
	"files.url_source":               reasonSource,
	"files.release_date_source":      reasonSource,
	"files.publisher_source":         reasonSource,
	"files.chapter_source":           reasonSource,
	"files.language_source":          reasonSource,
	"files.abridged_source":          reasonSource,
	"files.review_override":          "the review workflow, which only the web app shows",
	"files.review_overridden_at":     "the review workflow, which only the web app shows",
	"files.reviewed":                 "the review workflow, which only the web app shows",
	"files.scan_error":               "a server-side scan problem",
	"files.reading_direction_source": reasonSource,

	"authors.id":     reasonPrimaryKey,
	"book_series.id": reasonPrimaryKey,

	"persons.id":               reasonPrimaryKey,
	"persons.created_at":       reasonBookkeep,
	"persons.updated_at":       reasonBookkeep,
	"persons.library_id":       "a Person never moves between Libraries",
	"persons.sort_name_source": reasonSource,

	"series.id":                   reasonPrimaryKey,
	"series.created_at":           reasonBookkeep,
	"series.updated_at":           reasonBookkeep,
	"series.library_id":           "a Series never moves between Libraries",
	"series.name_source":          reasonSource,
	"series.sort_name_source":     reasonSource,
	"series.cover_image_filename": "the app shows a Series by its Books' covers",

	"user_library_settings.id":         reasonPrimaryKey,
	"user_library_settings.created_at": reasonBookkeep,
	"user_library_settings.updated_at": reasonBookkeep,

	"chapters.id":         reasonPrimaryKey,
	"narrators.id":        reasonPrimaryKey,
	"chapters.created_at": reasonBookkeep,
	"chapters.updated_at": reasonBookkeep,

	"reading_positions.created_at":         reasonBookkeep,
	"reading_positions.updated_at":         "bookkeeping; occurred_at says when the reader was there",
	"reading_positions.locator_file_type":  "changes only with the locator, which is compared",
	"reading_positions.chapter_percentage": "a Kobo's fallback for placing its span, which the app doesn't read",
	"reading_positions.reported_device":    "how KOReader named itself, which only KOReader reads back",
	"reading_positions.reported_device_id": "how KOReader named itself, which only KOReader reads back",

	"genres.id":             reasonPrimaryKey,
	"genres.created_at":     reasonBookkeep,
	"genres.updated_at":     reasonBookkeep,
	"genres.library_id":     "a Genre never moves between Libraries",
	"tags.id":               reasonPrimaryKey,
	"tags.created_at":       reasonBookkeep,
	"tags.updated_at":       reasonBookkeep,
	"tags.library_id":       "a Tag never moves between Libraries",
	"publishers.id":         reasonPrimaryKey,
	"publishers.created_at": reasonBookkeep,
	"publishers.updated_at": reasonBookkeep,
	"publishers.library_id": "a Publisher never moves between Libraries",

	"person_aliases.id":            reasonPrimaryKey,
	"person_aliases.created_at":    reasonBookkeep,
	"person_aliases.library_id":    reasonAliasLibrary,
	"series_aliases.id":            reasonPrimaryKey,
	"series_aliases.created_at":    reasonBookkeep,
	"series_aliases.library_id":    reasonAliasLibrary,
	"genre_aliases.id":             reasonPrimaryKey,
	"genre_aliases.created_at":     reasonBookkeep,
	"genre_aliases.library_id":     reasonAliasLibrary,
	"tag_aliases.id":               reasonPrimaryKey,
	"tag_aliases.created_at":       reasonBookkeep,
	"tag_aliases.library_id":       reasonAliasLibrary,
	"publisher_aliases.id":         reasonPrimaryKey,
	"publisher_aliases.created_at": reasonBookkeep,
	"publisher_aliases.library_id": reasonAliasLibrary,

	"book_genres.id":              reasonPrimaryKey,
	"book_tags.id":                reasonPrimaryKey,
	"file_identifiers.id":         reasonPrimaryKey,
	"file_identifiers.created_at": reasonBookkeep,
	"file_identifiers.updated_at": reasonBookkeep,
	"file_identifiers.source":     reasonSource,

	"lists.id":                      reasonPrimaryKey,
	"lists.created_at":              reasonBookkeep,
	"lists.updated_at":              reasonBookkeep,
	"list_books.id":                 reasonPrimaryKey,
	"list_books.added_by_user_id":   "who added a Book, which only the web app shows",
	"list_shares.id":                reasonPrimaryKey,
	"list_shares.created_at":        reasonBookkeep,
	"list_shares.shared_by_user_id": "who shared the List, which only the web app shows",

	"reading_position_updates.id":               reasonPrimaryKey,
	"reading_position_updates.created_at":       reasonBookkeep,
	"reading_position_updates.user_id":          "an update never moves between users",
	"reading_position_updates.cfi":              reasonLastReadOnly,
	"reading_position_updates.xpointer":         reasonLastReadOnly,
	"reading_position_updates.page":             reasonLastReadOnly,
	"reading_position_updates.seconds":          reasonLastReadOnly,
	"reading_position_updates.kobo_span":        reasonLastReadOnly,
	"reading_position_updates.percentage":       reasonLastReadOnly,
	"reading_position_updates.source":           reasonLastReadOnly,
	"reading_position_updates.at_end":           reasonLastReadOnly,
	"reading_position_updates.auto_finished":    reasonLastReadOnly,
	"reading_position_updates.source_device_id": reasonLastReadOnly,

	"user_books.created_at":        reasonBookkeep,
	"user_books.updated_at":        reasonBookkeep,
	"user_books.rating_updated_at": "orders late Ratings on the server, which the app doesn't read",
	"user_books.status_chosen_at":  "orders late journal events on the server, which the app doesn't read",
	"user_books.page_flow":         reasonNotYet,

	"read_throughs.id":                    reasonPrimaryKey,
	"read_throughs.created_at":            reasonBookkeep,
	"read_throughs.updated_at":            reasonBookkeep,
	"read_throughs.book_title":            "set only when its Book is deleted, which clears book_id",
	"read_throughs.book_authors":          "set only when its Book is deleted, which clears book_id",
	"read_throughs.left_to_read_position": "the To Read place a delete restores, which only the server uses",

	"bookmarks.id":         reasonPrimaryKey,
	"bookmarks.updated_at": "when the note last changed, which only the server compares",
}

const (
	reasonServer   = "server-side state the app never shows"
	reasonPlugins  = "plugin configuration, managed only on the web"
	reasonDevices  = "Device and sync state for other Device types"
	reasonAccounts = "accounts and access, answered outside the cursor"
	reasonUserNext = "a user's own records, which the feed doesn't carry yet"
	// reasonLastReadOnly: a Read-through takes only when its updates were.
	reasonLastReadOnly = "a Read-through's document takes only when its updates were"
	reasonFTS          = "a search index the server rebuilds from the catalog"
)

// tablesNotInFeed are the tables without change log triggers, each with why.
var tablesNotInFeed = map[string]string{
	"app_settings":                  reasonServer,
	"bun_migration_locks":           "created by Bun's migrator",
	"bun_migrations":                "created by Bun's migrator",
	"change_log_entries":            "the change log itself",
	"change_log_expiries":           "the change log's own bookkeeping",
	"app_journal_events":            "which Shisho app journal events were taken in, for ignoring repeats",
	"app_journal_sessions":          "which Reading Session each Shisho app sitting's snapshots update",
	"app_journal_bookmarks":         "written with its Bookmark and never changed, so the Bookmark's entry carries it",
	"jobs":                          reasonServer,
	"job_logs":                      reasonServer,
	"library_paths":                 reasonServer,
	"file_fingerprints":             reasonServer,
	"plugins":                       reasonPlugins,
	"plugin_configs":                reasonPlugins,
	"plugin_field_settings":         reasonPlugins,
	"plugin_hook_configs":           reasonPlugins,
	"plugin_identifier_types":       reasonPlugins,
	"plugin_repositories":           reasonPlugins,
	"library_plugin_customizations": reasonPlugins,
	"library_plugin_field_settings": reasonPlugins,
	"library_plugin_hook_configs":   reasonPlugins,
	"devices":                       reasonDevices,
	"device_delivery_rules":         reasonDevices,
	"device_short_urls":             reasonDevices,
	"kobo_sync_entries":             reasonDevices,
	"kobo_sync_point_books":         reasonDevices,
	"kobo_sync_points":              reasonDevices,
	"koreader_document_hashes":      reasonDevices,
	"koreader_document_links":       reasonDevices,
	"koreader_held_events":          reasonDevices,
	"koreader_page_rows":            reasonDevices,
	"koreader_plugin_events":        reasonDevices,
	"koreader_pulls":                reasonDevices,
	"koreader_unrecognized_books":   reasonDevices,
	"users":                         reasonAccounts,
	"roles":                         reasonAccounts,
	"permissions":                   reasonAccounts,
	"user_library_access":           reasonAccounts,
	"invite_links":                  reasonAccounts,
	"invite_link_libraries":         reasonAccounts,
	"share_links":                   "Share Links, managed only on the web",
	"user_delivery_rules":           "Delivery Rules, which the Shisho app doesn't follow",
	"user_settings":                 "web display settings",
	"reading_sessions":              reasonUserNext,
	"books_fts":                     reasonFTS,
	"genres_fts":                    reasonFTS,
	"persons_fts":                   reasonFTS,
	"publishers_fts":                reasonFTS,
	"series_fts":                    reasonFTS,
	"tags_fts":                      reasonFTS,
}

type changeLogTrigger struct {
	name, table, event, sql string
}

func listChangeLogTriggers(ctx context.Context, t *testing.T, db *bun.DB) []changeLogTrigger {
	t.Helper()
	rows, err := db.QueryContext(ctx,
		`SELECT name, tbl_name, sql FROM sqlite_master WHERE type = 'trigger' AND name LIKE 'change\_log\_%' ESCAPE '\' ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()
	eventPattern := regexp.MustCompile(`(?i)\bAFTER\s+(INSERT|UPDATE|DELETE)\b`)
	var triggers []changeLogTrigger
	for rows.Next() {
		var tr changeLogTrigger
		require.NoError(t, rows.Scan(&tr.name, &tr.table, &tr.sql))
		match := eventPattern.FindStringSubmatch(tr.sql)
		require.NotNil(t, match, "trigger %s has no AFTER INSERT, UPDATE, or DELETE", tr.name)
		tr.event = strings.ToUpper(match[1])
		triggers = append(triggers, tr)
	}
	require.NoError(t, rows.Err())
	return triggers
}

var (
	whenPattern    = regexp.MustCompile(`(?is)\bWHEN\s+(.*?)\s+BEGIN\b`)
	comparePattern = regexp.MustCompile(`^OLD\.(\w+) IS NOT NEW\.(\w+)$`)
)

// comparedColumns returns the columns an update trigger's WHEN clause
// compares, or a message when the clause is anything but comparisons of
// OLD and NEW joined by OR, which could fire for a write that changes
// nothing.
func comparedColumns(tr changeLogTrigger) ([]string, string) {
	match := whenPattern.FindStringSubmatch(tr.sql)
	if match == nil {
		return nil, fmt.Sprintf("update trigger %s has no WHEN clause, so it fires for writes that change nothing", tr.name)
	}
	var columns []string
	for _, term := range strings.Split(match[1], " OR ") {
		m := comparePattern.FindStringSubmatch(strings.TrimSpace(term))
		if m == nil || m[1] != m[2] {
			return nil, fmt.Sprintf("update trigger %s compares %q; compare only OLD.column IS NOT NEW.column, joined by OR", tr.name, term)
		}
		columns = append(columns, m[1])
	}
	return columns, ""
}

// checkChangeLogCoverage returns one message per table or column that isn't
// classified, per missing trigger, and per update trigger that could fire
// without a change.
func checkChangeLogCoverage(ctx context.Context, t *testing.T, db *bun.DB, covered map[string][]string, notCompared, notInFeed map[string]string) []string {
	t.Helper()
	var problems []string
	triggers := listChangeLogTriggers(ctx, t, db)

	events := map[string][]string{}
	compared := map[string]map[string]bool{}
	for _, tr := range triggers {
		events[tr.table] = append(events[tr.table], tr.event)
		if tr.event != "UPDATE" {
			continue
		}
		columns, problem := comparedColumns(tr)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		if compared[tr.table] == nil {
			compared[tr.table] = map[string]bool{}
		}
		for _, column := range columns {
			compared[tr.table][column] = true
		}
	}

	seenTables := map[string]bool{}
	seenColumns := map[string]bool{}
	for _, table := range listTables(ctx, t, db) {
		seenTables[table.name] = true
		_, isCovered := covered[table.name]
		_, isExcluded := notInFeed[table.name]
		switch {
		case isCovered && isExcluded:
			problems = append(problems, fmt.Sprintf("table %s is both in feedTables and tablesNotInFeed", table.name))
		case !isCovered && !isExcluded:
			problems = append(problems, fmt.Sprintf(
				"table %s has no change log triggers: add triggers so the Shisho app gets its changes (ADR 0023), or add it to tablesNotInFeed with the reason the app doesn't need it",
				table.name))
		}
		if !isCovered {
			if len(events[table.name]) > 0 {
				problems = append(problems, fmt.Sprintf("table %s has change log triggers but isn't in feedTables", table.name))
			}
			continue
		}
		for _, event := range covered[table.name] {
			if !slices.Contains(events[table.name], event) {
				problems = append(problems, fmt.Sprintf("table %s has no change log trigger for %s", table.name, event))
			}
		}
		var columns []string
		require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info(?) ORDER BY cid", table.name).Scan(ctx, &columns))
		for _, column := range columns {
			key := table.name + "." + column
			seenColumns[key] = true
			_, excused := notCompared[key]
			switch {
			case compared[table.name][column] && excused:
				problems = append(problems, fmt.Sprintf("column %s is compared but also in columnsNotCompared", key))
			case !compared[table.name][column] && !excused:
				problems = append(problems, fmt.Sprintf(
					"column %s isn't compared by a change log trigger: compare it in a new migration that recreates the update trigger, or add it to columnsNotCompared with the reason the app doesn't need it",
					key))
			}
		}
	}
	for table := range notInFeed {
		if !seenTables[table] {
			problems = append(problems, fmt.Sprintf("tablesNotInFeed lists %s, which doesn't exist", table))
		}
	}
	for key := range notCompared {
		if !seenColumns[key] {
			problems = append(problems, fmt.Sprintf("columnsNotCompared lists %s, which doesn't exist", key))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestChangeLog_EveryTableAndColumnIsClassified(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	problems := checkChangeLogCoverage(context.Background(), t, db, feedTables, columnsNotCompared, tablesNotInFeed)
	assert.Empty(t, problems, strings.Join(problems, "\n"))
}

// TestChangeLog_WritesThatChangeNothingRecordNothing rewrites every column of
// every row in the covered tables with the value it holds, and checks the
// log is untouched.
func TestChangeLog_WritesThatChangeNothingRecordNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testdb.New(t)
	for _, stmt := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'Library', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source) VALUES (1, 1, '/b', 'Book', 'file', 'Book', 'file')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, filesize_bytes, cover_image_filename) VALUES (1, 1, 1, '/b/b.epub', 'epub', 10, 'b.epub.cover.jpg')`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'Ann Author', 'Author, Ann', 'file')`,
		`INSERT INTO authors (id, book_id, person_id, sort_order) VALUES (1, 1, 1, 1)`,
		`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source) VALUES (1, 1, 'Saga', 'file', 'Saga', 'file')`,
		`INSERT INTO book_series (id, book_id, series_id, series_number, sort_order) VALUES (1, 1, 1, 1, 1)`,
		`INSERT INTO chapters (id, file_id, sort_order, title, href) VALUES (1, 1, 0, 'One', 'one.xhtml')`,
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (1, 'reader', 'x', 1, TRUE)`,
		`INSERT INTO reading_positions (user_id, file_id, cfi, percentage, source, occurred_at) VALUES (1, 1, 'epubcfi(/6/2)', 0.5, 'web_reader', '2026-01-01 00:00:00')`,
		`INSERT INTO read_throughs (id, user_id, book_id, outcome, begin_day) VALUES (1, 1, 1, 'underway', '2026-01-01')`,
		`INSERT INTO person_aliases (person_id, name, library_id) VALUES (1, 'A. Author', 1)`,
		`INSERT INTO series_aliases (series_id, name, library_id) VALUES (1, 'The Saga', 1)`,
		`INSERT INTO genres (id, library_id, name) VALUES (1, 1, 'Drama')`,
		`INSERT INTO genre_aliases (genre_id, name, library_id) VALUES (1, 'Theatre', 1)`,
		`INSERT INTO book_genres (book_id, genre_id) VALUES (1, 1)`,
		`INSERT INTO tags (id, library_id, name) VALUES (1, 1, 'Sea')`,
		`INSERT INTO tag_aliases (tag_id, name, library_id) VALUES (1, 'Ocean', 1)`,
		`INSERT INTO book_tags (book_id, tag_id) VALUES (1, 1)`,
		`INSERT INTO publishers (id, library_id, name) VALUES (1, 1, 'Press')`,
		`INSERT INTO publisher_aliases (publisher_id, name, library_id) VALUES (1, 'The Press', 1)`,
		`UPDATE files SET publisher_id = 1 WHERE id = 1`,
		`INSERT INTO file_identifiers (file_id, type, value, source) VALUES (1, 'isbn_13', '9780000000000', 'file')`,
		`INSERT INTO lists (id, user_id, name) VALUES (1, 1, 'Favorites')`,
		`INSERT INTO list_books (list_id, book_id, sort_order) VALUES (1, 1, 1)`,
		`INSERT INTO users (id, username, password_hash, role_id, is_active) VALUES (2, 'friend', 'x', 1, TRUE)`,
		`INSERT INTO list_shares (list_id, user_id, permission) VALUES (1, 2, 'viewer')`,
		`INSERT INTO user_books (user_id, book_id, rating, to_read_position) VALUES (1, 1, 8, 1)`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}

	var before []string
	require.NoError(t, db.NewRaw("SELECT kind || ':' || record_id || ':' || seq FROM change_log_entries ORDER BY seq").Scan(ctx, &before))
	require.NotEmpty(t, before)

	for table := range feedTables {
		var columns []string
		require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info(?) ORDER BY cid", table).Scan(ctx, &columns))
		assignments := make([]string, len(columns))
		for i, column := range columns {
			assignments[i] = fmt.Sprintf("%[1]q = %[1]q", column)
		}
		_, err := db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s", table, strings.Join(assignments, ", ")))
		require.NoError(t, err, table)
	}

	var after []string
	require.NoError(t, db.NewRaw("SELECT kind || ':' || record_id || ':' || seq FROM change_log_entries ORDER BY seq").Scan(ctx, &after))
	assert.Equal(t, before, after)
}

func TestChangeLogCoverageChecker_CatchesViolations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	connector, err := sqliteconn.NewConnector(":memory:", "PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	sqldb := sql.OpenDB(connector)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqliteconn.NewDialect())
	t.Cleanup(func() { _ = db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE change_log_entries (seq INTEGER PRIMARY KEY, kind TEXT, record_id INTEGER)`,
		`CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT, color TEXT, size INTEGER)`,
		`CREATE TRIGGER change_log_widgets_insert AFTER INSERT ON widgets BEGIN SELECT 1; END`,
		// Compares name, but color is neither compared nor excused.
		`CREATE TRIGGER change_log_widgets_update AFTER UPDATE ON widgets WHEN OLD.name IS NOT NEW.name BEGIN SELECT 1; END`,
		// Fires on every write.
		`CREATE TABLE gadgets (id INTEGER PRIMARY KEY)`,
		`CREATE TRIGGER change_log_gadgets_update AFTER UPDATE ON gadgets BEGIN SELECT 1; END`,
		// Not classified at all.
		`CREATE TABLE gizmos (id INTEGER PRIMARY KEY)`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}

	problems := checkChangeLogCoverage(ctx, t, db,
		map[string][]string{"widgets": {"INSERT", "UPDATE", "DELETE"}, "gadgets": {"UPDATE"}},
		map[string]string{"widgets.id": "pk", "widgets.size": "excused", "gadgets.id": "pk", "widgets.weight": "gone"},
		map[string]string{"change_log_entries": "the log", "sprockets": "gone"},
	)
	joined := strings.Join(problems, "\n")
	assert.Contains(t, joined, "update trigger change_log_gadgets_update has no WHEN clause")
	assert.Contains(t, joined, "table gizmos has no change log triggers")
	assert.Contains(t, joined, "table widgets has no change log trigger for DELETE")
	assert.Contains(t, joined, "column widgets.color isn't compared")
	assert.Contains(t, joined, "columnsNotCompared lists widgets.weight, which doesn't exist")
	assert.Contains(t, joined, "tablesNotInFeed lists sprockets, which doesn't exist")
	assert.NotContains(t, joined, "widgets.size")
	assert.Len(t, problems, 6, joined)
}
