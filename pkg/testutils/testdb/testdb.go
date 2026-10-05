// Package testdb opens the database that package tests run against. It
// imports only pkg/migrations (and, through it, pkg/models and
// pkg/identifiers) and pkg/sqliteconn, so tests in every other package can
// use it without an import cycle, including the packages pkg/testutils
// imports. The internal
// tests of pkg/migrations, pkg/models, and pkg/identifiers are the exception:
// they would import their own package through this one.
package testdb

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// New returns an in-memory SQLite database migrated to the latest schema, with
// foreign keys on, that is closed when the test ends. Each call returns a
// separate database, so parallel tests do not share rows.
//
// The pool is pinned to one connection, as pkg/database does in production:
// every connection to ":memory:" opens its own empty database. PRAGMA
// foreign_keys applies only to the connection that ran it, so sqliteconn runs
// it on every connection the pool opens, as pkg/database does. A replacement
// connection, opened after a query is canceled mid-statement, has foreign
// keys on but sees a new empty ":memory:" database, so the schema is gone.
func New(t testing.TB) *bun.DB {
	t.Helper()

	connector, err := sqliteconn.NewConnector(":memory:", "PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	sqldb := sql.OpenDB(connector)
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	return db
}
