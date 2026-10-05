package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
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

// connectionPragmas returns the pragmas every connection runs when it opens.
// They apply per connection, so sqliteconn runs them on replacements too.
func connectionPragmas(cfg *config.Config) []string {
	return []string{
		// busy_timeout makes SQLite wait before returning SQLITE_BUSY.
		// This handles short-term lock contention automatically.
		fmt.Sprintf("PRAGMA busy_timeout=%d", cfg.DatabaseBusyTimeout.Milliseconds()),
		// synchronous=NORMAL is faster than FULL and still safe with WAL mode.
		// It only risks data loss on OS crash, not application crash.
		"PRAGMA synchronous=NORMAL",
		// Increase page cache to 64MB (negative value = KB).
		// Improves read performance for repeated queries.
		"PRAGMA cache_size=-65536",
		// Store temporary tables in memory instead of disk.
		// Faster for complex queries with temp results.
		"PRAGMA temp_store=MEMORY",
		// SQLite has foreign keys OFF by default; without this, ON DELETE
		// CASCADE and other FK actions are silently ignored.
		"PRAGMA foreign_keys=ON",
	}
}

func New(cfg *config.Config) (*bun.DB, error) {
	connector, err := sqliteconn.NewConnector(cfg.DatabaseFilePath, connectionPragmas(cfg)...)
	if err != nil {
		return nil, err
	}

	// Wrap the connector with retry logic for SQLITE_BUSY errors.
	retryConnector := newRetryConnector(connector, cfg.DatabaseMaxRetries)
	sqldb := sql.OpenDB(retryConnector)

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

	// WAL mode allows concurrent reads during writes. It is stored in the
	// database file, so one call covers every later connection.
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return nil, errors.Wrap(err, "failed to enable WAL mode")
	}

	return db, nil
}
