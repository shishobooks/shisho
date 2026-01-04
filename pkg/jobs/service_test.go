package jobs

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestHasActiveJobByType_NoJobs(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJobByType_PendingJob(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a pending scan job
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJobByType_InProgressJob(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create an in-progress scan job
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusInProgress,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJobByType_CompletedJob(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a completed scan job
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJobByType_DifferentType(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a pending export job
	job := &models.Job{
		Type:       models.JobTypeExport,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobExportData{},
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// Should not find an active scan job
	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.False(t, hasActive)

	// Should find an active export job
	hasActive, err = svc.HasActiveJobByType(ctx, models.JobTypeExport)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJobByType_MultipleJobs(t *testing.T) {
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a completed scan job
	job1 := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, job1)
	require.NoError(t, err)

	// Create a pending scan job
	job2 := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err = svc.CreateJob(ctx, job2)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJobByType(ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}
