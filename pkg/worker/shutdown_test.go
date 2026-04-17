package worker

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShutdown_CancelsInFlightJob verifies that when Worker.Shutdown is called
// while a job is executing, the job's context is cancelled so the job handler
// can return early. Without this, long-running jobs (e.g. hash generation over
// a large library) block shutdown past air's kill_delay, leaving the old API
// process holding port 3689 and making the new process fail to bind with
// "address already in use".
func TestShutdown_CancelsInFlightJob(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Use New() so the internal channels and shutdown context are initialized
	// the same way they are in production. The test context's worker is built
	// manually and doesn't have Start/Shutdown-ready plumbing.
	cfg := &config.Config{WorkerProcesses: 1}
	w := New(cfg, tc.db, nil, nil, nil)

	// Install a test-only process function under the scan job type that blocks
	// until its context is cancelled. This stands in for a slow, real job
	// (hash generation, scan, bulk download) so we can exercise the shutdown
	// path without needing real file I/O.
	jobStarted := make(chan struct{})
	jobCtxCh := make(chan context.Context, 1)
	w.processFuncs[models.JobTypeScan] = func(ctx context.Context, _ *models.Job, _ *joblogs.JobLogger) error {
		close(jobStarted)
		jobCtxCh <- ctx
		<-ctx.Done()
		return ctx.Err()
	}

	// Insert a real job row so the UpdateJob call inside processJobs succeeds.
	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	w.Start()
	// Push the job onto the internal queue directly; we don't want to wait for
	// the 5-second fetchJobs tick.
	w.queue <- job

	// Wait for the job to actually start executing so we know Shutdown is
	// racing against in-flight work, not an idle worker.
	select {
	case <-jobStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("job never started executing")
	}

	// Call Shutdown in a goroutine with a generous timeout so a broken
	// implementation (Shutdown blocks forever) fails the test instead of
	// hanging the whole test run.
	shutdownDone := make(chan struct{})
	start := time.Now()
	go func() {
		w.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
		elapsed := time.Since(start)
		// Shutdown should complete promptly because the job handler returns
		// as soon as its context is cancelled. We allow a comfortable margin
		// so this isn't flaky on slow CI, but catch the real bug (infinite
		// wait) decisively.
		assert.Less(t, elapsed, 500*time.Millisecond, "Shutdown should complete quickly after cancelling the in-flight job")
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not complete within 3 seconds; in-flight job was not cancelled")
	}

	// Verify the job's context was actually cancelled (not just ignored).
	select {
	case ctx := <-jobCtxCh:
		require.Error(t, ctx.Err(), "job context should have been cancelled during shutdown")
	default:
		t.Error("job never reported its context")
	}
}

// TestShutdown_PersistsFailedJobStatus verifies that when a handler returns an
// error during shutdown, the job row is still updated to "failed" in the DB.
// Without a separate un-cancelled context for the status write, UpdateJob uses
// the cancelled job context and fails silently, leaving the row stuck in
// "in_progress". That orphan row does self-heal on restart (processID
// regenerates, fetchJobs re-picks it up), but a completed-but-unpersisted job
// would otherwise re-run wastefully — bulk download in particular would
// re-render its zip.
func TestShutdown_PersistsFailedJobStatus(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	cfg := &config.Config{WorkerProcesses: 1}
	w := New(cfg, tc.db, nil, nil, nil)

	// Handler returns ctx.Err() once cancelled — same shape as a real job
	// that observes shutdown and bails out.
	jobStarted := make(chan struct{})
	w.processFuncs[models.JobTypeScan] = func(ctx context.Context, _ *models.Job, _ *joblogs.JobLogger) error {
		close(jobStarted)
		<-ctx.Done()
		return ctx.Err()
	}

	job := &models.Job{
		Type:       models.JobTypeScan,
		Status:     models.JobStatusPending,
		DataParsed: &models.JobScanData{},
	}
	err := tc.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	w.Start()
	w.queue <- job

	select {
	case <-jobStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("job never started executing")
	}

	shutdownDone := make(chan struct{})
	go func() {
		w.Shutdown()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}

	// After shutdown, the job row should reflect that the handler completed
	// with a failure — NOT be stuck in "in_progress" from the initial update.
	retrieved, err := tc.jobService.RetrieveJob(tc.ctx, jobs.RetrieveJobOptions{ID: &job.ID})
	require.NoError(t, err)
	assert.Equal(t, models.JobStatusFailed, retrieved.Status,
		"failed status should persist even when the job ctx was cancelled during shutdown")
}
