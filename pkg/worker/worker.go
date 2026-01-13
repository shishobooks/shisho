package worker

import (
	"context"
	"math/rand"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/tags"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
)

var (
	processID         = randStringBytes(8)
	errJobPanicked    = errors.New("job panicked")
	errUnknownJobType = errors.New("unknown job type")
)

type Worker struct {
	config *config.Config
	log    logger.Logger

	processFuncs map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error

	bookService      *books.Service
	genreService     *genres.Service
	imprintService   *imprints.Service
	jobService       *jobs.Service
	jobLogService    *joblogs.Service
	libraryService   *libraries.Service
	personService    *people.Service
	publisherService *publishers.Service
	searchService    *search.Service
	seriesService    *series.Service
	tagService       *tags.Service

	queue          chan *models.Job
	shutdown       chan struct{}
	doneFetching   chan struct{}
	doneProcessing chan struct{}
	doneScheduling chan struct{}
	doneCleanup    chan struct{}
}

func New(cfg *config.Config, db *bun.DB) *Worker {
	bookService := books.NewService(db)
	genreService := genres.NewService(db)
	imprintService := imprints.NewService(db)
	jobService := jobs.NewService(db)
	jobLogService := joblogs.NewService(db)
	libraryService := libraries.NewService(db)
	personService := people.NewService(db)
	publisherService := publishers.NewService(db)
	searchService := search.NewService(db)
	seriesService := series.NewService(db)
	tagService := tags.NewService(db)

	w := &Worker{
		config: cfg,
		log:    logger.New(),

		bookService:      bookService,
		genreService:     genreService,
		imprintService:   imprintService,
		jobService:       jobService,
		jobLogService:    jobLogService,
		libraryService:   libraryService,
		personService:    personService,
		publisherService: publisherService,
		searchService:    searchService,
		seriesService:    seriesService,
		tagService:       tagService,

		queue:          make(chan *models.Job, cfg.WorkerProcesses),
		shutdown:       make(chan struct{}),
		doneFetching:   make(chan struct{}),
		doneProcessing: make(chan struct{}, cfg.WorkerProcesses),
		doneScheduling: make(chan struct{}),
		doneCleanup:    make(chan struct{}),
	}

	w.processFuncs = map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error{
		models.JobTypeScan: w.ProcessScanJob,
	}

	return w
}

func (w *Worker) Start() {
	go w.fetchJobs()
	for i := 0; i < w.config.WorkerProcesses; i++ {
		go w.processJobs()
	}
	if w.config.SyncIntervalMinutes > 0 {
		go w.scheduleScanJobs()
	} else {
		// No scheduling needed, mark as done immediately
		go func() {
			w.doneScheduling <- struct{}{}
		}()
	}
	if w.config.JobRetentionDays > 0 {
		go w.cleanupOldJobs()
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
				Statuses:           []string{models.JobStatusPending, models.JobStatusInProgress},
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

			// Create job logger for DB persistence
			jobLog := w.jobLogService.NewJobLogger(ctx, job.ID, log)

			// Update job to be in progress and claimed by this process.
			job.Status = models.JobStatusInProgress
			job.ProcessID = &processID

			err = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
				Columns: []string{"status", "process_id"},
			})
			if err != nil {
				log.Err(err).Error("update job error")
				continue
			}

			// Process with panic recovery
			func() {
				defer func() {
					if r := recover(); r != nil {
						jobLog.Fatal("job panicked", errors.Wrapf(errJobPanicked, "%v", r), logger.Data{"panic": r})
						job.Status = models.JobStatusFailed
						_ = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
							Columns: []string{"status"},
						})
					}
				}()

				// Find and invoke the appropriate process function.
				fn, ok := w.processFuncs[job.Type]
				if !ok {
					jobLog.Error("can't find process function for type", errors.Wrapf(errUnknownJobType, "%s", job.Type), nil)
					job.Status = models.JobStatusFailed
					_ = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
						Columns: []string{"status"},
					})
					return
				}

				err = fn(ctx, job, jobLog)
				if err != nil {
					jobLog.Error("job failed", err, nil)
					job.Status = models.JobStatusFailed
					_ = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
						Columns: []string{"status"},
					})
					return
				}

				// Update job to be completed so that it's not picked up anymore.
				job.Status = models.JobStatusCompleted
				err = w.jobService.UpdateJob(ctx, job, jobs.UpdateJobOptions{
					Columns: []string{"status"},
				})
				if err != nil {
					log.Err(err).Error("update job error")
				}
			}()
		}
	}
}

func (w *Worker) scheduleScanJobs() {
	duration := time.Duration(w.config.SyncIntervalMinutes) * time.Minute
	timer := time.NewTimer(duration)

	for {
		select {
		case <-w.shutdown:
			timer.Stop()
			w.doneScheduling <- struct{}{}
			return
		case <-timer.C:
			ctx := context.Background()
			log := w.log.Root(logger.Data{"scheduler": "scan"})

			// Check if there are any non-deleted libraries
			libs, err := w.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{
				Limit: pointerutil.Int(1),
			})
			if err != nil {
				log.Err(err).Error("failed to list libraries for scheduled scan")
				timer.Reset(duration)
				continue
			}
			if len(libs) == 0 {
				log.Debug("no libraries configured, skipping scheduled scan")
				timer.Reset(duration)
				continue
			}

			// Check if a scan job is already active
			hasActive, err := w.jobService.HasActiveJobByType(ctx, models.JobTypeScan)
			if err != nil {
				log.Err(err).Error("failed to check for active scan job")
				timer.Reset(duration)
				continue
			}
			if hasActive {
				log.Debug("scan job already running or pending, skipping scheduled scan")
				timer.Reset(duration)
				continue
			}

			// Create a new scan job
			scanJob := &models.Job{
				Type:       models.JobTypeScan,
				Status:     models.JobStatusPending,
				DataParsed: &models.JobScanData{},
			}
			if err := w.jobService.CreateJob(ctx, scanJob); err != nil {
				log.Err(err).Error("failed to create scheduled scan job")
				timer.Reset(duration)
				continue
			}

			log.Info("created scheduled scan job")
			timer.Reset(duration)
		}
	}
}

func (w *Worker) cleanupOldJobs() {
	// Run cleanup hourly
	duration := 1 * time.Hour
	timer := time.NewTimer(duration)

	for {
		select {
		case <-w.shutdown:
			timer.Stop()
			w.doneCleanup <- struct{}{}
			return
		case <-timer.C:
			ctx := context.Background()
			log := w.log.Root(logger.Data{"cleanup": "jobs"})

			count, err := w.jobService.CleanupOldJobs(ctx, w.config.JobRetentionDays)
			if err != nil {
				log.Err(err).Error("failed to cleanup old jobs")
			} else if count > 0 {
				log.Info("cleaned up old jobs", logger.Data{"count": count})
			}
			timer.Reset(duration)
		}
	}
}

func (w *Worker) Shutdown() {
	close(w.shutdown)

	<-w.doneFetching
	<-w.doneScheduling
	for i := 0; i < w.config.WorkerProcesses; i++ {
		<-w.doneProcessing
	}
	if w.config.JobRetentionDays > 0 {
		<-w.doneCleanup
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
