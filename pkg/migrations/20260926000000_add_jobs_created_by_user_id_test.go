package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

// A job keeps its row when its creator is deleted, and the column rolls back
// cleanly so the migration can be re-applied.
func TestAddJobsCreatedByUserID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	for _, query := range []string{
		"PRAGMA foreign_keys = ON",
		"CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT NOT NULL)",
		"CREATE TABLE jobs (id INTEGER PRIMARY KEY, type TEXT NOT NULL)",
		"INSERT INTO users VALUES (1, 'creator')",
		"INSERT INTO jobs VALUES (1, 'scan')",
	} {
		_, err := db.ExecContext(ctx, query)
		require.NoError(t, err)
	}
	migrationSet := migrate.NewMigrations()
	for _, migration := range Migrations.Sorted() {
		if migration.Name == "20260926000000" {
			migrationSet.Add(migration)
		}
	}
	require.Len(t, migrationSet.Sorted(), 1)
	migrator := newMigrator(db, migrationSet)
	require.NoError(t, migrator.Init(ctx))

	columns := func() []string {
		t.Helper()
		var names []string
		require.NoError(t, db.NewRaw("SELECT name FROM pragma_table_info('jobs')").Scan(ctx, &names))
		return names
	}
	indexes := func() []string {
		t.Helper()
		var names []string
		require.NoError(t, db.NewRaw("SELECT name FROM pragma_index_list('jobs')").Scan(ctx, &names))
		return names
	}

	for range 2 {
		_, err = migrator.Migrate(ctx)
		require.NoError(t, err)
		assert.Contains(t, columns(), "created_by_user_id")
		assert.Contains(t, indexes(), "ix_jobs_created_by_user_id")

		var createdBy *int
		require.NoError(t, db.NewRaw("SELECT created_by_user_id FROM jobs WHERE id = 1").Scan(ctx, &createdBy))
		assert.Nil(t, createdBy, "existing jobs have no recorded creator")

		_, err = db.ExecContext(ctx, "INSERT INTO jobs (id, type, created_by_user_id) VALUES (2, 'bulk_download', 1)")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "INSERT INTO jobs (id, type, created_by_user_id) VALUES (3, 'bulk_download', 404)")
		require.ErrorContains(t, err, "FOREIGN KEY")

		_, err = db.ExecContext(ctx, "DELETE FROM users WHERE id = 1")
		require.NoError(t, err)
		var count int
		require.NoError(t, db.NewRaw("SELECT COUNT(*) FROM jobs WHERE id = 2 AND created_by_user_id IS NULL").Scan(ctx, &count))
		assert.Equal(t, 1, count, "deleting the creator keeps the job and clears the reference")

		_, err = migrator.Rollback(ctx)
		require.NoError(t, err)
		assert.NotContains(t, columns(), "created_by_user_id")
		assert.NotContains(t, indexes(), "ix_jobs_created_by_user_id")

		// Reset for the second pass.
		for _, query := range []string{"DELETE FROM jobs WHERE id = 2", "INSERT INTO users VALUES (1, 'creator')"} {
			_, err = db.ExecContext(ctx, query)
			require.NoError(t, err)
		}
	}
}
