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

	// Self-write ignore list: paths written by the scanner that should not trigger rescans.
	ignoreMu sync.RWMutex
	ignored  map[string]time.Time

	// Library path → library ID mapping for resolving which library a file belongs to.
	pathToLibrary map[string]int

	shutdown chan struct{}
	done     chan struct{}
}

// minMonitorDelaySeconds is the minimum allowed debounce delay.
// Values below this are clamped to prevent instant event firing.
const minMonitorDelaySeconds = 5

// newMonitor creates a new filesystem monitor for the given worker.
func newMonitor(w *Worker) *Monitor {
	delaySec := w.config.LibraryMonitorDelaySeconds
	if delaySec < minMonitorDelaySeconds {
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
		"library_paths":       m.libraryPathList(),
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

// libraryPathList returns the watched library paths for logging.
func (m *Monitor) libraryPathList() []string {
	paths := make([]string, 0, len(m.pathToLibrary))
	for p := range m.pathToLibrary {
		paths = append(paths, p)
	}
	return paths
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
			m.log.Info("filesystem event queued (existing file)", logger.Data{
				"path":       path,
				"library_id": libID,
				"pending":    len(m.pending),
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
				m.log.Info("added watches for new directory", logger.Data{
					"path":  path,
					"count": n,
				})
			}
			m.enqueueExistingFiles(path)
			return // directories themselves are not scannable files
		}
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
		existing.Op |= event.Op
		m.pending[path] = existing
	} else {
		m.pending[path] = pendingEvent{
			Op:        event.Op,
			LibraryID: libID,
		}
	}

	m.log.Info("filesystem event queued", logger.Data{
		"path":       path,
		"op":         event.Op.String(),
		"library_id": libID,
		"pending":    len(m.pending),
	})

	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(m.delay, m.processPendingEvents)
}

// processPendingEvents is called when the debounce timer fires.
// It drains the pending events map and processes each one.
func (m *Monitor) processPendingEvents() {
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
		m.log.Err(err).Warn("failed to check for active scan job")
		return
	}
	if hasActive {
		m.log.Debug("scan job active, deferring to running scan")
		return
	}

	m.log.Info("processing filesystem events", logger.Data{"count": len(events)})

	hadDeletes := false
	for path, event := range events {
		result := m.processEvent(ctx, path, event)
		if result != nil && (result.FileDeleted || result.BookDeleted) {
			hadDeletes = true
		}
	}

	// Cleanup orphaned entities after deletes.
	if hadDeletes {
		m.runOrphanCleanup(ctx)
	}

	// Rebuild search indexes for the affected books.
	if m.worker.searchService != nil {
		if err := m.worker.searchService.RebuildAllIndexes(ctx); err != nil {
			m.log.Err(err).Warn("failed to rebuild search indexes")
		}
	}
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

// runOrphanCleanup removes entities that are no longer referenced by any books.
func (m *Monitor) runOrphanCleanup(ctx context.Context) {
	if n, err := m.worker.seriesService.CleanupOrphanedSeries(ctx); err != nil {
		m.log.Err(err).Warn("failed to cleanup orphaned series")
	} else if n > 0 {
		m.log.Info("cleaned up orphaned series", logger.Data{"count": n})
	}

	if n, err := m.worker.personService.CleanupOrphanedPeople(ctx); err != nil {
		m.log.Err(err).Warn("failed to cleanup orphaned people")
	} else if n > 0 {
		m.log.Info("cleaned up orphaned people", logger.Data{"count": n})
	}

	if n, err := m.worker.genreService.CleanupOrphanedGenres(ctx); err != nil {
		m.log.Err(err).Warn("failed to cleanup orphaned genres")
	} else if n > 0 {
		m.log.Info("cleaned up orphaned genres", logger.Data{"count": n})
	}

	if n, err := m.worker.tagService.CleanupOrphanedTags(ctx); err != nil {
		m.log.Err(err).Warn("failed to cleanup orphaned tags")
	} else if n > 0 {
		m.log.Info("cleaned up orphaned tags", logger.Data{"count": n})
	}
}
