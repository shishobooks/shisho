package worker

import (
	"sync"
	"time"

	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/models"
)

// scanProgressInterval is the least time between two progress events for one
// library. The frontend refetches the gallery on them, so a scan importing
// hundreds of files a second must not publish once per file.
const scanProgressInterval = time.Second

// scanProgressReporter counts a scan job's progress through one library and
// publishes it as scan.progress events, at most once per scanProgressInterval
// and once more, marked done, when the library is finished.
type scanProgressReporter struct {
	broker *events.Broker
	now    func() time.Time

	mu            sync.Mutex
	progress      models.JobScanProgress
	lastPublished time.Time
}

// newScanProgressReporter returns a reporter for one library's pass of a scan
// job. A nil broker makes every method a no-op beyond counting.
func newScanProgressReporter(broker *events.Broker, jobID, libraryID int, scanForNewMetadata bool) *scanProgressReporter {
	return &scanProgressReporter{
		broker: broker,
		now:    time.Now,
		progress: models.JobScanProgress{
			JobID:              jobID,
			LibraryID:          libraryID,
			ScanForNewMetadata: scanForNewMetadata,
		},
	}
}

// fileFound counts a file the scan will process.
func (r *scanProgressReporter) fileFound() {
	r.update(func(p *models.JobScanProgress) { p.FilesFound++ })
}

// setFilesFound replaces the found count, for files added after the walk
// (input converter output).
func (r *scanProgressReporter) setFilesFound(n int) {
	r.update(func(p *models.JobScanProgress) { p.FilesFound = n })
}

// fileScanned counts a processed file, and the Book it created if any.
func (r *scanProgressReporter) fileScanned(bookCreated bool) {
	r.update(func(p *models.JobScanProgress) {
		p.FilesScanned++
		if bookCreated {
			p.BooksImported++
		}
	})
}

// flush publishes the current counts now, whatever the interval. The scan
// calls it once the last file is processed, since organizing and orphan
// cleanup can take a while before finish.
func (r *scanProgressReporter) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishLocked()
}

// finish publishes the final counts, marked done, whatever the interval.
func (r *scanProgressReporter) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress.Done = true
	r.publishLocked()
}

func (r *scanProgressReporter) update(apply func(p *models.JobScanProgress)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	apply(&r.progress)
	if r.now().Sub(r.lastPublished) >= scanProgressInterval {
		r.publishLocked()
	}
}

func (r *scanProgressReporter) publishLocked() {
	r.lastPublished = r.now()
	if r.broker == nil {
		return
	}
	r.broker.Publish(events.NewScanProgressEvent(r.progress))
}
