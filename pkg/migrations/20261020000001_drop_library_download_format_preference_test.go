package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropLibraryDownloadFormatPreference_KeepsLibrariesAndRoundTrips(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261020000001")
	_, err := db.ExecContext(ctx, `INSERT INTO libraries (id, name, cover_aspect_ratio, download_format_preference) VALUES (1, 'Comics', 'book', 'kepub')`)
	require.NoError(t, err)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `SELECT download_format_preference FROM libraries`)
	require.Error(t, err, "the setting is still in the schema")
	var name string
	require.NoError(t, db.NewRaw(`SELECT name FROM libraries WHERE id = 1`).Scan(ctx, &name))
	assert.Equal(t, "Comics", name)

	_, err = migrator.Rollback(ctx)
	require.NoError(t, err)
	var preference string
	require.NoError(t, db.NewRaw(`SELECT download_format_preference FROM libraries WHERE id = 1`).Scan(ctx, &preference))
	assert.Equal(t, "original", preference)

	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)
}
