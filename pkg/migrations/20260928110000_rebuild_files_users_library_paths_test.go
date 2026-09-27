package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

const rebuildFKMigrationName = "20260928110000"

var rebuiltTables = []string{"files", "users", "library_paths"}

// Every table with a foreign key to files or users. The rebuild drops and
// renames the parent tables, so these rows must come through untouched.
var rebuiltTableChildren = []string{
	"chapters", "file_identifiers", "file_fingerprints", "narrators",
	"api_keys", "jobs", "lists", "list_books", "list_shares",
	"user_library_access", "user_library_settings", "user_settings",
}

type rebuildSnapshot struct {
	schemaObjects map[string]string
	rows          map[string][]string
	childCounts   map[string]int
	sequences     map[string]int
}

func openRebuildTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	// One connection, as in production, so every statement sees the same
	// in-memory database and the same PRAGMA state.
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	return db
}

// migrateBeforeRebuild applies every registered migration that sorts before
// the rebuild.
func migrateBeforeRebuild(ctx context.Context, t *testing.T, db *bun.DB) {
	t.Helper()
	set := migrate.NewMigrations()
	for _, m := range Migrations.Sorted() {
		if m.Name < rebuildFKMigrationName {
			set.Add(m)
		}
	}
	migrator := newMigrator(db, set)
	require.NoError(t, migrator.Init(ctx))
	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
}

func seedRebuildRows(ctx context.Context, t *testing.T, db *bun.DB) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'One', 'book'), (2, 'Two', 'book')`,
		`INSERT INTO library_paths (id, library_id, filepath) VALUES (1, 1, '/one'), (2, 2, '/two'), (3, 1, '/one-extra')`,
		`DELETE FROM library_paths WHERE id = 3`,

		`INSERT INTO roles (id, name) VALUES (4, 'custom')`,
		`INSERT INTO permissions (role_id, resource, operation) VALUES (4, 'books', 'read')`,
		`INSERT INTO users (id, username, email, password_hash, role_id, must_change_password)
			VALUES (1, 'admin', NULL, 'hash-a', 1, FALSE),
			       (2, 'reader', 'reader@example.com', 'hash-r', 4, TRUE),
			       (3, 'gone', NULL, 'hash-g', 2, FALSE)`,
		`DELETE FROM users WHERE id = 3`,
		`INSERT INTO api_keys (id, user_id, name, key, created_at, updated_at) VALUES ('k1', 2, 'key', 'secret', '2026-01-01', '2026-01-01')`,
		`INSERT INTO user_library_access (user_id, library_id) VALUES (2, 1)`,

		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (1, 1, '/one/a', 'A', 'filepath', 'A', 'filepath'),
			       (2, 2, '/two/b', 'B', 'filepath', 'B', 'filepath')`,
		// File 2 sits in library 1 while its Book lives in library 2, so only
		// files.library_id ties it to library 1.
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, name, page_count, is_preferred_cover, scan_error)
			VALUES (1, 1, 1, '/one/a/a.epub', 'epub', 'A file', 12, TRUE, NULL),
			       (2, 1, 2, '/one/stray.cbz', 'cbz', NULL, 3, FALSE, 'bad zip'),
			       (3, 2, 2, '/two/b/b.m4b', 'm4b', 'B file', NULL, FALSE, NULL),
			       (4, 2, 2, '/two/b/deleted.pdf', 'pdf', 'gone', 9, FALSE, NULL)`,
		`DELETE FROM files WHERE id = 4`,
		`INSERT INTO chapters (file_id, sort_order, title) VALUES (1, 0, 'One')`,
		`INSERT INTO file_identifiers (file_id, type, value, source) VALUES (1, 'isbn_13', '9780000000000', 'epub_metadata')`,
		`INSERT INTO file_fingerprints (file_id, algorithm, value) VALUES (3, 'sha256', 'abc')`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 2, 'Narrator', 'Narrator', 'filepath')`,
		`INSERT INTO narrators (file_id, person_id, sort_order) VALUES (3, 1, 0)`,
		`INSERT INTO jobs (type, status, data, progress, created_by_user_id) VALUES ('scan', 'completed', '{}', 100, 2)`,
		`INSERT INTO lists (id, user_id, name) VALUES (1, 2, 'Reading')`,
		`INSERT INTO list_books (list_id, book_id, added_by_user_id) VALUES (1, 1, 2)`,
		`INSERT INTO list_shares (list_id, user_id, permission, shared_by_user_id) VALUES (1, 1, 'viewer', 2)`,
		`INSERT INTO user_library_settings (user_id, library_id, sort_spec) VALUES (2, 1, 'title:asc')`,
		`INSERT INTO user_settings (user_id) VALUES (2)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
}

func takeRebuildSnapshot(ctx context.Context, t *testing.T, db *bun.DB) rebuildSnapshot {
	t.Helper()
	snap := rebuildSnapshot{
		schemaObjects: map[string]string{},
		rows:          map[string][]string{},
		childCounts:   map[string]int{},
		sequences:     map[string]int{},
	}

	var objects []struct {
		Type string         `bun:"type"`
		Name string         `bun:"name"`
		SQL  sql.NullString `bun:"sql"`
	}
	require.NoError(t, db.NewRaw(
		`SELECT type, name, sql FROM sqlite_master WHERE type IN ('index', 'trigger') AND tbl_name IN (?)`,
		bun.List(rebuiltTables),
	).Scan(ctx, &objects))
	for _, o := range objects {
		snap.schemaObjects[o.Type+" "+o.Name] = o.SQL.String
	}

	for _, table := range rebuiltTables {
		rows, err := db.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY id")
		require.NoError(t, err)
		cols, err := rows.Columns()
		require.NoError(t, err)
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			// fmt.Sprint compares values, not storage classes, so the
			// library_paths TEXT '1' and INTEGER 1 read the same.
			snap.rows[table] = append(snap.rows[table], fmt.Sprint(cols, values))
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())

		var seq int
		require.NoError(t, db.NewRaw("SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = ?), 0)", table).Scan(ctx, &seq))
		snap.sequences[table] = seq
	}

	for _, table := range rebuiltTableChildren {
		var count int
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM "+table).Scan(ctx, &count))
		snap.childCounts[table] = count
	}
	return snap
}

func assertRebuiltSchema(ctx context.Context, t *testing.T, db *bun.DB, before rebuildSnapshot, beforeColumns map[string][]string) {
	t.Helper()

	assert.Contains(t, foreignKeys(ctx, t, db, "files"), foreignKey{"library_id", "libraries", "id", "CASCADE"})
	assert.Contains(t, foreignKeys(ctx, t, db, "files"), foreignKey{"book_id", "books", "id", "CASCADE"})
	assert.Contains(t, foreignKeys(ctx, t, db, "files"), foreignKey{"publisher_id", "publishers", "id", "SET NULL"})
	assert.Equal(t, []foreignKey{{"role_id", "roles", "id", "RESTRICT"}}, foreignKeys(ctx, t, db, "users"))
	assert.Equal(t, []foreignKey{{"library_id", "libraries", "id", "CASCADE"}}, foreignKeys(ctx, t, db, "library_paths"))

	typ, notNull := columnDecl(ctx, t, db, "library_paths", "library_id")
	assert.Equal(t, "INTEGER", typ)
	assert.True(t, notNull)
	typ, notNull = columnDecl(ctx, t, db, "files", "library_id")
	assert.Equal(t, "INTEGER", typ)
	assert.True(t, notNull)
	typ, notNull = columnDecl(ctx, t, db, "users", "role_id")
	assert.Equal(t, "INTEGER", typ)
	assert.True(t, notNull)

	var textIDs int
	require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM library_paths WHERE typeof(library_id) <> 'integer'").Scan(ctx, &textIDs))
	assert.Zero(t, textIDs, "library_paths.library_id values are stored as integers")

	after := takeRebuildSnapshot(ctx, t, db)
	for key, sqlText := range before.schemaObjects {
		if assert.Contains(t, after.schemaObjects, key, "schema object dropped by the rebuild") {
			assert.Equal(t, sqlText, after.schemaObjects[key], key)
		}
	}
	assert.Equal(t, before.rows, after.rows)
	assert.Equal(t, before.childCounts, after.childCounts)
	assert.Equal(t, before.sequences, after.sequences)
	for _, table := range rebuiltTables {
		assert.Equal(t, beforeColumns[table], columnNames(ctx, t, db, table), table)
	}

	assert.Zero(t, foreignKeyViolations(ctx, t, db))

	var fkEnabled int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &fkEnabled))
	assert.Equal(t, 1, fkEnabled, "the migration restores foreign key enforcement")
}

// The rebuild restores files.library_id's CASCADE, makes users.role_id
// RESTRICT, and retypes library_paths.library_id as INTEGER without losing
// rows, indexes, AUTOINCREMENT high-water marks, or child rows.
func TestRebuildFilesUsersLibraryPaths(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openRebuildTestDB(t)

	migrateBeforeRebuild(ctx, t, db)
	seedRebuildRows(ctx, t, db)

	var textIDs int
	require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM library_paths WHERE typeof(library_id) = 'text'").Scan(ctx, &textIDs))
	require.Equal(t, 2, textIDs, "the TEXT column stores library ids as text before the rebuild")

	before := takeRebuildSnapshot(ctx, t, db)
	for _, key := range []string{
		"index ix_files_library_id", "index ix_files_book_id", "index ix_files_publisher_id",
		"index ix_files_file_type_book_id", "index ux_files_filepath_library_id",
		"index idx_files_book_reviewed", "index idx_files_language",
		"index ix_users_role_id", "index ux_users_email", "index sqlite_autoindex_users_1",
		"index ix_library_paths_library_id",
	} {
		require.Contains(t, before.schemaObjects, key)
	}
	beforeColumns := map[string][]string{}
	for _, table := range rebuiltTables {
		beforeColumns[table] = columnNames(ctx, t, db, table)
	}

	migrator := NewMigrator(db)
	require.NoError(t, migrator.Init(ctx))
	for pass := range 2 {
		_, err := migrator.Migrate(ctx)
		require.NoError(t, err, "pass %d", pass)
		assertRebuiltSchema(ctx, t, db, before, beforeColumns)

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err, "pass %d", pass)
	}
	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	// RESTRICT refuses to delete a role that users still hold.
	_, err = db.ExecContext(ctx, "DELETE FROM roles WHERE id = 4")
	require.ErrorContains(t, err, "FOREIGN KEY")

	// Deleting library 1 removes file 2 through files.library_id alone,
	// since its Book lives in library 2.
	_, err = db.ExecContext(ctx, "DELETE FROM libraries WHERE id = 1")
	require.NoError(t, err)
	var fileIDs []int
	require.NoError(t, db.NewRaw("SELECT id FROM files ORDER BY id").Scan(ctx, &fileIDs))
	assert.Equal(t, []int{3}, fileIDs)
	var pathIDs []int
	require.NoError(t, db.NewRaw("SELECT id FROM library_paths ORDER BY id").Scan(ctx, &pathIDs))
	assert.Equal(t, []int{2}, pathIDs)
	assert.Zero(t, foreignKeyViolations(ctx, t, db))
}

// A rebuild must not change the connection's foreign key setting, whether
// enforcement was on or off beforehand.
func TestRebuildFilesUsersLibraryPathsKeepsForeignKeySetting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openRebuildTestDB(t)
	migrateBeforeRebuild(ctx, t, db)
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)

	migrator := NewMigrator(db)
	require.NoError(t, migrator.Init(ctx))
	group, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, group.Migrations)
	assert.Equal(t, rebuildFKMigrationName, group.Migrations[0].Name)

	var fkEnabled int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &fkEnabled))
	assert.Equal(t, 0, fkEnabled)
}

// A failed rebuild rolls back every table, leaves the migration unapplied,
// and restores foreign key enforcement, so the next startup can retry.
func TestRebuildFilesUsersLibraryPathsRollsBackOnFailure(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		breakDB []string
		wantErr string
	}{
		// A user whose role is gone moves to viewer, so with no viewer role
		// there is nowhere safe to put them.
		"orphaned user role without viewer": {
			breakDB: []string{
				`INSERT INTO users (id, username, password_hash, role_id) VALUES (9, 'orphan', 'hash', 99)`,
				`UPDATE roles SET name = 'renamed' WHERE name = 'viewer'`,
			},
			wantErr: "1 users reference a missing role (first: user 9, role 99) and the built-in viewer role to move them to is missing",
		},
		// A column the rebuild does not know about must stop the copy rather
		// than be dropped.
		"unexpected column": {
			breakDB: []string{`ALTER TABLE users ADD COLUMN nickname TEXT`},
			wantErr: "do not match users_new columns",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := openRebuildTestDB(t)
			migrateBeforeRebuild(ctx, t, db)
			seedRebuildRows(ctx, t, db)
			_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
			require.NoError(t, err)
			for _, query := range tc.breakDB {
				_, err = db.ExecContext(ctx, query)
				require.NoError(t, err, query)
			}
			_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
			require.NoError(t, err)
			before := takeRebuildSnapshot(ctx, t, db)

			migrator := NewMigrator(db)
			require.NoError(t, migrator.Init(ctx))
			_, err = migrator.Migrate(ctx)
			require.ErrorContains(t, err, tc.wantErr)

			var applied int
			require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM bun_migrations WHERE name = ?", rebuildFKMigrationName).Scan(ctx, &applied))
			assert.Zero(t, applied)
			assert.NotContains(t, foreignKeys(ctx, t, db, "files"), foreignKey{"library_id", "libraries", "id", "CASCADE"})
			typ, _ := columnDecl(ctx, t, db, "library_paths", "library_id")
			assert.Equal(t, "TEXT", typ)
			assert.Equal(t, before, takeRebuildSnapshot(ctx, t, db))
			for _, table := range rebuiltTables {
				var leftover int
				require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM sqlite_master WHERE name = ?", table+"_new").Scan(ctx, &leftover))
				assert.Zero(t, leftover)
			}

			var fkEnabled int
			require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &fkEnabled))
			assert.Equal(t, 1, fkEnabled)
		})
	}
}

// Rows orphaned before foreign keys were enforced get what their column's ON
// DELETE action would have done, instead of failing startup on every boot.
func TestRebuildFilesUsersLibraryPathsRepairsOrphans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openRebuildTestDB(t)
	migrateBeforeRebuild(ctx, t, db)
	seedRebuildRows(ctx, t, db)

	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	for _, query := range []string{
		// Book 5 points at a deleted library.
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source)
			VALUES (5, 99, '/gone/e', 'E', 'filepath', 'E', 'filepath')`,
		`INSERT INTO files (id, library_id, book_id, filepath, file_type, publisher_id) VALUES
			(10, 99, 1, '/one/a/lost-library.epub', 'epub', NULL),
			(11, 1, 98, '/one/lost-book.epub', 'epub', NULL),
			(12, 99, 97, '/gone/both.epub', 'epub', NULL),
			(13, 99, 5, '/gone/e/e.epub', 'epub', NULL),
			(14, 2, 2, '/two/b/lost-publisher.epub', 'epub', 777)`,
		`INSERT INTO chapters (file_id, sort_order, title) VALUES (11, 0, 'Lost')`,
		`INSERT INTO file_identifiers (file_id, type, value, source) VALUES (11, 'asin', 'B000', 'epub_metadata')`,
		`INSERT INTO file_fingerprints (file_id, algorithm, value) VALUES (12, 'sha256', 'def')`,
		`INSERT INTO narrators (file_id, person_id, sort_order) VALUES (13, 1, 0)`,
		`INSERT INTO library_paths (id, library_id, filepath) VALUES (10, 99, '/gone')`,
		`INSERT INTO users (id, username, password_hash, role_id) VALUES (9, 'orphan', 'hash', 99)`,
		"PRAGMA foreign_keys = ON",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}

	migrator := NewMigrator(db)
	require.NoError(t, migrator.Init(ctx))
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	var files []struct {
		ID          int  `bun:"id"`
		LibraryID   int  `bun:"library_id"`
		PublisherID *int `bun:"publisher_id"`
	}
	require.NoError(t, db.NewRaw("SELECT id, library_id, publisher_id FROM files ORDER BY id").Scan(ctx, &files))
	ids := make([]int, len(files))
	for i, f := range files {
		ids[i] = f.ID
	}
	// 11 lost its Book, 12 lost Book and Library, 13's Book and Library are
	// both gone: CASCADE removes them. 10 takes its Book's library.
	assert.Equal(t, []int{1, 2, 3, 10, 14}, ids)
	assert.Equal(t, 1, files[3].LibraryID)
	assert.Nil(t, files[4].PublisherID)
	assert.Equal(t, 1, files[1].LibraryID, "a file in another library than its Book is not an orphan")

	for _, table := range []string{"chapters", "file_identifiers", "file_fingerprints", "narrators"} {
		var count int
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM "+table+" WHERE file_id IN (11, 12, 13)").Scan(ctx, &count))
		assert.Zero(t, count, table)
	}
	var pathIDs []int
	require.NoError(t, db.NewRaw("SELECT id FROM library_paths ORDER BY id").Scan(ctx, &pathIDs))
	assert.Equal(t, []int{1, 2}, pathIDs)

	// A user whose role is gone moves to viewer; other users keep theirs.
	var roles []string
	require.NoError(t, db.NewRaw("SELECT r.name FROM users u JOIN roles r ON r.id = u.role_id ORDER BY u.id").Scan(ctx, &roles))
	assert.Equal(t, []string{"admin", "custom", "viewer"}, roles)

	for _, table := range rebuiltTables {
		var violations int
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM pragma_foreign_key_check(?)", table).Scan(ctx, &violations))
		assert.Zero(t, violations, table)
	}
}
