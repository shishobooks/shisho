# SQLite Database Lock Fix Design

## Problem

Production scans show frequent "database is locked" errors in logs. Investigation revealed:

1. Multiple parallel workers (4+ goroutines) make concurrent writes during scans
2. No connection pool limits in production code
3. SQLite only supports one writer at a time, even with WAL mode
4. When retry logic (5 attempts) is exhausted, errors surface in logs and data may not be saved

## Root Cause

SQLite serializes all writes internally. With multiple Go connections competing for the write lock, the existing retry logic (5 retries with exponential backoff up to 2s) can be exhausted during heavy scan operations, especially on slow filesystems.

## Solution

Set `MaxOpenConns(1)` to serialize all database operations through a single connection. This moves the wait from "busy_timeout + retry" to "connection pool queue" - which is more efficient and guarantees eventual completion without data loss.

## Changes

### pkg/database/database.go

Add after creating the DB connection:

```go
sqldb = sql.OpenDB(retryConnector)

// Limit to a single connection for SQLite.
// SQLite only supports one writer at a time, so multiple connections
// just compete for the write lock. A single connection serializes
// all operations at the Go level, eliminating SQLITE_BUSY errors.
sqldb.SetMaxOpenConns(1)
```

Add PRAGMA optimizations for performance:

```go
// synchronous=NORMAL is faster than FULL and still safe with WAL mode.
// It only risks data loss on OS crash, not application crash.
_, err = db.Exec("PRAGMA synchronous=NORMAL")

// Increase page cache to 64MB (negative value = KB).
// Improves read performance for repeated queries.
_, err = db.Exec("PRAGMA cache_size=-65536")

// Store temporary tables in memory instead of disk.
// Faster for complex queries with temp results.
_, err = db.Exec("PRAGMA temp_store=MEMORY")
```

### No Changes Required

- `pkg/database/retry.go` - Keep existing retry logic as safety net for edge cases
- `pkg/config/config.go` - Keep current defaults (5 retries, 5s busy timeout)
- `shisho.example.yaml` - No new config options needed

## Testing

1. Existing unit tests continue to pass
2. Attempt to write a concurrent stress test (may not be reliably reproducible)
3. Monitor production logs after deployment for any remaining lock errors

## References

- [Making SQLite faster in Go](https://turriate.com/articles/making-sqlite-faster-in-go)
- [Go and SQLite in the Cloud](https://www.golang.dk/articles/go-and-sqlite-in-the-cloud)
- [SQLite WAL documentation](https://sqlite.org/wal.html)
