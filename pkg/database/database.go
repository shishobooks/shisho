package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type key int

const ctxKey key = 0

func WithLogging(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey, true)
}

type logQueryHook struct {
	log logger.Logger
}

func (*logQueryHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (qh *logQueryHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	enabled, ok := ctx.Value(ctxKey).(bool)
	if !ok || !enabled {
		return
	}

	qh.log.Debug(event.Query)
}

// CheckFTS5Support verifies FTS5 is available in the SQLite build.
// This should be called after database initialization to ensure search functionality will work.
func CheckFTS5Support(db *bun.DB) error {
	_, err := db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS _fts5_check USING fts5(test)")
	if err != nil {
		return errors.New("FTS5 is not enabled on this SQLite build. " +
			"This is required for search functionality. " +
			"Please create an issue at https://github.com/shishobooks/shisho/issues")
	}
	// Clean up the test table
	_, _ = db.Exec("DROP TABLE IF EXISTS _fts5_check")
	return nil
}

func New(cfg *config.Config) (*bun.DB, error) {
	var sqldb *sql.DB
	var err error

	// Get the underlying SQLite driver.
	drv := sqliteshim.Driver()

	// Try to use native OpenConnector if supported, otherwise create our own connector.
	var connector driver.Connector
	drvCtx, ok := drv.(interface {
		OpenConnector(name string) (driver.Connector, error)
	})
	if ok {
		connector, err = drvCtx.OpenConnector(cfg.DatabaseFilePath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		// Fallback: wrap the driver in our own connector implementation.
		connector = newDriverConnector(drv, cfg.DatabaseFilePath)
	}

	// Wrap the connector with retry logic for SQLITE_BUSY errors.
	retryConnector := newRetryConnector(connector, cfg.DatabaseMaxRetries)
	sqldb = sql.OpenDB(retryConnector)

	// Limit to a single connection for SQLite.
	// SQLite only supports one writer at a time, so multiple connections
	// just compete for the write lock. A single connection serializes
	// all operations at the Go level, eliminating SQLITE_BUSY errors.
	sqldb.SetMaxOpenConns(1)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// print out all queries in debug mode
	if cfg.DatabaseDebug {
		db.AddQueryHook(&logQueryHook{logger.NewWithLevel("debug")})
	}

	// Retry up to a few times to ensure that the database can connect.
	for i := 0; i < cfg.DatabaseConnectRetryCount; i++ {
		_, err = db.Exec("SELECT 1")
		if err != nil {
			time.Sleep(cfg.DatabaseConnectRetryDelay)
			continue
		}
		// We've successfully connected.
		break
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Configure SQLite for better concurrency handling.
	// WAL mode allows concurrent reads during writes.
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable WAL mode")
	}

	// busy_timeout makes SQLite wait before returning SQLITE_BUSY.
	// This handles short-term lock contention automatically.
	busyTimeoutMs := cfg.DatabaseBusyTimeout.Milliseconds()
	_, err = db.Exec("PRAGMA busy_timeout=?", busyTimeoutMs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set busy_timeout")
	}

	// synchronous=NORMAL is faster than FULL and still safe with WAL mode.
	// It only risks data loss on OS crash, not application crash.
	_, err = db.Exec("PRAGMA synchronous=NORMAL")
	if err != nil {
		return nil, errors.Wrap(err, "failed to set synchronous mode")
	}

	// Increase page cache to 64MB (negative value = KB).
	// Improves read performance for repeated queries.
	_, err = db.Exec("PRAGMA cache_size=-65536")
	if err != nil {
		return nil, errors.Wrap(err, "failed to set cache_size")
	}

	// Store temporary tables in memory instead of disk.
	// Faster for complex queries with temp results.
	_, err = db.Exec("PRAGMA temp_store=MEMORY")
	if err != nil {
		return nil, errors.Wrap(err, "failed to set temp_store")
	}

	return db, nil
}
