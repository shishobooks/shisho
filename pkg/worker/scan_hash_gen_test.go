package worker

import (
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanJob_QueuesHashGenerationAtEnd verifies that after a scan completes
// for a library, a hash generation job is queued for that library (so any
// files missing fingerprints get hashed in the background).
func TestScanJob_QueuesHashGenerationAtEnd(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Run a scan over all libraries (nil job = no library filter).
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	err := tc.worker.ProcessScanJob(tc.ctx, nil, jobLog)
	require.NoError(t, err)

	// Verify a hash generation job is pending for this library.
	jobType := models.JobTypeHashGeneration
	pending, err := tc.jobService.ListJobs(tc.ctx, jobs.ListJobsOptions{
		Type:     &jobType,
		Statuses: []string{models.JobStatusPending, models.JobStatusInProgress},
	})
	require.NoError(t, err)
	assert.Len(t, pending, 1, "expected exactly one pending hash generation job after scan")
	if len(pending) == 1 {
		assert.Equal(t, models.JobTypeHashGeneration, pending[0].Type)
		assert.NotNil(t, pending[0].LibraryID)
	}
}
