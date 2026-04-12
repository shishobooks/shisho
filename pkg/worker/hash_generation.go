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
//
// The job loops until ListFilesMissingAlgorithm returns an empty list, so
// files added to the library while the job is running (either by a monitor
// batch in parallel, or because a spurious rescan invalidated an existing
// fingerprint) get picked up in a subsequent pass instead of being orphaned
// by job dedup in EnsureHashGenerationJob.
func (w *Worker) ProcessHashGenerationJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	data, ok := job.DataParsed.(*models.JobHashGenerationData)
	if !ok || data == nil {
		return errors.New("invalid or missing job data for hash generation job")
	}

	libraryID := data.LibraryID
	jobLog.Info("processing hash generation job", logger.Data{"library_id": libraryID})

	// maxPasses bounds the outer loop so a persistent failure (e.g. every
	// remaining file is unreadable) cannot spin forever. Loop exit rule:
	// stop when a pass makes zero progress AND the next ListFilesMissingAlgorithm
	// returns only IDs we've already attempted. This lets files added by a
	// concurrent monitor batch (during a no-progress pass whose own files
	// were all unreadable) get picked up instead of being orphaned.
	const maxPasses = 10
	totalHashed := int64(0)
	totalAttempted := 0
	attempted := make(map[int]struct{})

	for pass := 0; pass < maxPasses; pass++ {
		fileIDs, err := w.fingerprintService.ListFilesMissingAlgorithm(ctx, libraryID, models.FingerprintAlgorithmSHA256)
		if err != nil {
			return errors.Wrap(err, "list files missing sha256")
		}
		if len(fileIDs) == 0 {
			break
		}

		// Filter out IDs we've already attempted this job so a failing file
		// can't force us into the retry loop forever. If every pending file
		// has already been attempted, we're done.
		pending := make([]int, 0, len(fileIDs))
		for _, id := range fileIDs {
			if _, seen := attempted[id]; seen {
				continue
			}
			pending = append(pending, id)
		}
		if len(pending) == 0 {
			break
		}
		for _, id := range pending {
			attempted[id] = struct{}{}
		}

		batch := len(pending)
		totalAttempted += batch
		jobLog.Info("files needing sha256 fingerprint", logger.Data{
			"count":      batch,
			"library_id": libraryID,
			"pass":       pass,
		})

		// Worker pool size: at least 4 goroutines, at most NumCPU.
		numWorkers := runtime.NumCPU()
		if numWorkers < 4 {
			numWorkers = 4
		}

		type workItem struct {
			fileID int
		}

		workCh := make(chan workItem, numWorkers)
		var passHashed int64

		var wg sync.WaitGroup
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range workCh {
					inserted, err := w.hashOneFile(ctx, item.fileID, jobLog)
					if err != nil {
						jobLog.Warn("hash generation failed for file", logger.Data{
							"file_id": item.fileID,
							"error":   err.Error(),
						})
					}
					if inserted {
						atomic.AddInt64(&passHashed, 1)
					}
				}
			}()
		}

		for _, id := range pending {
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

		totalHashed += atomic.LoadInt64(&passHashed)
	}

	jobLog.Info("hash generation complete", logger.Data{
		"hashed":     totalHashed,
		"attempted":  totalAttempted,
		"library_id": libraryID,
	})
	return nil
}

// hashOneFile retrieves a single file, computes its sha256, and persists the
// result. Per-file errors (missing on disk, permission denied, read error) are
// logged as warnings and the function returns (false, nil) so the job
// continues. Returns (true, nil) only when a fingerprint was successfully
// written to the DB — the caller uses this to detect no-progress passes.
//
//nolint:unparam // error return reserved for future hard-failure propagation
func (w *Worker) hashOneFile(ctx context.Context, fileID int, jobLog *joblogs.JobLogger) (bool, error) {
	file, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		jobLog.Warn("could not retrieve file record", logger.Data{
			"file_id": fileID,
			"error":   err.Error(),
		})
		return false, nil
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
		return false, nil
	}

	hash, err := fingerprint.ComputeSHA256(file.Filepath)
	if err != nil {
		jobLog.Warn("could not compute sha256, skipping", logger.Data{
			"file_id":  fileID,
			"filepath": file.Filepath,
			"error":    err.Error(),
		})
		return false, nil
	}

	if err := w.fingerprintService.Insert(ctx, fileID, models.FingerprintAlgorithmSHA256, hash); err != nil {
		jobLog.Warn("could not insert fingerprint, skipping", logger.Data{
			"file_id":  fileID,
			"filepath": file.Filepath,
			"error":    err.Error(),
		})
		return false, nil
	}

	return true, nil
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
