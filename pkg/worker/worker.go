package worker

import (
	"context"
	"math/rand"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/chapters"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/plugins"
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
	db     *bun.DB

	processFuncs map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error

	bookService        *books.Service
	chapterService     *chapters.Service
	genreService       *genres.Service
	imprintService     *imprints.Service
	jobService         *jobs.Service
	jobLogService      *joblogs.Service
	libraryService     *libraries.Service
	personService      *people.Service
	publisherService   *publishers.Service
	searchService      *search.Service
	seriesService      *series.Service
	tagService         *tags.Service
	fingerprintService *fingerprints.Service

	appSettingsService *appsettings.Service

	pluginService *plugins.Service
	pluginManager *plugins.Manager

	broker        *events.Broker
	downloadCache *downloadcache.Cache

	monitor *Monitor

	queue           chan *models.Job
	shutdown        chan struct{}
	doneFetching    chan struct{}
	doneProcessing  chan struct{}
	doneScheduling  chan struct{}
	doneCleanup     chan struct{}
	doneUpdateCheck chan struct{}

	// ctx is the worker-wide context cancelled by Shutdown. It is the parent
	// for every job handler's context (hash generation, scans, bulk downloads)
	// AND for the DB calls inside fetchJobs/scheduleScanJobs/cleanupOldJobs/
	// checkPluginUpdates. Without this, a hash-gen job iterating a large
	// library would keep processJobs busy past air's 1s kill_delay (and the
	// next `mise start` reload would race the outgoing process for port
	// 3689), and a slow scheduler DB query would block the scheduler goroutine
	// from observing shutdown until the query returned.
	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg *config.Config, db *bun.DB, pm *plugins.Manager, broker *events.Broker, dlCache *downloadcache.Cache) *Worker {
	appSettingsService := appsettings.NewService(db)
	bookService := books.NewService(db).WithAppSettings(appSettingsService)
	chapterService := chapters.NewService(db)
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
	pluginService := plugins.NewService(db)
	fingerprintService := fingerprints.NewService(db)

	ctx, cancel := context.WithCancel(context.Background())

	w := &Worker{
		config: cfg,
		log:    logger.New(),
		db:     db,

		ctx:    ctx,
		cancel: cancel,

		appSettingsService: appSettingsService,
		bookService:        bookService,
		chapterService:     chapterService,
		genreService:       genreService,
		imprintService:     imprintService,
		jobService:         jobService,
		jobLogService:      jobLogService,
		libraryService:     libraryService,
		personService:      personService,
		publisherService:   publisherService,
		searchService:      searchService,
		seriesService:      seriesService,
		tagService:         tagService,
		fingerprintService: fingerprintService,
		pluginService:      pluginService,
		pluginManager:      pm,
		broker:             broker,
		downloadCache:      dlCache,

		queue:           make(chan *models.Job, cfg.WorkerProcesses),
		shutdown:        make(chan struct{}),
		doneFetching:    make(chan struct{}),
		doneProcessing:  make(chan struct{}, cfg.WorkerProcesses),
		doneScheduling:  make(chan struct{}),
		doneCleanup:     make(chan struct{}),
		doneUpdateCheck: make(chan struct{}),
	}

	w.processFuncs = map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error{
		models.JobTypeScan:            w.ProcessScanJob,
		models.JobTypeBulkDownload:    w.ProcessBulkDownloadJob,
		models.JobTypeHashGeneration:  w.ProcessHashGenerationJob,
		models.JobTypeRecomputeReview: w.ProcessRecomputeReviewJob,
	}

	if dlCache != nil {
		dlCache.ShouldSkipCleanup = func() bool {
			// Use w.ctx so the query bails out during shutdown instead of
			// holding up the cleanup goroutine on a DB that may be closing.
			hasActive, err := w.jobService.HasActiveJob(w.ctx, models.JobTypeBulkDownload, nil)
			if err != nil {
				// During shutdown the query returns context.Canceled — we
				// can't confirm whether a bulk download is active, so err on
				// the side of skipping cleanup rather than racing against
				// Shutdown and potentially deleting an in-use cache file.
				// Any other error (real DB failure) falls through to false
				// so cleanup can still make progress under normal operation.
				return errors.Is(err, context.Canceled)
			}
			return hasActive
		}
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
	go w.checkPluginUpdates()
	if w.config.LibraryMonitorEnabled {
		w.monitor = newMonitor(w)
		w.monitor.start()
	} else {
		w.log.Info("library monitor disabled")
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
			j, err := w.jobService.ListJobs(w.ctx, jobs.ListJobsOptions{
				Limit:              pointerutil.Int(1),
				Statuses:           []string{models.JobStatusPending, models.JobStatusInProgress},
				ProcessIDToExclude: &processID,
			})
			if err != nil {
				// Silently drop ctx.Canceled errors during shutdown; the
				// enclosing select will observe w.shutdown on the next loop.
				if !errors.Is(err, context.Canceled) {
					w.log.Err(err).Error("list jobs error")
				}
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
			// Derive from w.ctx (not context.Background) so Shutdown can
			// cancel in-flight jobs. Hash generation, scan, and bulk download
			// all check ctx.Done() at loop boundaries and return ctx.Err()
			// when cancelled.
			ctx := log.WithContext(w.ctx)

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
			w.publishJobEvent("job.status_changed", job)

			// Process with panic recovery
			func() {
				defer func() {
					if r := recover(); r != nil {
						jobLog.Fatal("job panicked", errors.Wrapf(errJobPanicked, "%v", r), logger.Data{"panic": r})
						job.Status = models.JobStatusFailed
						w.persistJobStatus(job, log)
					}
				}()

				// Find and invoke the appropriate process function.
				fn, ok := w.processFuncs[job.Type]
				if !ok {
					jobLog.Error("can't find process function for type", errors.Wrapf(errUnknownJobType, "%s", job.Type), nil)
					job.Status = models.JobStatusFailed
					w.persistJobStatus(job, log)
					return
				}

				err = fn(ctx, job, jobLog)
				if err != nil {
					// A context.Canceled from shutdown isn't a real failure —
					// suppress the ERROR log (and its doomed DB persist, since
					// the job ctx is already cancelled) to match how the
					// schedulers above handle the same case. We still fall
					// through to mark the row failed.
					//
					// Cancelled-at-shutdown jobs intentionally stay at
					// failed rather than being reset to pending:
					//   - scan is re-queued by scheduleScanJobs on the next
					//     SyncIntervalMinutes tick
					//   - hash_generation is re-queued by the tail-hook at
					//     the end of every scan
					//   - bulk_download is NOT re-queued automatically; a
					//     partial zip is cheaper to abandon than to re-render
					//     unprompted on restart, so the user re-initiates it
					if !errors.Is(err, context.Canceled) {
						jobLog.Error("job failed", err, nil)
					}
					job.Status = models.JobStatusFailed
					w.persistJobStatus(job, log)
					return
				}

				// Update job to be completed so that it's not picked up anymore.
				job.Status = models.JobStatusCompleted
				w.persistJobStatus(job, log)
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
			log := w.log.Root(logger.Data{"scheduler": "scan"})

			// Check if there are any non-deleted libraries
			libs, err := w.libraryService.ListLibraries(w.ctx, libraries.ListLibrariesOptions{
				Limit: pointerutil.Int(1),
			})
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Err(err).Error("failed to list libraries for scheduled scan")
				}
				timer.Reset(duration)
				continue
			}
			if len(libs) == 0 {
				log.Debug("no libraries configured, skipping scheduled scan")
				timer.Reset(duration)
				continue
			}

			// Check if a scan job is already active
			hasActive, err := w.jobService.HasActiveJob(w.ctx, models.JobTypeScan, nil)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Err(err).Error("failed to check for active scan job")
				}
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
			if err := w.jobService.CreateJob(w.ctx, scanJob); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Err(err).Error("failed to create scheduled scan job")
				}
				timer.Reset(duration)
				continue
			}
			w.publishJobEvent("job.created", scanJob)

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
			log := w.log.Root(logger.Data{"cleanup": "jobs"})

			count, err := w.jobService.CleanupOldJobs(w.ctx, w.config.JobRetentionDays)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Err(err).Error("failed to cleanup old jobs")
				}
			} else if count > 0 {
				log.Info("cleaned up old jobs", logger.Data{"count": count})
			}
			timer.Reset(duration)
		}
	}
}

func (w *Worker) checkPluginUpdates() {
	if w.pluginManager == nil {
		w.doneUpdateCheck <- struct{}{}
		return
	}

	// Fire immediately on startup so a server restart refreshes stale
	// update_available_version values, then run every 24 hours.
	duration := 24 * time.Hour
	timer := time.NewTimer(0)

	for {
		select {
		case <-w.shutdown:
			timer.Stop()
			w.doneUpdateCheck <- struct{}{}
			return
		case <-timer.C:
			log := w.log.Root(logger.Data{"scheduler": "plugin-update-check"})

			if err := w.pluginManager.CheckForUpdates(w.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Err(err).Error("failed to check for plugin updates")
				}
			} else {
				log.Debug("completed plugin update check")
			}
			timer.Reset(duration)
		}
	}
}

// cleanupOrphanedEntities removes series, people, genres, and tags
// that are no longer referenced by any books.
func (w *Worker) cleanupOrphanedEntities(ctx context.Context, log logger.Logger) {
	if deletedIDs, err := w.seriesService.CleanupOrphanedSeries(ctx); err != nil {
		log.Err(err).Warn("failed to cleanup orphaned series")
	} else if len(deletedIDs) > 0 {
		for _, id := range deletedIDs {
			if err := w.searchService.DeleteFromSeriesIndex(ctx, id); err != nil {
				log.Err(err).Warn("failed to remove orphaned series from search index", logger.Data{"series_id": id})
			}
		}
		log.Info("cleaned up orphaned series", logger.Data{"count": len(deletedIDs)})
	}

	if n, err := w.personService.CleanupOrphanedPeople(ctx); err != nil {
		log.Err(err).Warn("failed to cleanup orphaned people")
	} else if n > 0 {
		log.Info("cleaned up orphaned people", logger.Data{"count": n})
	}

	if deletedIDs, err := w.genreService.CleanupOrphanedGenres(ctx); err != nil {
		log.Err(err).Warn("failed to cleanup orphaned genres")
	} else if len(deletedIDs) > 0 {
		for _, id := range deletedIDs {
			if err := w.searchService.DeleteFromGenreIndex(ctx, id); err != nil {
				log.Err(err).Warn("failed to remove orphaned genre from search index", logger.Data{"genre_id": id})
			}
		}
		log.Info("cleaned up orphaned genres", logger.Data{"count": len(deletedIDs)})
	}

	if deletedIDs, err := w.tagService.CleanupOrphanedTags(ctx); err != nil {
		log.Err(err).Warn("failed to cleanup orphaned tags")
	} else if len(deletedIDs) > 0 {
		for _, id := range deletedIDs {
			if err := w.searchService.DeleteFromTagIndex(ctx, id); err != nil {
				log.Err(err).Warn("failed to remove orphaned tag from search index", logger.Data{"tag_id": id})
			}
		}
		log.Info("cleaned up orphaned tags", logger.Data{"count": len(deletedIDs)})
	}
}

// RefreshMonitorWatches signals the filesystem monitor to reload library paths.
// Safe to call even if the monitor is disabled (no-op).
func (w *Worker) RefreshMonitorWatches() {
	if w.monitor != nil {
		w.monitor.RefreshWatches()
	}
}

func (w *Worker) Shutdown() {
	if w.monitor != nil {
		w.monitor.stop()
	}

	// Cancel w.ctx BEFORE closing w.shutdown. This unblocks two things at
	// once: in-flight job handlers currently running inside fn(ctx, ...),
	// and DB queries in any of the scheduler goroutines that happened to
	// be mid-call when shutdown arrived. Without this, processJobs would
	// wait for the current job to finish naturally, and the schedulers
	// wouldn't observe w.shutdown until their driver-level timeouts fired.
	if w.cancel != nil {
		w.cancel()
	}

	close(w.shutdown)

	<-w.doneFetching
	<-w.doneScheduling
	for i := 0; i < w.config.WorkerProcesses; i++ {
		<-w.doneProcessing
	}
	if w.config.JobRetentionDays > 0 {
		<-w.doneCleanup
	}
	<-w.doneUpdateCheck
}

func (w *Worker) publishJobEvent(eventType string, job *models.Job) {
	if w.broker == nil {
		return
	}
	w.broker.Publish(events.NewJobEvent(eventType, job.ID, job.Status, job.Type, job.LibraryID))
}

// jobStatusWriteTimeout bounds the final status-update DB call when a job
// finishes. It's short because the update is a single-row write — if it's
// slower than this, something is wrong with the DB and blocking Shutdown on
// it would only compound the problem.
const jobStatusWriteTimeout = time.Second

// persistJobStatus writes the job's current status (completed/failed) to the
// DB and broadcasts the status_changed event, using a background-derived
// context so it can still persist during shutdown. The in-progress job ctx
// is cancelled the moment Shutdown starts, which would otherwise make this
// final write fail silently and leave the row stuck at in_progress. The
// 1s deadline keeps us from stalling Shutdown if the DB itself hangs.
func (w *Worker) persistJobStatus(job *models.Job, log logger.Logger) {
	statusCtx, cancel := context.WithTimeout(context.Background(), jobStatusWriteTimeout)
	defer cancel()
	if err := w.jobService.UpdateJob(statusCtx, job, jobs.UpdateJobOptions{
		Columns: []string{"status"},
	}); err != nil {
		log.Err(err).Error("job status update error")
		return
	}
	w.publishJobEvent("job.status_changed", job)
}

const letterBytes = "abcdef0123456789"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
