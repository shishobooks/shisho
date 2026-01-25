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

func TestHasActiveJob_NilLibraryID_NoJobs(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_PendingJob(t *testing.T) {
	t.Parallel()
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

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_InProgressJob(t *testing.T) {
	t.Parallel()
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

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_CompletedJob(t *testing.T) {
	t.Parallel()
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

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_DifferentType(t *testing.T) {
	t.Parallel()
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
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.False(t, hasActive)

	// Should find an active export job
	hasActive, err = svc.HasActiveJob(ctx, models.JobTypeExport, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_MultipleJobs(t *testing.T) {
	t.Parallel()
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

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_NoJobs(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_MatchingLibrary(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_DifferentLibrary(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID1 := 1
	libraryID2 := 2
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID1,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// Should not find active job for different library
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID2)
	require.NoError(t, err)
	assert.False(t, hasActive)
}

func TestHasActiveJob_WithLibraryID_GlobalJobBlocks(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create a global scan job (library_id is NULL)
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  nil,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// Should block any library-specific scan
	libraryID := 1
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, &libraryID)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestHasActiveJob_NilLibraryID_ChecksAny(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID := 1
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID,
	}
	err := svc.CreateJob(ctx, job)
	require.NoError(t, err)

	// With nil libraryID, should find any active scan
	hasActive, err := svc.HasActiveJob(ctx, models.JobTypeScan, nil)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestListJobs_FilterByType(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Create one scan job and one export job
	scanJob := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
	}
	err := svc.CreateJob(ctx, scanJob)
	require.NoError(t, err)

	exportJob := &models.Job{
		Type:       models.JobTypeExport,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobExportData{},
	}
	err = svc.CreateJob(ctx, exportJob)
	require.NoError(t, err)

	// Filter by scan type
	scanType := models.JobTypeScan
	jobs, err := svc.ListJobs(ctx, ListJobsOptions{Type: &scanType})
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, models.JobTypeScan, jobs[0].Type)
}

func TestListJobs_FilterByLibraryIDOrGlobal(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	libraryID1 := 1
	libraryID2 := 2

	// Create jobs: global, library 1, library 2
	globalJob := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  nil,
	}
	err := svc.CreateJob(ctx, globalJob)
	require.NoError(t, err)

	lib1Job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID1,
	}
	err = svc.CreateJob(ctx, lib1Job)
	require.NoError(t, err)

	lib2Job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
		LibraryID:  &libraryID2,
	}
	err = svc.CreateJob(ctx, lib2Job)
	require.NoError(t, err)

	// Filter for library 1 or global - should get global and lib1 jobs
	jobs, err := svc.ListJobs(ctx, ListJobsOptions{LibraryIDOrGlobal: &libraryID1})
	require.NoError(t, err)
	assert.Len(t, jobs, 2)

	// Verify we got the right jobs
	var foundGlobal, foundLib1 bool
	for _, j := range jobs {
		if j.LibraryID == nil {
			foundGlobal = true
		} else if *j.LibraryID == libraryID1 {
			foundLib1 = true
		}
	}
	assert.True(t, foundGlobal, "should include global job")
	assert.True(t, foundLib1, "should include library 1 job")
}
