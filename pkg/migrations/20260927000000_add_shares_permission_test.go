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

// Only the Admin role receives the shares permission, and rolling back
// removes it from every role, including custom roles granted it later.
func TestAddSharesPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, query := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE roles (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE)",
		`CREATE TABLE permissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role_id INTEGER REFERENCES roles (id) ON DELETE CASCADE NOT NULL,
			resource TEXT NOT NULL,
			operation TEXT NOT NULL,
			UNIQUE (role_id, resource, operation)
		)`,
		"INSERT INTO roles VALUES (1, 'admin'), (2, 'editor'), (3, 'viewer'), (4, 'custom')",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20260927000000" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))

	sharePermissions := func() []string {
		t.Helper()
		var rows []string
		require.NoError(t, db.NewRaw(`
			SELECT r.name || ':' || p.operation
			FROM permissions p JOIN roles r ON r.id = p.role_id
			WHERE p.resource = 'shares'
			ORDER BY r.name, p.operation
		`).Scan(ctx, &rows))
		return rows
	}

	for range 2 {
		_, err = migrator.Migrate(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"admin:read", "admin:write"}, sharePermissions())

		_, err = db.ExecContext(ctx, "INSERT INTO permissions (role_id, resource, operation) VALUES (4, 'shares', 'read')")
		require.NoError(t, err)

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
		assert.Empty(t, sharePermissions())
	}
}
