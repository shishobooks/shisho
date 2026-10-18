package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddKoboSyncPointBooksCoverModifiedAt_RoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261018000001")

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT cover_modified_at FROM kobo_sync_point_books`)
	require.NoError(t, err)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT cover_modified_at FROM kobo_sync_point_books`)
	require.Error(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
