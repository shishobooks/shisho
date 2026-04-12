package worker

import (
	"context"
	"os"
	"runtime"
	"sync"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/fingerprint"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
)

// hashGenWorkItem is the per-file unit of work dispatched to the worker pool.
type hashGenWorkItem struct {
	fileID int
}

// hashGenResult is what a worker reports back after attempting one file.
// done=true means the caller should mark the ID as attempted so we don't
// retry it in subsequent passes of the same job; done=false means the
// failure was transient (DB contention, etc.) and retrying next pass may
// succeed.
type hashGenResult struct {
	fileID   int
	inserted bool
	done     bool
}

// ProcessHashGenerationJob computes and stores sha256 fingerprints for all
// files in a library that don't yet have one. Per-file errors are logged and
// skipped so a single unreadable file cannot fail the whole job.
//
// The job loops until ListFilesMissingAlgorithm returns only IDs we've
// already attempted. Files added to the library while the job is running
// (either by a monitor batch in parallel, or because a spurious rescan
// invalidated an existing fingerprint) get picked up in a subsequent pass
// instead of being orphaned by job dedup in EnsureHashGenerationJob.
//
// Error handling is split into "permanent" vs "transient": a file that's
// missing on disk, unreadable, or gone from the DB is marked as attempted
// and never retried within this job run. DB errors on Insert/Retrieve are
// treated as transient — the ID is left unmarked so the next pass tries
// again. maxPasses still bounds total retries.
func (w *Worker) ProcessHashGenerationJob(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error {
	data, ok := job.DataParsed.(*models.JobHashGenerationData)
	if !ok || data == nil {
		return errors.New("invalid or missing job data for hash generation job")
	}

	libraryID := data.LibraryID
	jobLog.Info("processing hash generation job", logger.Data{"library_id": libraryID})

	const maxPasses = 10
	totalHashed := int64(0)
	// totalAttempted counts unique IDs we've run through the worker pool
	// across all passes; transient failures retry on the next pass and do
	// not inflate this count. Invariant: totalHashed <= totalAttempted.
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

		// Filter out IDs that we already marked as permanently attempted —
		// retrying them won't help and would lock the loop.
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

		batch := len(pending)
		totalAttempted += batch
		jobLog.Info("files needing sha256 fingerprint", logger.Data{
			"count":      batch,
			"library_id": libraryID,
			"pass":       pass,
		})

		passHashed, err := w.runHashGenPass(ctx, pending, attempted, jobLog)
		if err != nil {
			return err
		}
		totalHashed += passHashed
	}

	jobLog.Info("hash generation complete", logger.Data{
		"hashed":     totalHashed,
		"attempted":  totalAttempted,
		"library_id": libraryID,
	})
	return nil
}

// runHashGenPass dispatches the pending file IDs to a worker pool and
// collects the results. IDs whose results are marked done=true are added to
// the attempted set; transient failures are left unmarked so they can be
// retried in the next pass. Returns the number of fingerprints successfully
// inserted during this pass.
func (w *Worker) runHashGenPass(
	ctx context.Context,
	pending []int,
	attempted map[int]struct{},
	jobLog *joblogs.JobLogger,
) (int64, error) {
	numWorkers := hashGenWorkerCount()
	workCh := make(chan hashGenWorkItem, numWorkers)
	resultCh := make(chan hashGenResult, len(pending))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range workCh {
				inserted, done, err := w.hashOneFile(ctx, item.fileID, jobLog)
				if err != nil {
					jobLog.Warn("hash generation failed for file", logger.Data{
						"file_id":   item.fileID,
						"permanent": done,
						"error":     err.Error(),
					})
				}
				resultCh <- hashGenResult{
					fileID:   item.fileID,
					inserted: inserted,
					done:     done,
				}
			}
		}()
	}

	// Dispatch work, honoring cancellation.
	dispatchErr := func() error {
		defer close(workCh)
		for _, id := range pending {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case workCh <- hashGenWorkItem{fileID: id}:
			}
		}
		return nil
	}()
	wg.Wait()
	close(resultCh)

	var passHashed int64
	for r := range resultCh {
		if r.done {
			attempted[r.fileID] = struct{}{}
		}
		if r.inserted {
			passHashed++
		}
	}

	if dispatchErr != nil {
		return passHashed, dispatchErr
	}
	return passHashed, nil
}

// hashGenWorkerCount returns the worker pool size for hash generation
// (max(NumCPU, 4)). Mirrors the sizing used by the scan job.
func hashGenWorkerCount() int {
	return max(runtime.NumCPU(), 4)
}

// hashOneFile retrieves a single file, computes its sha256, and persists the
// result. It returns:
//   - inserted: true if a new fingerprint row was written to the DB
//   - done: true if the caller should NOT retry this file within the job
//     (either it succeeded or it hit a permanent failure)
//   - err: a descriptive error for logging; may be non-nil even when done=true
//     (e.g. "file not found on disk" is logged but considered final)
//
// Permanent failures (done=true, no retry): file row gone, file missing on
// disk, permission denied, read error, local stat error.
//
// Transient failures (done=false, retry next pass): DB errors when retrieving
// the file row or inserting the fingerprint. These often recover on a
// retry within the same job run.
func (w *Worker) hashOneFile(ctx context.Context, fileID int, jobLog *joblogs.JobLogger) (inserted, done bool, err error) {
	file, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		// File row missing is permanent; other DB errors may be transient.
		if errors.Is(err, errcodes.NotFound("File")) {
			jobLog.Warn("file row not in DB, skipping", logger.Data{
				"file_id": fileID,
				"error":   err.Error(),
			})
			return false, true, nil
		}
		return false, false, errors.Wrap(err, "retrieve file record")
	}

	// Stat before reading to surface missing/permission errors with better context.
	if _, statErr := os.Stat(file.Filepath); statErr != nil {
		if os.IsNotExist(statErr) {
			jobLog.Warn("file not found on disk, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
			})
		} else if os.IsPermission(statErr) {
			jobLog.Warn("permission denied reading file, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
			})
		} else {
			jobLog.Warn("could not stat file, skipping", logger.Data{
				"file_id":  fileID,
				"filepath": file.Filepath,
				"error":    statErr.Error(),
			})
		}
		return false, true, nil
	}

	hash, err := fingerprint.ComputeSHA256(file.Filepath)
	if err != nil {
		// Read error (permission, disk I/O, truncation) — unlikely to
		// improve on retry. Mark as permanent.
		jobLog.Warn("could not compute sha256, skipping", logger.Data{
			"file_id":  fileID,
			"filepath": file.Filepath,
			"error":    err.Error(),
		})
		return false, true, nil
	}

	if err := w.fingerprintService.Insert(ctx, fileID, models.FingerprintAlgorithmSHA256, hash); err != nil {
		// DB insert may hit contention or a transient sqlite busy. Leave
		// it unmarked so the next pass retries.
		return false, false, errors.Wrap(err, "insert fingerprint")
	}

	return true, true, nil
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
