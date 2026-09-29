package migrations

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

// Setup and schema inspection helpers shared by migration tests.

// openMigrationTestDB returns an empty in-memory database with foreign keys
// on. One connection, as in production, so every statement sees the same
// in-memory database and the same PRAGMA state.
func openMigrationTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	return db
}

// migrateToBefore applies every registered migration that sorts before name
// and returns a migrator for the migration named name alone.
func migrateToBefore(ctx context.Context, t *testing.T, db *bun.DB, name string) *migrate.Migrator {
	t.Helper()
	previous := migrate.NewMigrations()
	target := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		switch {
		case migration.Name < name:
			previous.Add(migration)
		case migration.Name == name:
			target.Add(migration)
		}
	}
	require.Len(t, target.Sorted(), 1, "migration %s should be registered", name)
	previousMigrator := newMigrator(db, previous)
	require.NoError(t, previousMigrator.Init(ctx))
	_, err := previousMigrator.Migrate(ctx)
	require.NoError(t, err)
	return newMigrator(db, target)
}

func indexExists(ctx context.Context, t *testing.T, db *bun.DB, name string) bool {
	t.Helper()
	count, err := db.NewSelect().
		TableExpr("sqlite_master").
		Where("type = 'index' AND name = ?", name).
		Count(ctx)
	require.NoError(t, err)
	return count == 1
}

// queryPlan returns the EXPLAIN QUERY PLAN details of query, one per line.
func queryPlan(ctx context.Context, t *testing.T, db *bun.DB, query string) string {
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

type foreignKey struct {
	From     string `bun:"from"`
	Table    string `bun:"table"`
	To       string `bun:"to"`
	OnDelete string `bun:"on_delete"`
}

func foreignKeys(ctx context.Context, t *testing.T, db *bun.DB, table string) []foreignKey {
	t.Helper()
	var fks []foreignKey
	require.NoError(t, db.NewRaw(`SELECT "from", "table", "to", on_delete FROM pragma_foreign_key_list(?)`, table).Scan(ctx, &fks))
	return fks
}

func columnDecl(ctx context.Context, t *testing.T, db *bun.DB, table, column string) (string, bool) {
	t.Helper()
	var info struct {
		Type    string `bun:"type"`
		NotNull bool   `bun:"column:notnull"`
	}
	require.NoError(t, db.NewRaw("SELECT type, \"notnull\" FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(ctx, &info))
	return info.Type, info.NotNull
}

func columnNames(ctx context.Context, t *testing.T, db *bun.DB, table string) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info(?) ORDER BY cid", table).Scan(ctx, &names))
	return names
}

func foreignKeyViolations(ctx context.Context, t *testing.T, db *bun.DB) int {
	t.Helper()
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	return count
}
