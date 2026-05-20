package migrations

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
	"github.com/uptrace/bun/migrate"
)

func TestNewMigratorMarksFailedMigrationUnapplied(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqldb.Close())
	})

	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	testMigrations := migrate.NewMigrations()
	testMigrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TABLE created_before_failure (id INTEGER)`)
			require.NoError(t, err)

			return errors.New("migration failed")
		},
		func(context.Context, *bun.DB) error {
			return nil
		},
	)

	migrator := newMigrator(db, testMigrations)
	require.NoError(t, migrator.Init(ctx))

	_, err = migrator.Migrate(ctx)
	require.Error(t, err)

	var appliedCount int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bun_migrations`).Scan(&appliedCount))
	require.Zero(t, appliedCount)
}
