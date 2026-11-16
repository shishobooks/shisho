package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropViewerPlaybackSpeed_KeepsSettingsAndRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261116230712")
	_, err := db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'listener', 'x', 2)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO user_settings (user_id, gallery_size, viewer_playback_speed) VALUES (1, 'l', 1.5)`)
	require.NoError(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT viewer_playback_speed FROM user_settings`)
	require.Error(t, err, "the setting is still in the schema")
	var size string
	require.NoError(t, db.NewRaw(`SELECT gallery_size FROM user_settings WHERE user_id = 1`).Scan(ctx, &size))
	assert.Equal(t, "l", size)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	var speed float64
	require.NoError(t, db.NewRaw(`SELECT viewer_playback_speed FROM user_settings WHERE user_id = 1`).Scan(ctx, &speed))
	assert.InDelta(t, 1.0, speed, 0)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
