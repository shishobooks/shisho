package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const addShishoAppSync = "20261127040931"

// Bun records a rollback after the down migration commits, so a rollback
// stopped between the two runs the down migration again.
func TestAddShishoAppSync_RollsBackAgainAfterAnInterruptedRecord(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, addShishoAppSync)
	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES (?, 1, CURRENT_TIMESTAMP)`, addShishoAppSync)
	require.NoError(t, err)
	_, err = migrator.Rollback(ctx)
	require.NoError(t, err, "the second rollback finds it already done")
}

// The migrations it replaced were never released, but a development
// database may have run them: this one would skip itself there, leaving
// that database's older change log in place, so it refuses instead.
func TestAddShishoAppSync_RefusesADatabaseThatRanTheMigrationsItReplaced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, addShishoAppSync)
	_, err := db.ExecContext(ctx, `INSERT INTO bun_migrations (name, group_id, migrated_at) VALUES ('20261120042258', 1, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	_, err = migrator.Migrate(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "20261120042258")
}
