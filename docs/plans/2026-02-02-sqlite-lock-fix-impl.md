# SQLite Database Lock Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate "database is locked" errors by serializing all SQLite operations through a single connection and adding PRAGMA optimizations for performance.

**Architecture:** Set `MaxOpenConns(1)` on the connection pool to serialize writes at the Go level instead of relying on SQLite's busy_timeout + retry. Add PRAGMA optimizations (synchronous=NORMAL, cache_size, temp_store) to compensate for the single-connection bottleneck.

**Tech Stack:** Go, SQLite, Bun ORM, `database/sql` connection pool

---

## Task 1: Add Connection Pool Limit

**Files:**
- Modify: `pkg/database/database.go:78-81`

**Step 1: Add MaxOpenConns after creating the DB connection**

In `pkg/database/database.go`, after line 80 (`sqldb = sql.OpenDB(retryConnector)`), add:

```go
sqldb = sql.OpenDB(retryConnector)

// Limit to a single connection for SQLite.
// SQLite only supports one writer at a time, so multiple connections
// just compete for the write lock. A single connection serializes
// all operations at the Go level, eliminating SQLITE_BUSY errors.
sqldb.SetMaxOpenConns(1)
```

**Step 2: Run existing tests to verify no regressions**

Run: `go test ./pkg/database/... -v`

Expected: All tests pass (retry_test.go should still work)

**Step 3: Commit**

```bash
git add pkg/database/database.go
git commit -m "$(cat <<'EOF'
[Fix] Limit SQLite to single connection to prevent lock errors

Set MaxOpenConns(1) to serialize all database operations through
a single connection. This moves the wait from "busy_timeout + retry"
to "connection pool queue" - more efficient and guarantees eventual
completion without data loss.
EOF
)"
```

---

## Task 2: Add PRAGMA Optimizations

**Files:**
- Modify: `pkg/database/database.go:103-117`

**Step 1: Add PRAGMA optimizations after busy_timeout**

In `pkg/database/database.go`, after the busy_timeout PRAGMA (after line 116), add the following PRAGMAs:

```go
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
```

**Step 2: Run tests to verify no regressions**

Run: `go test ./pkg/database/... -v`

Expected: All tests pass

**Step 3: Run the full test suite**

Run: `make test`

Expected: All tests pass

**Step 4: Commit**

```bash
git add pkg/database/database.go
git commit -m "$(cat <<'EOF'
[Fix] Add SQLite PRAGMA optimizations for performance

- synchronous=NORMAL: Faster syncs, safe with WAL mode
- cache_size=-65536: 64MB page cache for better reads
- temp_store=MEMORY: In-memory temp tables for complex queries

These compensate for the single-connection serialization.
EOF
)"
```

---

## Task 3: Verify Full System

**Step 1: Run the full check suite**

Run: `make check`

Expected: All checks pass (tests, Go lint, JS lint)

**Step 2: Start the dev server and verify it works**

Run: `make start` (in a separate terminal or briefly)

Expected: Server starts without database errors

**Step 3: Manual verification (optional)**

If time permits, trigger a library scan to verify no lock errors appear in logs.

---

## Summary of Changes

After completing all tasks, `pkg/database/database.go` will have:

1. `sqldb.SetMaxOpenConns(1)` after `sql.OpenDB()` (Task 1)
2. Three new PRAGMAs after `busy_timeout` (Task 2):
   - `PRAGMA synchronous=NORMAL`
   - `PRAGMA cache_size=-65536`
   - `PRAGMA temp_store=MEMORY`

No changes to:
- `pkg/database/retry.go` (keep as safety net)
- `pkg/config/config.go` (no new config needed)
- `shisho.example.yaml` (no new config needed)
