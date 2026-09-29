// Package testdb opens the database that package tests run against. It
// imports only pkg/migrations (and, through it, pkg/models and
// pkg/identifiers), so tests in every other package can use it without an
// import cycle, including the packages pkg/testutils imports. The internal
// tests of pkg/migrations, pkg/models, and pkg/identifiers are the exception:
// they would import their own package through this one.
package testdb

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// New returns an in-memory SQLite database migrated to the latest schema, with
// foreign keys on, that is closed when the test ends. Each call returns a
// separate database, so parallel tests do not share rows.
//
// The pool is pinned to one connection, as pkg/database does in production:
// every connection to ":memory:" opens its own empty database, and
// PRAGMA foreign_keys applies only to the connection that ran it.
func New(t testing.TB) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	return db
}
