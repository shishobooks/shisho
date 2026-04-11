package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
)

// pendingEvent represents a filesystem event accumulated during the debounce window.
type pendingEvent struct {
	Op        fsnotify.Op
	LibraryID int
	// IsDirectory marks Remove/Rename events whose path is not a scannable file —
	// typically a book folder that was removed or renamed. processPendingEvents
	// dispatches these to processDirectoryEvent, which cascades cleanup to every
	// DB file whose filepath sits under the directory.
	IsDirectory bool
}

// Monitor watches library paths for filesystem changes and triggers targeted rescans
// using a debounce pattern inspired by Jellyfin's FileRefresher.
type Monitor struct {
	worker *Worker
	log    logger.Logger
	delay  time.Duration

	// Debounce state
	mu      sync.Mutex
	timer   *time.Timer
	pending map[string]pendingEvent

	// Prevents concurrent processPendingEvents invocations. time.AfterFunc runs
	// in its own goroutine, so a second batch could fire while the first is still
	// scanning files. TryLock lets the second invocation bail out harmlessly.
	processing sync.Mutex

	// Self-write ignore list: paths written by the scanner that should not trigger rescans.
	ignoreMu sync.RWMutex
	ignored  map[string]time.Time

	// Library path → library ID mapping for resolving which library a file belongs to.
	// Only accessed from the run() goroutine (setupWatches, handleEvent, findLibraryID,
	// enqueueExistingFiles). Must NOT be accessed from processPendingEvents or
	// processEvent, which run in time.AfterFunc goroutines.
	pathToLibrary map[string]int

	shutdown chan struct{}
	done     chan struct{}
	refresh  chan struct{} // signals run() to reload library watches
}

// minMonitorDelaySeconds is the minimum allowed debounce delay.
// Values below this are clamped to prevent instant event firing.
const minMonitorDelaySeconds = 5

// newMonitor creates a new filesystem monitor for the given worker.
func newMonitor(w *Worker) *Monitor {
	delaySec := w.config.LibraryMonitorDelaySeconds
	if delaySec < minMonitorDelaySeconds {
		w.log.Warn("library_monitor_delay_seconds too low, clamping to minimum", logger.Data{
			"configured": w.config.LibraryMonitorDelaySeconds,
			"minimum":    minMonitorDelaySeconds,
		})
		delaySec = minMonitorDelaySeconds
	}
	return &Monitor{
		worker:        w,
		log:           w.log.Root(logger.Data{"component": "monitor"}),
		delay:         time.Duration(delaySec) * time.Second,
		pending:       make(map[string]pendingEvent),
		ignored:       make(map[string]time.Time),
		pathToLibrary: make(map[string]int),
		shutdown:      make(chan struct{}),
		done:          make(chan struct{}),
		refresh:       make(chan struct{}, 1),
	}
}

// start begins watching library paths for filesystem changes.
func (m *Monitor) start() {
	go m.run()
}

// stop signals the monitor to shut down and waits for it to finish.
func (m *Monitor) stop() {
	close(m.shutdown)
	<-m.done
}

// RefreshWatches signals the monitor to reload library paths and update watches.
// Call this after libraries are created, updated, or deleted.
func (m *Monitor) RefreshWatches() {
	select {
	case m.refresh <- struct{}{}:
	default:
		// Refresh already pending, skip.
	}
}

// IgnorePath temporarily suppresses filesystem events for the given path.
// Used by the scanner to prevent its own writes (covers, sidecars, file moves)
// from triggering redundant rescans. The path is ignored for 2× the debounce delay.
func (m *Monitor) IgnorePath(path string) {
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	m.ignored[path] = time.Now().Add(m.delay * 2)
}

func (m *Monitor) isIgnored(path string) bool {
	m.ignoreMu.RLock()
	defer m.ignoreMu.RUnlock()
	expiry, ok := m.ignored[path]
	if !ok {
		return false
	}
	return time.Now().Before(expiry)
}

func (m *Monitor) cleanupIgnored() {
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	now := time.Now()
	for path, expiry := range m.ignored {
		if now.After(expiry) {
			delete(m.ignored, path)
		}
	}
}

func (m *Monitor) run() {
	defer close(m.done)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		m.log.Err(err).Error("failed to create filesystem watcher")
		return
	}
	defer watcher.Close()

	watchCount, err := m.setupWatches(watcher)
	if err != nil {
		m.log.Err(err).Error("failed to set up filesystem watches")
		return
	}
	m.log.Info("library monitor started", logger.Data{
		"directories_watched": watchCount,
		"delay_seconds":       int(m.delay.Seconds()),
	})

	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-m.shutdown:
			m.mu.Lock()
			if m.timer != nil {
				m.timer.Stop()
			}
			m.mu.Unlock()
			return

		case <-m.refresh:
			n, refreshErr := m.setupWatches(watcher)
			if refreshErr != nil {
				m.log.Err(refreshErr).Warn("failed to refresh filesystem watches")
			} else {
				m.log.Info("library watches refreshed", logger.Data{"directories_watched": n})
			}

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			m.handleEvent(watcher, event)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			m.log.Err(err).Warn("filesystem watcher error")

		case <-cleanupTicker.C:
			m.cleanupIgnored()
		}
	}
}

// setupWatches loads all library paths from the database and adds recursive watches.
// Returns the total number of directories being watched.
func (m *Monitor) setupWatches(watcher *fsnotify.Watcher) (int, error) {
	ctx := context.Background()
	libs, err := m.worker.libraryService.ListLibraries(ctx, libraries.ListLibrariesOptions{})
	if err != nil {
		return 0, err
	}

	// Clear stale mappings so paths from removed/deleted libraries
	// are ignored by findLibraryID even if OS-level watches linger.
	m.pathToLibrary = make(map[string]int)

	watchCount := 0
	for _, lib := range libs {
		for _, lp := range lib.LibraryPaths {
			m.pathToLibrary[lp.Filepath] = lib.ID
			n, watchErr := m.watchRecursive(watcher, lp.Filepath)
			if watchErr != nil {
				m.log.Warn("failed to watch library path", logger.Data{
					"path":  lp.Filepath,
					"error": watchErr.Error(),
				})
				continue
			}
			watchCount += n
		}
	}

	return watchCount, nil
}

// watchRecursive adds watches on root and all its subdirectories.
// Returns the number of directories added.
func (m *Monitor) watchRecursive(watcher *fsnotify.Watcher, root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible directories
		}
		if !d.IsDir() {
			return nil
		}
		if addErr := watcher.Add(path); addErr != nil {
			m.log.Warn("failed to watch directory", logger.Data{
				"path":  path,
				"error": addErr.Error(),
			})
			return nil
		}
		count++
		return nil
	})
	return count, err
}

// findLibraryID returns the library ID for a file path by checking which
// library path is a prefix of the given path. Returns 0 if no match.
func (m *Monitor) findLibraryID(path string) int {
	for lp, libID := range m.pathToLibrary {
		if strings.HasPrefix(path, lp+string(os.PathSeparator)) || path == lp {
			return libID
		}
	}
	return 0
}

// isScannable returns true if the file extension is one that the scanner handles.
func (m *Monitor) isScannable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}

	// Built-in extensions
	if _, ok := extensionsToScan[ext]; ok {
		return true
	}

	// Plugin-registered extensions
	if m.worker.pluginManager != nil {
		extNoDot := strings.TrimPrefix(ext, ".")
		pluginExts := m.worker.pluginManager.RegisteredFileExtensions()
		if _, ok := pluginExts[extNoDot]; ok {
			return true
		}
		converterExts := m.worker.pluginManager.RegisteredConverterExtensions()
		if _, ok := converterExts[extNoDot]; ok {
			return true
		}
	}

	return false
}

// enqueueExistingFiles walks a directory tree and enqueues any scannable files
// that already exist. This handles the race condition where files are created
// inside a new directory before the watch is added (common with bulk copies on macOS/kqueue).
func (m *Monitor) enqueueExistingFiles(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !m.isScannable(path) {
			return nil
		}
		if isShishoSpecialFile(filepath.Base(path)) {
			return nil
		}
		if m.isIgnored(path) {
			return nil
		}
		libID := m.findLibraryID(path)
		if libID == 0 {
			return nil
		}

		m.mu.Lock()
		if _, ok := m.pending[path]; !ok {
			m.pending[path] = pendingEvent{
				Op:        fsnotify.Create,
				LibraryID: libID,
			}
			m.log.Debug("filesystem event queued (existing file)", logger.Data{
				"path":       path,
				"library_id": libID,
			})
		}
		m.mu.Unlock()

		return nil
	})

	// Reset the debounce timer if we enqueued anything.
	m.mu.Lock()
	if len(m.pending) > 0 {
		if m.timer != nil {
			m.timer.Stop()
		}
		m.timer = time.AfterFunc(m.delay, m.processPendingEvents)
	}
	m.mu.Unlock()
}

// handleEvent processes a single fsnotify event, filtering irrelevant events
// and feeding relevant ones into the debounce mechanism.
func (m *Monitor) handleEvent(watcher *fsnotify.Watcher, event fsnotify.Event) {
	path := event.Name

	if m.isIgnored(path) {
		return
	}

	// Handle new directory creation: add recursive watches so we catch
	// events for files created inside new subdirectories.
	// Also scan for existing files — on macOS (kqueue), files may be created
	// before the watch is added during bulk copies, so we won't get events for them.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			n, watchErr := m.watchRecursive(watcher, path)
			if watchErr != nil {
				m.log.Warn("failed to watch new directory", logger.Data{
					"path":  path,
					"error": watchErr.Error(),
				})
			} else if n > 0 {
				m.log.Debug("added watches for new directory", logger.Data{
					"path":  path,
					"count": n,
				})
			}
			m.enqueueExistingFiles(path)
			return // directories themselves are not scannable files
		}
	}

	// Directory-level Remove/Rename: fsnotify emits the event against the
	// directory path (not the files inside), so the path is not scannable.
	// Queue a synthetic directory event so processDirectoryEvent can cascade
	// cleanup to every DB file whose filepath sits under this directory.
	// Skip shisho-owned files (covers, sidecars) so cover/sidecar removals
	// don't trigger pointless prefix queries — they're already suppressed
	// for creates/writes by isShishoSpecialFile below.
	if (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)) && !m.isScannable(path) {
		if isShishoSpecialFile(filepath.Base(path)) {
			return
		}
		libID := m.findLibraryID(path)
		if libID == 0 {
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()

		existing, ok := m.pending[path]
		if ok {
			existing.Op |= event.Op
			existing.IsDirectory = true
			m.pending[path] = existing
		} else {
			m.pending[path] = pendingEvent{
				Op:          event.Op,
				LibraryID:   libID,
				IsDirectory: true,
			}
		}

		m.log.Debug("filesystem directory event queued", logger.Data{
			"path": path,
			"op":   event.Op.String(),
		})

		if m.timer != nil {
			m.timer.Stop()
		}
		m.timer = time.AfterFunc(m.delay, m.processPendingEvents)
		return
	}

	// Only care about files with scannable extensions.
	// For Remove/Rename the file no longer exists so we can't stat it,
	// but we can still check the extension from the path.
	if !m.isScannable(path) {
		return
	}

	// Skip shisho special files (covers, sidecars)
	if isShishoSpecialFile(filepath.Base(path)) {
		return
	}

	libID := m.findLibraryID(path)
	if libID == 0 {
		return
	}

	// Accumulate event and start/reset the debounce timer.
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.pending[path]
	if ok {
		// Merge operations so we don't lose information about what happened.
		// Clear IsDirectory — this event is for a scannable file at this path,
		// so any prior same-path directory event is superseded (latest wins).
		existing.Op |= event.Op
		existing.IsDirectory = false
		m.pending[path] = existing
	} else {
		m.pending[path] = pendingEvent{
			Op:        event.Op,
			LibraryID: libID,
		}
	}

	m.log.Debug("filesystem event queued", logger.Data{
		"path": path,
		"op":   event.Op.String(),
	})

	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.delay, m.processPendingEvents)
}

// processPendingEvents is called when the debounce timer fires.
// It drains the pending events map and processes each one.
func (m *Monitor) processPendingEvents() {
	// Prevent concurrent processing — time.AfterFunc runs in its own goroutine,
	// so a second batch could fire while the first is still scanning. If that
	// happens, the new events stay in m.pending for the next cycle.
	if !m.processing.TryLock() {
		return
	}
	defer m.processing.Unlock()

	// Drain pending events under the lock.
	m.mu.Lock()
	events := m.pending
	m.pending = make(map[string]pendingEvent)
	m.mu.Unlock()

	if len(events) == 0 {
		return
	}

	ctx := context.Background()

	// Skip processing if a scan job is already running — it will pick up all changes.
	hasActive, err := m.worker.jobService.HasActiveJob(ctx, models.JobTypeScan, nil)
	if err != nil {
		m.log.Err(err).Warn("failed to check for active scan job, re-queuing events")
		m.requeue(events)
		return
	}
	if hasActive {
		m.log.Debug("scan job active, re-queuing events for later")
		m.requeue(events)
		return
	}

	m.log.Info("processing filesystem events", logger.Data{"count": len(events)})

	hadDeletes := false
	booksToOrganize := make(map[int]struct{})
	affectedBookIDs := make(map[int]struct{})
	applyResult := func(result *ScanResult) {
		if result == nil {
			return
		}
		if result.FileDeleted || result.BookDeleted {
			hadDeletes = true
		}
		if result.FileCreated && result.Book != nil {
			booksToOrganize[result.Book.ID] = struct{}{}
		}
		// Track all affected books for targeted search indexing.
		if result.Book != nil {
			affectedBookIDs[result.Book.ID] = struct{}{}
		}
	}
	for path, event := range events {
		if event.IsDirectory {
			for _, result := range m.processDirectoryEvent(ctx, path, event) {
				applyResult(result)
			}
			continue
		}
		applyResult(m.processEvent(ctx, path, event))
	}

	// Organize new books — scanInternal with FilePath mode defers organization,
	// so we must run it here (same as ProcessScanJob's post-scan organization).
	if len(booksToOrganize) > 0 {
		m.organizeBooks(ctx, booksToOrganize)
	}

	// Cleanup orphaned entities after deletes.
	if hadDeletes {
		m.runOrphanCleanup(ctx)
	}

	// Update search indexes for affected books only.
	if m.worker.searchService != nil {
		for bookID := range affectedBookIDs {
			book, err := m.worker.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
			if err != nil {
				// Book was deleted — remove from index.
				_ = m.worker.searchService.DeleteFromBookIndex(ctx, bookID)
				continue
			}
			if err := m.worker.searchService.IndexBook(ctx, book); err != nil {
				m.log.Warn("failed to index book", logger.Data{"book_id": bookID, "error": err.Error()})
			}
		}
	}
}

// requeue puts events back into m.pending (merging with any new arrivals)
// and restarts the debounce timer.
func (m *Monitor) requeue(events map[string]pendingEvent) {
	m.mu.Lock()
	for path, ev := range events {
		if _, ok := m.pending[path]; !ok {
			m.pending[path] = ev
		}
	}
	m.timer = time.AfterFunc(m.delay, m.processPendingEvents)
	m.mu.Unlock()
}

// processDirectoryEvent handles a Remove/Rename event that landed on a
// directory path. It lists every DB file whose filepath sits at or under that
// directory in the given library and delegates cleanup to the existing
// per-file missing-on-disk path via scanInternal(FileID). This covers both
// "directory removed" and "directory renamed away" — in the rename case the
// corresponding Create event on the new path is handled independently and may
// create a fresh book row.
func (m *Monitor) processDirectoryEvent(ctx context.Context, path string, event pendingEvent) []*ScanResult {
	log := m.log.Root(logger.Data{"path": path, "op": event.Op.String()})

	files, err := m.worker.bookService.ListFiles(ctx, books.ListFilesOptions{
		LibraryID:      &event.LibraryID,
		FilepathPrefix: &path,
	})
	if err != nil {
		log.Err(err).Warn("failed to list files under removed directory")
		return nil
	}
	if len(files) == 0 {
		return nil
	}

	log.Info("directory event, cleaning up files", logger.Data{"count": len(files)})

	results := make([]*ScanResult, 0, len(files))
	for _, file := range files {
		result, err := m.worker.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
		if err != nil {
			log.Err(err).Warn("failed to cleanup file under removed directory", logger.Data{
				"file_id":       file.ID,
				"file_filepath": file.Filepath,
			})
			continue
		}
		results = append(results, result)
	}
	return results
}

// processEvent handles a single accumulated event for a file path.
func (m *Monitor) processEvent(ctx context.Context, path string, event pendingEvent) *ScanResult {
	log := m.log.Root(logger.Data{"path": path, "op": event.Op.String()})

	// Remove/Rename without a subsequent Create means the file was deleted.
	isDelete := (event.Op.Has(fsnotify.Remove) || event.Op.Has(fsnotify.Rename)) && !event.Op.Has(fsnotify.Create)

	if isDelete {
		log.Info("file removed, cleaning up")
		file, err := m.worker.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
			Filepath: &path,
		})
		if err != nil {
			// File not in DB (never scanned or already cleaned up).
			return nil
		}
		result, err := m.worker.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
		if err != nil {
			log.Err(err).Warn("failed to cleanup removed file")
		}
		return result
	}

	// File was created or modified — verify it still exists on disk.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	// For Write events on files already in the DB, use FileID + ForceRefresh
	// so metadata is re-read regardless of data source priority.
	if event.Op.Has(fsnotify.Write) {
		file, err := m.worker.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{
			Filepath: &path,
		})
		if err == nil {
			log.Info("file modified, rescanning with force refresh")
			result, err := m.worker.scanInternal(ctx, ScanOptions{
				FileID:       file.ID,
				ForceRefresh: true,
			}, nil)
			if err != nil {
				log.Err(err).Warn("failed to rescan modified file")
			}
			return result
		}
		// File not in DB — fall through to treat as new file.
	}

	// New file (Create or Write for a file not yet in DB).
	log.Info("new file detected, scanning")
	result, err := m.worker.scanInternal(ctx, ScanOptions{
		FilePath:  path,
		LibraryID: event.LibraryID,
	}, nil)
	if err != nil {
		log.Err(err).Warn("failed to scan new file")
	}
	return result
}

// organizeBooks runs file organization for newly created books, moving files into
// organized directory structures when the library has OrganizeFileStructure enabled.
// It uses IgnorePath to suppress fsnotify events generated by the file moves.
func (m *Monitor) organizeBooks(ctx context.Context, bookIDs map[int]struct{}) {
	for bookID := range bookIDs {
		book, err := m.worker.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			m.log.Warn("failed to retrieve book for organization", logger.Data{
				"book_id": bookID,
				"error":   err.Error(),
			})
			continue
		}

		library, err := m.worker.libraryService.RetrieveLibrary(ctx, libraries.RetrieveLibraryOptions{
			ID: &book.LibraryID,
		})
		if err != nil || !library.OrganizeFileStructure {
			continue
		}

		// Ignore old file paths — organization will generate Rename events for these.
		for _, f := range book.Files {
			m.IgnorePath(f.Filepath)
		}

		if err := m.worker.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{OrganizeFiles: true}); err != nil {
			m.log.Warn("failed to organize book", logger.Data{
				"book_id": bookID,
				"error":   err.Error(),
			})
			continue
		}

		// Re-read book to get new file paths and ignore those too —
		// organization generates Create events for the new locations.
		book, err = m.worker.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			continue
		}
		for _, f := range book.Files {
			m.IgnorePath(f.Filepath)
		}
	}
}

// runOrphanCleanup removes entities that are no longer referenced by any books.
func (m *Monitor) runOrphanCleanup(ctx context.Context) {
	m.worker.cleanupOrphanedEntities(ctx, m.log)
}
