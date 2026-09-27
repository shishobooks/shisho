package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

const indexForeignKeyColumnsMigrationName = "20260928100000"

// Each foreign key column below had no index whose leading column is the
// foreign key, so looking up children by parent (alias subqueries during
// reindexing, ON DELETE actions on the parent) scanned the whole table. The
// migration adds one plain index per column and rolling back drops them.
func TestIndexForeignKeyColumns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	// Bring the schema up to the migration just before this one.
	previous := migrate.NewMigrations()
	target := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		switch {
		case migration.Name < indexForeignKeyColumnsMigrationName:
			previous.Add(migration)
		case migration.Name == indexForeignKeyColumnsMigrationName:
			target.Add(migration)
		}
	}
	require.Len(t, target.Sorted(), 1, "migration %s should be registered", indexForeignKeyColumnsMigrationName)
	previousMigrator := newMigrator(db, previous)
	require.NoError(t, previousMigrator.Init(ctx))
	_, err = previousMigrator.Migrate(ctx)
	require.NoError(t, err)

	columns := []struct{ table, column string }{
		{"person_aliases", "person_id"},
		{"series_aliases", "series_id"},
		{"genre_aliases", "genre_id"},
		{"tag_aliases", "tag_id"},
		{"publisher_aliases", "publisher_id"},
		{"publishers", "parent_id"},
		{"jobs", "library_id"},
		{"list_books", "added_by_user_id"},
		{"list_shares", "shared_by_user_id"},
		{"user_library_settings", "library_id"},
	}

	indexExists := func(name string) bool {
		t.Helper()
		count, err := db.NewSelect().
			TableExpr("sqlite_master").
			Where("type = 'index' AND name = ?", name).
			Count(ctx)
		require.NoError(t, err)
		return count == 1
	}
	plan := func(query string) string {
		t.Helper()
		var rows []struct {
			ID      int    `bun:"id"`
			Parent  int    `bun:"parent"`
			NotUsed int    `bun:"notused"`
			Detail  string `bun:"detail"`
		}
		require.NoError(t, db.NewRaw("EXPLAIN QUERY PLAN "+query).Scan(ctx, &rows), query)
		details := make([]string, 0, len(rows))
		for _, row := range rows {
			details = append(details, row.Detail)
		}
		return strings.Join(details, "\n")
	}
	assertIndexed := func() {
		t.Helper()
		for _, c := range columns {
			name := "ix_" + c.table + "_" + c.column
			assert.True(t, indexExists(name), "index %s should exist", name)
			// A lookup of children by parent is an index search on the new
			// index, not a scan.
			assert.Contains(t,
				plan("SELECT * FROM "+c.table+" WHERE "+c.column+" = 1"),
				"SEARCH "+c.table+" USING INDEX "+name+" ("+c.column+"=?)",
			)
		}
	}

	migrator := newMigrator(db, target)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed()

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	for _, c := range columns {
		name := "ix_" + c.table + "_" + c.column
		assert.False(t, indexExists(name), "index %s should be dropped on rollback", name)
	}

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assertIndexed()
}
