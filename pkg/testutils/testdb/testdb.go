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
	"database/sql/driver"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"modernc.org/sqlite"
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
// database/sql discards a connection the driver reports bad and opens a
// replacement. (A query canceled mid-statement does this on a file database;
// the driver keeps an interrupted in-memory connection.) Every connection to a
// named shared-cache memory database sees the same data, so the replacement
// finds the schema, but the database is destroyed when its last connection
// closes, and the discarded one is often the only one in the pool. A keeper connection held outside the pool
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

	fwd := &forwardHook{}
	db := bun.NewDB(sqldb, sqliteconn.NewDialect()).WithQueryHook(fwd)
	forwarders.Store(db, fwd)
	t.Cleanup(func() {
		forwarders.Delete(db)
		_ = db.Close()
	})

	require.NoError(t, migrate(keeper, db))

	return db
}

// templateDSN names a database migrated once per test binary. Under -race,
// running the migrations takes seconds per database while copying the
// template takes milliseconds, and most tests open at least one database.
const templateDSN = "file:testdb-template?mode=memory&cache=shared"

var (
	templateOnce sync.Once
	templateErr  error
)

// restorer is the modernc connection's backup API, which sqliteshim uses
// wherever modernc supports the platform.
type restorer interface {
	NewRestore(srcURI string) (*sqlite.Backup, error)
}

// migrate brings db, whose connections share keeper's database, to the latest
// schema by copying the template into keeper, or by running the migrations
// when the driver has no backup API.
func migrate(keeper driver.Conn, db *bun.DB) error {
	r, ok := keeper.(restorer)
	if !ok {
		_, err := migrations.BringUpToDate(context.Background(), db)
		return err
	}

	templateOnce.Do(func() { templateErr = buildTemplate() })
	if templateErr != nil {
		return templateErr
	}

	backup, err := r.NewRestore(templateDSN)
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := backup.Step(-1); err != nil {
		_ = backup.Finish()
		return errors.WithStack(err)
	}
	return errors.WithStack(backup.Finish())
}

// buildTemplate migrates the template database and keeps a connection to it
// open for the life of the test binary, so it is never destroyed.
func buildTemplate() error {
	connector, err := sqliteconn.NewConnector(templateDSN, "PRAGMA foreign_keys=ON")
	if err != nil {
		return err
	}
	if _, err := connector.Connect(context.Background()); err != nil {
		return errors.WithStack(err)
	}

	sqldb := sql.OpenDB(connector)
	defer sqldb.Close()
	_, err = migrations.BringUpToDate(context.Background(), bun.NewDB(sqldb, sqliteconn.NewDialect()))
	return err
}
