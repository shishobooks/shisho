package migrations

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddKOReaderSync_QueuesTheBackfillForEachLibrary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := openMigrationTestDB(t)
	migrator := migrateToBefore(ctx, t, db, "20261105000000")

	_, err := db.ExecContext(ctx, `INSERT INTO libraries (id, name, cover_aspect_ratio, created_at, updated_at)
		VALUES (41, 'One', 'book', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), (42, 'Two', 'book', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = migrator.Migrate(ctx)
	require.NoError(t, err)

	var jobs []struct {
		Status    string `bun:"status"`
		LibraryID int    `bun:"library_id"`
		Data      string `bun:"data"`
	}
	err = db.NewRaw("SELECT status, library_id, data FROM jobs WHERE type = ? ORDER BY library_id", models.JobTypeHashGeneration).Scan(ctx, &jobs)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	for i, want := range []int{41, 42} {
		assert.Equal(t, models.JobStatusPending, jobs[i].Status)
		assert.Equal(t, want, jobs[i].LibraryID)
		var data models.JobHashGenerationData
		require.NoError(t, json.Unmarshal([]byte(jobs[i].Data), &data))
		assert.Equal(t, want, data.LibraryID)
	}
}
