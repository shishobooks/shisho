package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDeviceDeliveryRules_PresetTypesStartOnTheirPresetAndRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261021000000")

	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'reader', 'hash', 1)`)
	require.NoError(t, err)
	for _, device := range [][2]string{{"kobo-1", "kobo"}, {"kindle-1", "kindle"}, {"browser-1", "browser"}} {
		_, err = db.ExecContext(ctx, `INSERT INTO devices (id, user_id, name, device_type, credential, created_at, updated_at)
			VALUES (?, 1, 'Device', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, device[0], device[1], device[0])
		require.NoError(t, err)
	}

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	modes := map[string]string{}
	var rows []struct {
		ID   string `bun:"id"`
		Mode string `bun:"delivery_rules_mode"`
	}
	require.NoError(t, db.NewRaw(`SELECT id, delivery_rules_mode FROM devices`).Scan(ctx, &rows))
	for _, row := range rows {
		modes[row.ID] = row.Mode
	}
	assert.Equal(t, map[string]string{"kobo-1": "preset", "kindle-1": "preset", "browser-1": "default"}, modes)

	_, err = db.ExecContext(ctx, `INSERT INTO device_delivery_rules (device_id, source_file_type, delivered_file_type, position) VALUES ('browser-1', 'epub', 'epub', 0)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO device_delivery_rules (device_id, source_file_type, delivered_file_type, position) VALUES ('browser-1', 'epub', 'kepub', 1)`)
	require.Error(t, err, "a second rule for one source File Type was saved")
	_, err = db.ExecContext(ctx, `DELETE FROM devices WHERE id = 'browser-1'`)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.NewRaw(`SELECT COUNT(*) FROM device_delivery_rules`).Scan(ctx, &count))
	assert.Zero(t, count)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT 1 FROM device_delivery_rules`)
	require.Error(t, err)
	_, err = db.ExecContext(ctx, `SELECT delivery_rules_mode FROM devices`)
	require.Error(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
