package worker

import (
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashGenerationJob_ComputesMissingFingerprints(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create a library with a temp directory
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Write two real files with known contents
	path1 := testgen.WriteFile(t, libraryPath, "book1.epub", []byte("content of book one"))
	path2 := testgen.WriteFile(t, libraryPath, "book2.epub", []byte("content of book two"))

	// Compute expected hashes
	expectedHash1, err := fingerprint.ComputeSHA256(path1)
	require.NoError(t, err)
	expectedHash2, err := fingerprint.ComputeSHA256(path2)
	require.NoError(t, err)

	// Create book and file records pointing at those paths
	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Hash Test Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Hash Test Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err = tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      path1,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	err = tc.bookService.CreateFile(tc.ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      path2,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	err = tc.bookService.CreateFile(tc.ctx, file2)
	require.NoError(t, err)

	// Run the hash generation job
	job := &models.Job{
		Type:       models.JobTypeHashGeneration,
		DataParsed: &models.JobHashGenerationData{LibraryID: 1},
	}
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	err = tc.worker.ProcessHashGenerationJob(tc.ctx, job, jobLog)
	require.NoError(t, err)

	// Verify fingerprints were inserted with the correct sha256 values
	fps1, err := tc.fingerprintService.ListForFile(tc.ctx, file1.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Len(t, fps1, 1)
	assert.Equal(t, expectedHash1, fps1[0].Value)

	fps2, err := tc.fingerprintService.ListForFile(tc.ctx, file2.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Len(t, fps2, 1)
	assert.Equal(t, expectedHash2, fps2[0].Value)
}

func TestHashGenerationJob_Idempotent(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	path1 := testgen.WriteFile(t, libraryPath, "idempotent.epub", []byte("idempotent content"))

	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Idempotent Book",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Idempotent Book",
		AuthorSource: models.DataSourceFilepath,
	}
	err := tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      path1,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	err = tc.bookService.CreateFile(tc.ctx, file1)
	require.NoError(t, err)

	job := &models.Job{
		Type:       models.JobTypeHashGeneration,
		DataParsed: &models.JobHashGenerationData{LibraryID: 1},
	}
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)

	// Run the job twice
	err = tc.worker.ProcessHashGenerationJob(tc.ctx, job, jobLog)
	require.NoError(t, err)
	err = tc.worker.ProcessHashGenerationJob(tc.ctx, job, jobLog)
	require.NoError(t, err)

	// Verify there's only one fingerprint row — no duplicates
	count, err := tc.fingerprintService.CountForFile(tc.ctx, file1.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestHashGenerationJob_MissingFileContinues(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Write only one real file; the other will remain missing on disk
	realPath := testgen.WriteFile(t, libraryPath, "exists.epub", []byte("real content"))
	missingPath := testgen.TempLibraryDir(t) + "/does-not-exist.epub" // non-existent path

	expectedHash, err := fingerprint.ComputeSHA256(realPath)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID:    1,
		Filepath:     libraryPath,
		Title:        "Missing File Test",
		TitleSource:  models.DataSourceFilepath,
		SortTitle:    "Missing File Test",
		AuthorSource: models.DataSourceFilepath,
	}
	err = tc.bookService.CreateBook(tc.ctx, book)
	require.NoError(t, err)

	// File whose path does not exist on disk
	missingFile := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      missingPath,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	err = tc.bookService.CreateFile(tc.ctx, missingFile)
	require.NoError(t, err)

	// File that exists on disk
	realFile := &models.File{
		LibraryID:     1,
		BookID:        book.ID,
		Filepath:      realPath,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1,
	}
	err = tc.bookService.CreateFile(tc.ctx, realFile)
	require.NoError(t, err)

	// Run the job — it should not return an error despite the missing file
	job := &models.Job{
		Type:       models.JobTypeHashGeneration,
		DataParsed: &models.JobHashGenerationData{LibraryID: 1},
	}
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	err = tc.worker.ProcessHashGenerationJob(tc.ctx, job, jobLog)
	require.NoError(t, err)

	// The real file should have been hashed
	fps, err := tc.fingerprintService.ListForFile(tc.ctx, realFile.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Len(t, fps, 1)
	assert.Equal(t, expectedHash, fps[0].Value)

	// The missing file should have been skipped (no fingerprint)
	missingFps, err := tc.fingerprintService.ListForFile(tc.ctx, missingFile.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	assert.Empty(t, missingFps)
}

func TestEnsureHashGenerationJob_Dedupes(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Call EnsureHashGenerationJob twice for the same library
	err := EnsureHashGenerationJob(tc.ctx, tc.jobService, 1)
	require.NoError(t, err)
	err = EnsureHashGenerationJob(tc.ctx, tc.jobService, 1)
	require.NoError(t, err)

	// Query jobs for pending/in-progress hash generation for library 1
	jobType := models.JobTypeHashGeneration
	pendingJobs, err := tc.jobService.ListJobs(tc.ctx, jobs.ListJobsOptions{
		Type:     &jobType,
		Statuses: []string{models.JobStatusPending, models.JobStatusInProgress},
	})
	require.NoError(t, err)

	// There should be exactly one job — no duplicates
	assert.Len(t, pendingJobs, 1)
	assert.Equal(t, models.JobTypeHashGeneration, pendingJobs[0].Type)
	assert.Equal(t, 1, *pendingJobs[0].LibraryID)
}
