package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// database/sql discards a connection whose query is canceled mid-statement and
// opens a replacement. Pragmas are per connection, so the replacement must get
// them too, or foreign keys stop cascading until the process restarts.
func TestNew_PragmasSurviveConnectionReplacement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cfg := newTestConfig(t)
	cfg.DatabaseBusyTimeout = 4 * time.Second
	db, err := New(cfg)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE books (id INTEGER PRIMARY KEY)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE files (
		id INTEGER PRIMARY KEY,
		book_id INTEGER NOT NULL REFERENCES books (id) ON DELETE CASCADE
	)`)
	require.NoError(t, err)

	// Cancel a query that never finishes on its own.
	cancelCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = db.ExecContext(cancelCtx, `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c`)
	require.Error(t, err)

	var foreignKeys int
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	var busyTimeout int
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout))
	assert.Equal(t, 4000, busyTimeout)

	var synchronous, tempStore, cacheSize int
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous))
	assert.Equal(t, 1, synchronous, "synchronous=NORMAL")
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA temp_store").Scan(&tempStore))
	assert.Equal(t, 2, tempStore, "temp_store=MEMORY")
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA cache_size").Scan(&cacheSize))
	assert.Equal(t, -65536, cacheSize)

	_, err = db.ExecContext(ctx, `INSERT INTO books (id) VALUES (1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO files (id, book_id) VALUES (1, 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM books WHERE id = 1`)
	require.NoError(t, err)

	var files int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM files").Scan(&files))
	assert.Zero(t, files, "deleting the book cascades to its files")
}
