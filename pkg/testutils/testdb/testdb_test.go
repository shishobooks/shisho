package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestNew_MigratesWithForeignKeysOn(t *testing.T) {
	t.Parallel()
	db := New(t)
	ctx := context.Background()

	var foreignKeys int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	count, err := db.NewSelect().Model((*models.Library)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "the libraries table exists and starts empty")

	// A Library Path with no Library violates library_paths.library_id.
	now := time.Now()
	_, err = db.NewInsert().Model(&models.LibraryPath{CreatedAt: now, UpdatedAt: now, LibraryID: 999, Filepath: "/orphan"}).Exec(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FOREIGN KEY constraint failed")
}

// New copies a template migrated once, so the copy must carry the record of
// applied migrations along with the schema, or code that migrates the database
// it is handed would rerun them.
func TestNew_RecordsEveryMigrationApplied(t *testing.T) {
	t.Parallel()
	db := New(t)

	group, err := migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)
	assert.True(t, group.IsZero(), "no migration is left to apply")
}

// migrate falls back to running every migration when the connection has no
// backup API, which passes every test while losing the template's speedup.
func TestConnections_SupportRestoringTheTemplate(t *testing.T) {
	t.Parallel()
	connector, err := sqliteconn.NewConnector("file:testdb-restorer?mode=memory&cache=shared")
	require.NoError(t, err)
	conn, err := connector.Connect(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	assert.Implements(t, (*restorer)(nil), conn)
}

func TestNew_PinsOneConnection(t *testing.T) {
	t.Parallel()
	db := New(t)

	assert.Equal(t, 1, db.DB.Stats().MaxOpenConnections)
}

func TestNew_DatabasesAreIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first := New(t)
	second := New(t)

	_, err := first.NewInsert().Model(&models.Library{Name: "Only in first", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}).Exec(ctx)
	require.NoError(t, err)

	count, err := second.NewSelect().Model((*models.Library)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

// interruptStatement cancels a statement that never finishes on its own, so
// database/sql discards the connection it ran on. The driver reports only the
// context error, so the test checks the pool instead: a cancel that lands
// before the statement starts never takes a connection, and would let the test
// pass without a discard.
func interruptStatement(t *testing.T, db *bun.DB) {
	t.Helper()
	// The template copy runs outside the pool, so open a pool connection first.
	require.NoError(t, db.PingContext(t.Context()))
	open := db.DB.Stats().OpenConnections
	require.Positive(t, open)

	cancelCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err := db.ExecContext(cancelCtx, `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, open-1, db.DB.Stats().OpenConnections, "the interrupted connection is discarded")
}

// A connection whose query is canceled mid-statement is replaced. Foreign keys
// apply per connection, so the replacement must turn them on too, and it must
// open the same database, so the schema is still there.
func TestNew_SurvivesConnectionReplacement(t *testing.T) {
	t.Parallel()
	db := New(t)
	ctx := context.Background()

	interruptStatement(t, db)

	var foreignKeys int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	_, err := db.NewSelect().Model((*models.Library)(nil)).Count(ctx)
	require.NoError(t, err)
}

func TestNewShared_AllowsManyConnections(t *testing.T) {
	t.Parallel()
	db := NewShared(t)

	assert.Zero(t, db.DB.Stats().MaxOpenConnections)
}

func TestNewShared_SurvivesConnectionReplacement(t *testing.T) {
	t.Parallel()
	db := NewShared(t)
	ctx := context.Background()

	interruptStatement(t, db)

	var foreignKeys int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &foreignKeys))
	assert.Equal(t, 1, foreignKeys)

	_, err := db.NewSelect().Model((*models.Library)(nil)).Count(ctx)
	require.NoError(t, err)
}

func TestNewShared_DatabasesAreIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first := NewShared(t)
	second := NewShared(t)

	_, err := first.NewInsert().Model(&models.Library{Name: "Only in first", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}).Exec(ctx)
	require.NoError(t, err)

	count, err := second.NewSelect().Model((*models.Library)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}
