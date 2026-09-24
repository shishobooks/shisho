package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

// The backfill copies the first membership's series.name_source, which is
// exactly the value the scanner used as the effective source before the
// column existed, so existing Books keep their Scan protection.
func TestAddSeriesSourceBackfillsFromFirstMembership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, query := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE books (id INTEGER PRIMARY KEY, title TEXT NOT NULL, genre_source TEXT)",
		"CREATE UNIQUE INDEX books_title_unique ON books(title)",
		"CREATE TABLE series (id INTEGER PRIMARY KEY, name TEXT NOT NULL, name_source TEXT)",
		"CREATE TABLE book_series (id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE, series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE, sort_order INTEGER)",
		"INSERT INTO series VALUES (1, 'Manual Series', 'manual'), (2, 'Plugin Series', 'plugin:test/enricher'), (3, 'Embedded Series', 'epub_metadata')",
		"INSERT INTO books (id, title) VALUES (1, 'Manual First'), (2, 'Plugin Only'), (3, 'No Series')",
		// Insertion order deliberately disagrees with sort_order for book 1.
		"INSERT INTO book_series VALUES (10, 1, 3, 2), (11, 1, 1, 1), (12, 2, 2, 1)",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20260922000000" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	// Bun scans SQL NULL into a non-nil empty string pointer, so make NULL
	// explicit in SQL instead.
	var sources []string
	require.NoError(t, db.NewRaw("SELECT COALESCE(series_source, 'NULL') FROM books ORDER BY id").Scan(ctx, &sources))
	assert.Equal(t, []string{"manual", "plugin:test/enricher", "NULL"}, sources, "the first membership by sort_order wins; a book without memberships has no membership provenance")

	// Series name provenance is untouched by the backfill.
	var nameSources []string
	require.NoError(t, db.NewRaw("SELECT name_source FROM series ORDER BY id").Scan(ctx, &nameSources))
	assert.Equal(t, []string{"manual", "plugin:test/enricher", "epub_metadata"}, nameSources)

	assertBooksIndexAndForeignKeys := func() {
		t.Helper()
		_, err := db.ExecContext(ctx, "INSERT INTO books (id, title) VALUES (99, 'Manual First')")
		require.ErrorContains(t, err, "UNIQUE")
		_, err = db.ExecContext(ctx, "INSERT INTO book_series (id, book_id, series_id, sort_order) VALUES (99, 404, 1, 1)")
		require.ErrorContains(t, err, "FOREIGN KEY")
	}
	assertBooksIndexAndForeignKeys()

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	var columns []string
	require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info('books')").Scan(ctx, &columns))
	assert.NotContains(t, columns, "series_source")
	assertBooksIndexAndForeignKeys()

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	var reapplied string
	require.NoError(t, db.NewRaw("SELECT COALESCE(series_source, 'NULL') FROM books WHERE id = 1").Scan(ctx, &reapplied))
	assert.Equal(t, "manual", reapplied)
	assertBooksIndexAndForeignKeys()
}
