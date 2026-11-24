package migrations

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFileWordCount_QueuesAScanToCountExistingFiles(t *testing.T) {
	t.Parallel()

	scanJobStatuses := func(t *testing.T, withLibrary bool) []string {
		t.Helper()
		ctx := context.Background()
		db := openMigrationTestDB(t)
		migrator := migrateToBefore(ctx, t, db, "20261124190606")
		if withLibrary {
			_, err := db.ExecContext(ctx, `INSERT INTO libraries (id, name, cover_aspect_ratio, created_at, updated_at)
				VALUES (41, 'One', 'book', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
			require.NoError(t, err)
		}
		_, err := migrator.Migrate(ctx)
		require.NoError(t, err)

		var statuses []string
		err = db.NewRaw("SELECT status FROM jobs WHERE type = ? AND library_id IS NULL", models.JobTypeScan).Scan(ctx, &statuses)
		require.NoError(t, err)
		return statuses
	}

	t.Run("one scan of every library", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{models.JobStatusPending}, scanJobStatuses(t, true))
	})

	t.Run("none on a server with no library yet", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, scanJobStatuses(t, false))
	})
}
