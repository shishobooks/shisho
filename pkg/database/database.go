package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// slowQueryThreshold is how long a query may take before it is logged as a
// warning. The warning is always on, so it carries no SQL text or values.
const slowQueryThreshold = 250 * time.Millisecond

// maxLoggedQueryBytes caps the SQL text a database_debug log line carries, so
// a bulk insert does not flood the log buffer.
const maxLoggedQueryBytes = 2048

// queryLogHook logs queries in two tiers. Queries slower than
// slowQueryThreshold always log at warn with their operation, table and
// duration. bun inlines parameter values into the SQL text (password hashes,
// share tokens, plugin config), and Settings > Logs is readable with
// config:read, so that tier never includes the SQL. With database_debug on,
// every query also logs at debug with its SQL, truncated to
// maxLoggedQueryBytes.
type queryLogHook struct {
	log      logger.Logger
	debugLog *logger.Logger
}

// newQueryLogHook builds the hook. Build it after logger.SetOutput, since a
// logger keeps the output it was created with.
func newQueryLogHook(debug bool) *queryLogHook {
	h := &queryLogHook{log: logger.New()}
	if debug {
		// Debug logging must not depend on LOG_LEVEL: turning on
		// database_debug is the request to see every statement.
		debugLog := logger.NewWithLevel("debug")
		h.debugLog = &debugLog
	}
	return h
}

func (*queryLogHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *queryLogHook) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	duration := time.Since(event.StartTime)
	durationMS := float64(duration.Microseconds()) / 1000

	if h.debugLog != nil {
		query := event.Query
		if len(query) > maxLoggedQueryBytes {
			// Cut on a rune boundary so the log line stays valid UTF-8.
			end := maxLoggedQueryBytes
			for end > 0 && !utf8.RuneStart(query[end]) {
				end--
			}
			query = query[:end] + "...(truncated)"
		}
		data := logger.Data{"query": query, "duration_ms": durationMS}
		if event.Err != nil && !errors.Is(event.Err, sql.ErrNoRows) {
			data["error"] = event.Err.Error()
		}
		h.debugLog.Debug("sql query", data)
	}

	if duration >= slowQueryThreshold {
		data := logger.Data{"operation": event.Operation(), "duration_ms": durationMS}
		if table := queryTable(event); table != "" {
			data["table"] = table
		}
		h.log.Warn("slow query", data)
	}
}

// queryTable returns the table of a model query, or "" for a raw query. It
// reads the model's schema rather than the query text, which can hold values.
func queryTable(event *bun.QueryEvent) string {
	if tm, ok := event.Model.(bun.TableModel); ok && tm.Table() != nil {
		return tm.Table().Name
	}
	return ""
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

	db.AddQueryHook(newQueryLogHook(cfg.DatabaseDebug))

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

	// Enable foreign key constraint enforcement.
	// SQLite has foreign keys OFF by default; without this, ON DELETE CASCADE
	// and other FK actions are silently ignored.
	_, err = db.Exec("PRAGMA foreign_keys=ON")
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable foreign keys")
	}

	return db, nil
}
