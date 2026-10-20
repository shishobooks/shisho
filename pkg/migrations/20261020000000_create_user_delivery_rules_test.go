package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateUserDeliveryRules_CascadesFromUsersAndRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261020000000")

	_, err := migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'reader', 'hash', 1)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO user_delivery_rules (user_id, source_file_type, delivered_file_type, position) VALUES (1, 'epub', 'kepub', 0)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO user_delivery_rules (user_id, source_file_type, delivered_file_type, position) VALUES (1, 'epub', 'epub', 1)`)
	require.Error(t, err, "a second rule for one source File Type was saved")

	_, err = db.ExecContext(ctx, `DELETE FROM users WHERE id = 1`)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM user_delivery_rules`).Scan(ctx, &count))
	require.Zero(t, count)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT 1 FROM user_delivery_rules`)
	require.Error(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
