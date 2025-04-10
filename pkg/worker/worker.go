package worker

import (
	"context"
	"math/rand"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"

	"github.com/google/uuid"
	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
)

var processID = randStringBytes(8)

type Worker struct {
	config *config.Config
	log    logger.Logger

	processFuncs map[string]func(ctx context.Context, job *jobs.Job) error

	bookService    *books.Service
	jobService     *jobs.Service
	libraryService *libraries.Service

	queue          chan *jobs.Job
	shutdown       chan struct{}
	doneFetching   chan struct{}
	doneProcessing chan struct{}
}

func New(cfg *config.Config, db *bun.DB) *Worker {
	bookService := books.NewService(db)
	jobService := jobs.NewService(db)
	libraryService := libraries.NewService(db)

	w := &Worker{
		config: cfg,
		log:    logger.New(),

		bookService:    bookService,
		jobService:     jobService,
		libraryService: libraryService,

		queue:          make(chan *jobs.Job, cfg.WorkerProcesses),
		shutdown:       make(chan struct{}),
		doneFetching:   make(chan struct{}),
		doneProcessing: make(chan struct{}, cfg.WorkerProcesses),
	}

	w.processFuncs = map[string]func(ctx context.Context, job *jobs.Job) error{
		jobs.JobTypeScan: w.ProcessScanJob,
	}

	return w
}

func (w *Worker) Start() {
	go w.fetchJobs()
	for i := 0; i < w.config.WorkerProcesses; i++ {
		go w.processJobs()
	}
}

func (w *Worker) fetchJobs() {
	duration := 5 * time.Second
	timer := time.NewTimer(duration)

	for {
		select {
		case <-w.shutdown:
			// We're shutting down, so stop adding more jobs to the queue.
			w.doneFetching <- struct{}{}
			return
		case <-timer.C:
			j, err := w.jobService.ListJobs(context.Background(), jobs.ListJobsOptions{
				Limit:              pointerutil.Int(1),
				Statuses:           []string{jobs.JobStatusPending, jobs.JobStatusInProgress},
				ProcessIDToExclude: &processID,
			})
			if err != nil {
				w.log.Err(err).Error("list jobs error")
				timer.Reset(duration)
				continue
			}
			for _, job := range j {
				w.queue <- job
			}
			timer.Reset(duration)
		}
	}
}

func (w *Worker) processJobs() {
	for {
		select {
		case <-w.shutdown:
			w.doneProcessing <- struct{}{}
			return
		case job := <-w.queue:
			// Prep the context to be passed down to the process function.
			id, err := uuid.NewRandom()
			if err != nil {
				w.log.Err(err).Error("new uuid error")
				continue
			}
			log := w.log.ID(id.String()).Root(logger.Data{"job_id": job.ID, "type": job.Type, "process_id": processID})
			ctx := log.WithContext(context.Background())

			// Update job to be in progress and claimed by this process.
			job.Status = jobs.JobStatusInProgress
			job.ProcessID = &processID

			err = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
				Columns: []string{"status", "process_id"},
			})
			if err != nil {
				log.Err(err).Error("update job error")
				continue
			}

			// Find and invoke the appropriate process function.
			fn, ok := w.processFuncs[job.Type]
			if !ok {
				log.Err(err).Error("can't find process function for type")
				continue
			}
			err = fn(ctx, job)
			if err != nil {
				log.Err(err).Error("process error")
				continue
			}

			// Update job to be completed so that it's not picked up anymore.
			job.Status = jobs.JobStatusCompleted

			err = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
				Columns: []string{"status"},
			})
			if err != nil {
				log.Err(err).Error("update job error")
				continue
			}
		}
	}
}

func (w *Worker) Shutdown() {
	close(w.shutdown)

	<-w.doneFetching
	for i := 0; i < w.config.WorkerProcesses; i++ {
		<-w.doneProcessing
	}
}

const letterBytes = "abcdef0123456789"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
