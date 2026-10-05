package testdb

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// A bare ":memory:" database is private to one connection, and PRAGMA
// foreign_keys applies per connection, so a second pooled connection would see
// an empty database with foreign keys off. The pool is pinned to one
// connection, as in production.
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

// A connection whose query is canceled mid-statement is replaced, and foreign
// keys apply per connection, so the replacement must turn them on too.
func TestNew_ForeignKeysSurviveConnectionReplacement(t *testing.T) {
	t.Parallel()
	db := New(t)
	ctx := context.Background()

	cancelCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err := db.ExecContext(cancelCtx, `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c) SELECT count(*) FROM c`)
	require.Error(t, err)

	var foreignKeys int
	require.NoError(t, db.NewRaw("PRAGMA foreign_keys").Scan(ctx, &foreignKeys))
	assert.Equal(t, 1, foreignKeys)
}
