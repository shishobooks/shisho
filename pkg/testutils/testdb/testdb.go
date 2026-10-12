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
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

// seq gives each test database its own shared-cache name, so parallel tests
// never open each other's database.
var seq atomic.Int64

// New returns an in-memory SQLite database migrated to the latest schema, with
// foreign keys on, that is closed when the test ends. Each call returns a
// separate database, so parallel tests do not share rows. The pool holds one
// connection, as pkg/database does in production.
func New(t testing.TB) *bun.DB {
	t.Helper()
	return open(t, 1)
}

// NewShared is New without the one-connection cap, for tests whose code runs
// queries from several goroutines at once, such as the worker.
func NewShared(t testing.TB) *bun.DB {
	t.Helper()
	return open(t, 0)
}

// open returns a migrated database whose pool opens at most maxOpen
// connections, or any number when maxOpen is 0.
//
// database/sql discards a connection whose query is canceled mid-statement and
// opens a replacement. Every connection to a named shared-cache memory
// database sees the same data, so the replacement finds the schema, but the
// database is destroyed when its last connection closes, and the discarded one
// is often the only one in the pool. A keeper connection held outside the pool
// until the test ends keeps it alive. PRAGMA foreign_keys applies only to the
// connection that ran it, so sqliteconn runs it on every connection the pool
// opens, as pkg/database does.
func open(t testing.TB, maxOpen int) *bun.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:testdb-%d?mode=memory&cache=shared", seq.Add(1))
	connector, err := sqliteconn.NewConnector(dsn, "PRAGMA foreign_keys=ON")
	require.NoError(t, err)

	keeper, err := connector.Connect(context.Background())
	require.NoError(t, err)
	// Registered before the pool's cleanup, so it runs after it.
	t.Cleanup(func() {
		_ = keeper.Close()
	})

	sqldb := sql.OpenDB(connector)
	if maxOpen > 0 {
		sqldb.SetMaxOpenConns(maxOpen)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		_ = db.Close()
	})

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	return db
}
