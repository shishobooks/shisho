package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// rollbackKeys names the expression that identifies a row in each table the
// round trip checks. Rebuilds renumber rowids, so plugin_identifier_types is
// keyed by its composite primary key.
var rollbackKeys = map[string]string{
	"library_paths":           "id",
	"series":                  "id",
	"persons":                 "id",
	"books":                   "id",
	"book_series":             "id",
	"files":                   "id",
	"authors":                 "id",
	"narrators":               "id",
	"jobs":                    "id",
	"publishers":              "id",
	"plugin_identifier_types": "scope || '/' || plugin_id || '/' || id",
}

// rowValues maps table -> row key -> column -> value. Values are read as
// text, so a value stored as '1' under TEXT affinity and 1 under INTEGER
// affinity compare equal, while a value moved to another column does not.
type rowValues map[string]map[string]map[string]sql.NullString

func readRowValues(ctx context.Context, t *testing.T, db *bun.DB) rowValues {
	t.Helper()
	out := rowValues{}
	for table, key := range rollbackKeys {
		columns := columnNames(ctx, t, db, table)
		if len(columns) == 0 {
			// Created by a migration that has been rolled back.
			continue
		}
		selects := make([]string, len(columns))
		for i, column := range columns {
			selects[i] = fmt.Sprintf(`CAST("%s" AS TEXT)`, column)
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT %s, %s FROM %s", key, strings.Join(selects, ", "), table))
		require.NoError(t, err, table)
		out[table] = map[string]map[string]sql.NullString{}
		for rows.Next() {
			var rowKey string
			values := make([]sql.NullString, len(columns))
			dest := []any{&rowKey}
			for i := range values {
				dest = append(dest, &values[i])
			}
			require.NoError(t, rows.Scan(dest...), table)
			row := map[string]sql.NullString{}
			for i, column := range columns {
				row[column] = values[i]
			}
			out[table][rowKey] = row
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	return out
}

// assertValuesKept checks that every seeded row is still present and that
// every column it still has holds the value it had at head.
func assertValuesKept(t *testing.T, seeded, got rowValues) {
	t.Helper()
	for table, rows := range seeded {
		if _, ok := got[table]; !ok {
			continue
		}
		for key, row := range rows {
			gotRow, ok := got[table][key]
			if !assert.True(t, ok, "%s row %s is missing", table, key) {
				continue
			}
			for column, value := range row {
				gotValue, ok := gotRow[column]
				if !ok {
					continue
				}
				assert.Equal(t, value, gotValue, "%s row %s column %s", table, key, column)
			}
		}
	}
}

func seedHeadRows(ctx context.Context, t *testing.T, db *bun.DB) {
	t.Helper()
	for _, query := range []string{
		// Drop the jobs migrations enqueue, which their downs delete.
		`DELETE FROM jobs`,
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`,
		`INSERT INTO library_paths (id, library_id, filepath) VALUES (1, 1, '/lib')`,
		`INSERT INTO publishers (id, library_id, name) VALUES (1, 1, 'Pub')`,
		// A NULL series description and a NULL genre_source and author_source
		// on books: the later schema allows them, the old one did not.
		`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source, description, cover_image_filename)
			VALUES (1, 1, 'S', 'filepath', 'S sort', 'manual', NULL, 's.jpg'),
			       (2, 1, 'S2', 'manual', 'S2 sort', 'filepath', 'Described', NULL)`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source,
				subtitle, subtitle_source, description, description_source, genre_source, tag_source, author_source, series_source)
			VALUES (1, 1, '/lib/a', 'T', 'filepath', 'T sort', 'manual', 'Sub', 'manual', 'Desc', 'manual', NULL, 'manual', NULL, 'manual'),
			       (2, 1, '/lib/b', 'T2', 'manual', 'T2 sort', 'filepath', NULL, 'filepath', 'Desc 2', NULL, 'manual', NULL, 'filepath', NULL)`,
		`INSERT INTO book_series (id, book_id, series_id, series_number, sort_order) VALUES (1, 1, 1, 2.5, 1)`,
		`INSERT INTO persons (id, library_id, name, sort_name, sort_name_source) VALUES (1, 1, 'P', 'P sort', 'manual')`,
		`INSERT INTO authors (id, book_id, person_id, sort_order, role) VALUES (1, 1, 1, 1, 'writer')`,
		// A value in every files column, so a column that moves shows up.
		`INSERT INTO files (id, created_at, updated_at, library_id, book_id, filepath, file_type, file_role,
				filesize_bytes, file_modified_at, cover_image_filename, cover_mime_type, cover_source, cover_page,
				name, name_source, page_count, audiobook_duration_seconds, audiobook_bitrate_bps, audiobook_codec,
				narrator_source, identifier_source, url, url_source, release_date, release_date_source,
				publisher_id, publisher_source, chapter_source, language, language_source,
				abridged, abridged_source, review_override, review_overridden_at, reviewed, is_preferred_cover, scan_error)
			VALUES (1, '2026-01-01 00:00:01', '2026-01-01 00:00:02', 1, 1, '/lib/a/a.m4b', 'm4b', 'main',
				10, '2026-01-01 00:00:03', 'a.m4b.cover.jpg', 'image/jpeg', 'm4b_metadata', 4,
				'File name', 'manual', 5, 6.5, 7, 'aac',
				'm4b_metadata', 'manual', 'https://example.com', 'manual', '2026-01-01 00:00:04', 'manual',
				1, 'manual', 'm4b_metadata', 'en', 'manual',
				1, 'manual', 'complete', '2026-01-01 00:00:05', 1, 1, 'scan error')`,
		`INSERT INTO narrators (id, file_id, person_id, sort_order) VALUES (1, 1, 1, 1)`,
		`INSERT INTO jobs (id, type, status, data, progress, library_id) VALUES (100, 'scan', 'completed', '{}', 100, 1)`,
		// Two plugins that register the same identifier type id.
		`INSERT INTO plugins (scope, id, name, version) VALUES ('local', 'one', 'One', '1.0.0'), ('shisho', 'two', 'Two', '1.0.0')`,
		`INSERT INTO plugin_identifier_types (id, scope, plugin_id, name, url_template, pattern)
			VALUES ('goodreads', 'local', 'one', 'Goodreads', 'https://one/{value}', '^\d+$'),
			       ('goodreads', 'shisho', 'two', 'Goodreads 2', NULL, NULL)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
}

func leftoverRebuildTables(ctx context.Context, t *testing.T, db *bun.DB) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.NewRaw(
		`SELECT name FROM sqlite_master WHERE name LIKE '%\_new' ESCAPE '\' OR name LIKE '%\_old' ESCAPE '\'`,
	).Scan(ctx, &names))
	return names
}

// Rolling back every migration after the initial schema on a database with
// rows keeps each value in its own column, and migrating back up restores the
// head schema with the same values. mise start applies every migration as one
// group, so mise db:rollback runs every down in a row like this.
func TestRollbackAllMigrationsKeepsRowValues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	sorted := Migrations.Sorted()
	initial := migrate.NewMigrations()
	initial.Add(sorted[0])
	initialMigrator := newMigrator(db, initial)
	require.NoError(t, initialMigrator.Init(ctx))
	_, err := initialMigrator.Migrate(ctx)
	require.NoError(t, err)

	migrator := newMigrator(db, Migrations)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	seedHeadRows(ctx, t, db)
	seeded := readRowValues(ctx, t, db)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.Empty(t, leftoverRebuildTables(ctx, t, db))
	assert.Zero(t, foreignKeyViolations(ctx, t, db))
	rolledBack := readRowValues(ctx, t, db)
	assertValuesKept(t, seeded, rolledBack)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.Empty(t, leftoverRebuildTables(ctx, t, db))
	assert.Zero(t, foreignKeyViolations(ctx, t, db))
	// Columns a down drops come back empty, so only the ones that survived
	// the rollback can be expected to hold their original value.
	assertValuesKept(t, rolledBack, readRowValues(ctx, t, db))
}

func schemaSQL(ctx context.Context, t *testing.T, db *bun.DB) map[string]string {
	t.Helper()
	var objects []struct {
		Name string         `bun:"name"`
		SQL  sql.NullString `bun:"sql"`
	}
	require.NoError(t, db.NewRaw(`SELECT name, sql FROM sqlite_master WHERE name <> 'bun_migrations'`).Scan(ctx, &objects))
	out := map[string]string{}
	for _, o := range objects {
		out[o.Name] = o.SQL.String
	}
	return out
}

// A down that fails partway leaves the schema and rows as they were before
// it started. The files column the down does not know about makes it fail
// after it has already rebuilt five tables.
func TestFailedDownLeavesDatabaseUnchanged(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	migrator := migrateToBefore(ctx, t, db, "20260406100000")
	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`,
		`INSERT INTO series (id, library_id, name, name_source, sort_name, sort_name_source) VALUES (1, 1, 'S', 'filepath', 'S', 'filepath')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source, author_source)
			VALUES (1, 1, '/a', 'T', 'filepath', 'T', 'filepath', 'filepath')`,
		`INSERT INTO book_series (book_id, series_id, sort_order) VALUES (1, 1, 1)`,
		`ALTER TABLE files ADD COLUMN stray TEXT`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
	beforeSchema := schemaSQL(ctx, t, db)
	beforeRows := readRowValues(ctx, t, db)

	_, err = migrator.Rollback(ctx)
	require.ErrorContains(t, err, "files columns")

	assert.Equal(t, beforeSchema, schemaSQL(ctx, t, db))
	assert.Equal(t, beforeRows, readRowValues(ctx, t, db))

	var fkEnabled int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &fkEnabled))
	assert.Equal(t, 1, fkEnabled, "the failed down restores foreign key enforcement")
}

// Foreign keys were not enforced before 20260406100000, so an upgraded
// database can hold rows with a missing parent. The rebuilds must carry them
// through for 20261005000000 to repair, not stop the upgrade.
func TestUpgradeKeepsRowsWithMissingParentsForRepair(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)

	migrateToBefore(ctx, t, db, "20260406100000")
	_, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	for _, query := range []string{
		`INSERT INTO libraries (id, name, cover_aspect_ratio) VALUES (1, 'L', 'book')`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source, author_source)
			VALUES (1, 1, '/a', 'T', 'filepath', 'T', 'filepath', 'filepath')`,
		`INSERT INTO authors (book_id, person_id, sort_order) VALUES (1, 99, 1)`,
		`INSERT INTO books (id, library_id, filepath, title, title_source, sort_title, sort_title_source, author_source)
			VALUES (2, 99, '/b', 'T', 'filepath', 'T', 'filepath', 'filepath')`,
		`INSERT INTO files (library_id, book_id, filepath, file_type, filesize_bytes) VALUES (1, 98, '/c.epub', 'epub', 1)`,
		`INSERT INTO files (library_id, book_id, filepath, file_type, filesize_bytes, publisher_id) VALUES (1, 1, '/a/a.epub', 'epub', 1, 97)`,
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err, query)
	}
	_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = newMigrator(db, Migrations).Migrate(ctx)
	require.NoError(t, err)
	assert.Zero(t, foreignKeyViolations(ctx, t, db))
}
