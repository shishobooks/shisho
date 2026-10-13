package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

const nestCacheLimitsMigrationName = "20261013000000"

// The saved cover thumbnail limit moves under the cache's id, beside the
// limits of the other caches, so the saved policy survives the upgrade.
func TestNestCacheLimits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, nestCacheLimitsMigrationName)

	_, err := db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('cache', '{"cover_thumbnail_max_size_gb":2.5}', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	value := func() string {
		var v string
		require.NoError(t, db.NewRaw(`SELECT value FROM app_settings WHERE key = 'cache'`).Scan(ctx, &v))
		return v
	}

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{"cover_thumbnails":{"max_size_gb":2.5,"unlimited":false}}`, value())

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{"cover_thumbnail_max_size_gb":2.5}`, value())
}

// No saved row stays absent; a document already in the new shape is left
// alone; rollback turns a lifted limit into the old maximum and drops limits
// the old shape cannot hold.
func TestNestCacheLimits_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	value := func(db *bun.DB) string {
		var v string
		require.NoError(t, db.NewRaw(`SELECT value FROM app_settings WHERE key = 'cache'`).Scan(ctx, &v))
		return v
	}

	t.Run("no saved row", func(t *testing.T) {
		t.Parallel()
		db := openMigrationTestDB(t)
		migrator := migrateToBefore(ctx, t, db, nestCacheLimitsMigrationName)
		_, err := migrator.Migrate(ctx)
		require.NoError(t, err)
		count, err := db.NewSelect().Table("app_settings").Where("key = 'cache'").Count(ctx)
		require.NoError(t, err)
		assert.Zero(t, count)
		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
	})

	for _, tc := range []struct {
		name, stored, rolledBack string
	}{
		{"lifted thumbnail limit", `{"cover_thumbnails":{"max_size_gb":2,"unlimited":true},"downloads":{"max_size_gb":9,"unlimited":false}}`, `{"cover_thumbnail_max_size_gb":1024}`},
		{"no thumbnail limit", `{"downloads":{"max_size_gb":9,"unlimited":false}}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := openMigrationTestDB(t)
			migrator := migrateToBefore(ctx, t, db, nestCacheLimitsMigrationName)
			_, err := migrator.Migrate(ctx)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('cache', ?, CURRENT_TIMESTAMP)`, tc.stored)
			require.NoError(t, err)

			_, err = migrator.Rollback(ctx)
			require.NoError(t, err)
			assert.JSONEq(t, tc.rolledBack, value(db))
		})
	}

	t.Run("already nested", func(t *testing.T) {
		t.Parallel()
		db := openMigrationTestDB(t)
		migrator := migrateToBefore(ctx, t, db, nestCacheLimitsMigrationName)
		nested := `{"cover_thumbnails":{"max_size_gb":3,"unlimited":false}}`
		_, err := db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES ('cache', ?, CURRENT_TIMESTAMP)`, nested)
		require.NoError(t, err)
		_, err = migrator.Migrate(ctx)
		require.NoError(t, err)
		assert.JSONEq(t, nested, value(db))
	})
}
