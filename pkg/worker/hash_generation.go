package worker

import (
	"context"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
)

// ProcessHashGenerationJob computes and stores sha256 fingerprints for all
// files in a library that don't yet have one. Per-file errors are logged and
// skipped so a single unreadable file cannot fail the whole job.
func (w *Worker) ProcessHashGenerationJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	data, ok := job.DataParsed.(*models.JobHashGenerationData)
	if !ok || data == nil {
		return errors.New("invalid or missing job data for hash generation job")
	}

	libraryID := data.LibraryID
	jobLog.Info("processing hash generation job", logger.Data{"library_id": libraryID})

	fileIDs, err := w.fingerprintService.ListFilesMissingAlgorithm(ctx, libraryID, models.FingerprintAlgorithmSHA256)
	if err != nil {
		return errors.Wrap(err, "list files missing sha256")
	}

	total := len(fileIDs)
	jobLog.Info("files needing sha256 fingerprint", logger.Data{"count": total, "library_id": libraryID})

	if total == 0 {
		jobLog.Info("no files need hashing, done", logger.Data{"library_id": libraryID})
		return nil
	}

	// Worker pool size: at least 4 goroutines, at most NumCPU.
	numWorkers := runtime.NumCPU()
	if numWorkers < 4 {
		numWorkers = 4
	}

	type workItem struct {
		fileID int
	}

	workCh := make(chan workItem, numWorkers)
	var processed int64

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				if err := w.hashOneFile(ctx, item.fileID, jobLog); err != nil {
					// hashOneFile logs per-file errors internally; non-nil return means
					// a hard error worth noting here too.
					jobLog.Warn("hash generation failed for file", logger.Data{
						"file_id": item.fileID,
						"error":   err.Error(),
					})
				}
				n := atomic.AddInt64(&processed, 1)
				if n%50 == 0 {
					jobLog.Info("hash generation progress", logger.Data{
						"processed":  n,
						"total":      total,
						"library_id": libraryID,
					})
				}
			}
		}()
	}

	for _, id := range fileIDs {
		select {
		case <-ctx.Done():
			close(workCh)
			wg.Wait()
			return ctx.Err()
		case workCh <- workItem{fileID: id}:
		}
	}
	close(workCh)
	wg.Wait()

	jobLog.Info("hash generation complete", logger.Data{
		"processed":  atomic.LoadInt64(&processed),
		"total":      total,
		"library_id": libraryID,
	})
	return nil
}

// hashOneFile retrieves a single file, computes its sha256, and persists the
// result. Per-file errors (missing on disk, permission denied, read error) are
// logged as warnings and the function returns nil so the job continues.
func (w *Worker) hashOneFile(ctx context.Context, fileID int, jobLog *joblogs.JobLogger) error {
	file, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		jobLog.Warn("could not retrieve file record", logger.Data{
			"file_id": fileID,
			"error":   err.Error(),
		})
		return nil
	}

	// Stat before reading to surface missing/permission errors with better context.
	if _, err := os.Stat(file.Filepath); err != nil {
		if os.IsNotExist(err) {
			jobLog.Warn("file not found on disk, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
			})
		} else if os.IsPermission(err) {
			jobLog.Warn("permission denied reading file, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
			})
		} else {
			jobLog.Warn("could not stat file, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
				"error":    err.Error(),
			})
		}
		return nil
	}

	hash, err := fingerprint.ComputeSHA256(file.Filepath)
	if err != nil {
		jobLog.Warn("could not compute sha256, skipping", logger.Data{
			"file_id":  fileID,
			"filepath": file.Filepath,
			"error":    err.Error(),
		})
		return nil
	}

	if err := w.fingerprintService.Insert(ctx, fileID, models.FingerprintAlgorithmSHA256, hash); err != nil {
		jobLog.Warn("could not insert fingerprint, skipping", logger.Data{
			"file_id":  fileID,
			"filepath": file.Filepath,
			"error":    err.Error(),
		})
		return nil
	}

	return nil
}

// EnsureHashGenerationJob checks whether a pending or in-progress hash
// generation job already exists for the given library and creates one only if
// none is found. Safe to call from the scan job at the end of each scan.
func EnsureHashGenerationJob(ctx context.Context, jobService *jobs.Service, libraryID int) error {
	active, err := jobService.HasActiveJob(ctx, models.JobTypeHashGeneration, &libraryID)
	if err != nil {
		return errors.Wrap(err, "check for active hash generation job")
	}
	if active {
		return nil
	}

	newJob := &models.Job{
		Type:      models.JobTypeHashGeneration,
		Status:    models.JobStatusPending,
		LibraryID: &libraryID,
		DataParsed: &models.JobHashGenerationData{
			LibraryID: libraryID,
		},
	}
	if err := jobService.CreateJob(ctx, newJob); err != nil {
		return errors.Wrap(err, "create hash generation job")
	}

	return nil
}
