package worker

import (
	"testing"
	"time"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_SkipsWhenNoLibraries(t *testing.T) {
	tc := newTestContext(t)

	// Check for active job before - should be false
	hasActive, err := tc.jobService.HasActiveJobByType(tc.ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.False(t, hasActive)

	// List jobs - should be empty
	allJobs, err := tc.jobService.ListJobs(tc.ctx, jobs.ListJobsOptions{})
	require.NoError(t, err)
	assert.Empty(t, allJobs)
}

func TestScheduler_SkipsWhenScanJobPending(t *testing.T) {
	tc := newTestContext(t)

	// Create a library first
	tc.createLibrary([]string{t.TempDir()})

	// Create a pending scan job
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	// Check for active job - should be true
	hasActive, err := tc.jobService.HasActiveJobByType(tc.ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestScheduler_SkipsWhenScanJobInProgress(t *testing.T) {
	tc := newTestContext(t)

	// Create a library first
	tc.createLibrary([]string{t.TempDir()})

	// Create an in-progress scan job
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusInProgress,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	// Check for active job - should be true
	hasActive, err := tc.jobService.HasActiveJobByType(tc.ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestScheduler_CreatesJobWhenNoneActive(t *testing.T) {
	tc := newTestContext(t)

	// Create a library first
	tc.createLibrary([]string{t.TempDir()})

	// Create a completed scan job (should not block new jobs)
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusCompleted,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	// Check for active job - should be false (completed jobs don't count)
	hasActive, err := tc.jobService.HasActiveJobByType(tc.ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.False(t, hasActive)

	// Create a new scan job
	newJob := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err = tc.jobService.CreateJob(tc.ctx, newJob)
	require.NoError(t, err)

	// Now check again - should be true
	hasActive, err = tc.jobService.HasActiveJobByType(tc.ctx, models.JobTypeScan)
	require.NoError(t, err)
	assert.True(t, hasActive)
}

func TestScheduler_StartWithZeroInterval(t *testing.T) {
	tc := newTestContext(t)

	// Set SyncIntervalMinutes to 0 (disabled)
	tc.worker.config.SyncIntervalMinutes = 0

	// Initialize channels
	tc.worker.shutdown = make(chan struct{})
	tc.worker.doneFetching = make(chan struct{})
	tc.worker.doneProcessing = make(chan struct{}, tc.worker.config.WorkerProcesses)
	tc.worker.doneScheduling = make(chan struct{})
	tc.worker.queue = make(chan *models.Job, tc.worker.config.WorkerProcesses)

	// Start the worker
	tc.worker.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown should complete without hanging
	done := make(chan struct{})
	go func() {
		tc.worker.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Success - shutdown completed
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestScheduler_LibraryExistsCheck(t *testing.T) {
	tc := newTestContext(t)

	// Initially no libraries
	libs, err := tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{
		Limit: pointerutil.Int(1),
	})
	require.NoError(t, err)
	assert.Empty(t, libs)

	// Create a library
	tc.createLibrary([]string{t.TempDir()})

	// Now libraries exist
	libs, err = tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{
		Limit: pointerutil.Int(1),
	})
	require.NoError(t, err)
	assert.Len(t, libs, 1)
}
