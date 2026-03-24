package worker

import (
	"context"
	"testing"

	"github.com/segmentio/encoding/json"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBulkFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("deterministic for same inputs", func(t *testing.T) {
		t.Parallel()
		hashes := []string{"abc123", "def456", "ghi789"}
		hash1 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		hash2 := ComputeBulkFingerprint([]int{1, 2, 3}, hashes)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("sorts file IDs for consistency", func(t *testing.T) {
		t.Parallel()
		hashes1 := []string{"abc123", "def456"}
		hashes2 := []string{"def456", "abc123"}
		hash1 := ComputeBulkFingerprint([]int{1, 2}, hashes1)
		hash2 := ComputeBulkFingerprint([]int{2, 1}, hashes2)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different hashes produce different fingerprint", func(t *testing.T) {
		t.Parallel()
		hash1 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "def"})
		hash2 := ComputeBulkFingerprint([]int{1, 2}, []string{"abc", "xyz"})
		assert.NotEqual(t, hash1, hash2)
	})
}

func TestDeduplicateFilenames(t *testing.T) {
	t.Parallel()

	t.Run("no duplicates", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book A.epub", 2: "Book B.epub"}
		result := DeduplicateFilenames(names)
		assert.Equal(t, "Book A.epub", result[1])
		assert.Equal(t, "Book B.epub", result[2])
	})

	t.Run("duplicates get numbered", func(t *testing.T) {
		t.Parallel()
		names := map[int]string{1: "Book.epub", 2: "Book.epub", 3: "Book.epub"}
		result := DeduplicateFilenames(names)
		values := []string{result[1], result[2], result[3]}
		require.Contains(t, values, "Book.epub")
		require.Contains(t, values, "Book (2).epub")
		require.Contains(t, values, "Book (3).epub")
	})
}

func TestProcessBulkDownloadJob_EmptyFileIDs(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	data := models.JobBulkDownloadData{FileIDs: []int{}}
	dataBytes, err := json.Marshal(data)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusInProgress,
		Data:   string(dataBytes),
	}
	jobLog := tc.worker.jobLogService.NewJobLogger(tc.ctx, 0, tc.worker.log)

	err = tc.worker.ProcessBulkDownloadJob(tc.ctx, job, jobLog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no file IDs provided")
}

func TestProcessBulkDownloadJob_InvalidFileIDs(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	data := models.JobBulkDownloadData{FileIDs: []int{99999, 99998}}
	dataBytes, err := json.Marshal(data)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusInProgress,
		Data:   string(dataBytes),
	}
	jobLog := tc.worker.jobLogService.NewJobLogger(tc.ctx, 0, tc.worker.log)

	err = tc.worker.ProcessBulkDownloadJob(tc.ctx, job, jobLog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid files found")
}

func TestProcessBulkDownloadJob_CancelledContext(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(tc.ctx)
	cancel()

	data := models.JobBulkDownloadData{FileIDs: []int{1}}
	dataBytes, err := json.Marshal(data)
	require.NoError(t, err)

	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusInProgress,
		Data:   string(dataBytes),
	}
	jobLog := tc.worker.jobLogService.NewJobLogger(ctx, 0, tc.worker.log)

	// With no downloadCache set, this will fail at file retrieval, but that's fine —
	// the point is it processes without panicking on a cancelled context
	err = tc.worker.ProcessBulkDownloadJob(ctx, job, jobLog)
	require.Error(t, err)
}

func TestProcessBulkDownloadJob_InvalidJobData(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	job := &models.Job{
		Type:   models.JobTypeBulkDownload,
		Status: models.JobStatusInProgress,
		Data:   "not valid json",
	}
	jobLog := tc.worker.jobLogService.NewJobLogger(tc.ctx, 0, tc.worker.log)

	err := tc.worker.ProcessBulkDownloadJob(tc.ctx, job, jobLog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse bulk download job data")
}

func TestSaveBulkDownloadResult(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a job in the database
	job := &models.Job{
		Type:       models.JobTypeBulkDownload,
		Status:     models.JobStatusInProgress,
		DataParsed: &models.JobBulkDownloadData{FileIDs: []int{1, 2}},
	}
	err := tc.worker.jobService.CreateJob(tc.ctx, job)
	require.NoError(t, err)

	// Save result
	data := &models.JobBulkDownloadData{
		FileIDs:            []int{1, 2},
		EstimatedSizeBytes: 1000,
		ZipFilename:        "test.zip",
		SizeBytes:          950,
		FileCount:          2,
		FingerprintHash:    "abc123",
	}
	err = tc.worker.saveBulkDownloadResult(tc.ctx, job, data)
	require.NoError(t, err)

	// Verify the job data was updated
	var savedData models.JobBulkDownloadData
	err = json.Unmarshal([]byte(job.Data), &savedData)
	require.NoError(t, err)
	assert.Equal(t, "test.zip", savedData.ZipFilename)
	assert.Equal(t, int64(950), savedData.SizeBytes)
	assert.Equal(t, 2, savedData.FileCount)
	assert.Equal(t, "abc123", savedData.FingerprintHash)
	// Input fields are preserved
	assert.Equal(t, []int{1, 2}, savedData.FileIDs)
	assert.Equal(t, int64(1000), savedData.EstimatedSizeBytes)
}
