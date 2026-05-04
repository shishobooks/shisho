package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/shishobooks/shisho/pkg/models"
)

// PersonFinder is an interface for finding or creating persons.
type PersonFinder interface {
	FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error)
}

// GenreFinder is an interface for finding or creating genres.
type GenreFinder interface {
	FindOrCreateGenre(ctx context.Context, name string, libraryID int) (*models.Genre, error)
}

// TagFinder is an interface for finding or creating tags.
type TagFinder interface {
	FindOrCreateTag(ctx context.Context, name string, libraryID int) (*models.Tag, error)
}

// SeriesFinder is an interface for finding or creating series.
type SeriesFinder interface {
	FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error)
}

// PublisherFinder is an interface for finding or creating publishers.
type PublisherFinder interface {
	FindOrCreatePublisher(ctx context.Context, name string, libraryID int) (*models.Publisher, error)
}

// ImprintFinder is an interface for finding or creating imprints.
type ImprintFinder interface {
	FindOrCreateImprint(ctx context.Context, name string, libraryID int) (*models.Imprint, error)
}

// AliasLister fetches alias names for resolved resources so the cache can
// pre-populate entries for every name variant (canonical + aliases).
type AliasLister interface {
	ListPersonAliases(ctx context.Context, personID int) ([]string, error)
	ListGenreAliases(ctx context.Context, genreID int) ([]string, error)
	ListTagAliases(ctx context.Context, tagID int) ([]string, error)
	ListSeriesAliases(ctx context.Context, seriesID int) ([]string, error)
	ListPublisherAliases(ctx context.Context, publisherID int) ([]string, error)
	ListImprintAliases(ctx context.Context, imprintID int) ([]string, error)
}

// ScanCache provides thread-safe caching for entity lookups during parallel file processing.
// It uses sync.Map for cache storage and per-key mutexes to prevent duplicate concurrent DB calls.
type ScanCache struct {
	// Entity caches
	persons    sync.Map // map[string]*models.Person
	genres     sync.Map // map[string]*models.Genre
	tags       sync.Map // map[string]*models.Tag
	series     sync.Map // map[string]*models.Series
	publishers sync.Map // map[string]*models.Publisher
	imprints   sync.Map // map[string]*models.Imprint

	// Per-key mutexes to prevent duplicate concurrent DB calls
	personMu    sync.Map // map[string]*sync.Mutex
	genreMu     sync.Map // map[string]*sync.Mutex
	tagMu       sync.Map // map[string]*sync.Mutex
	seriesMu    sync.Map // map[string]*sync.Mutex
	publisherMu sync.Map // map[string]*sync.Mutex
	imprintMu   sync.Map // map[string]*sync.Mutex

	// Per-path mutexes to prevent concurrent book creation for same path
	bookPathMu sync.Map // map[string]*sync.Mutex

	// Per-book mutexes to prevent concurrent relationship updates for same book
	bookMu sync.Map // map[int]*sync.Mutex

	// Pre-loaded file lookup for fast path during batch scans.
	// Written during LoadKnownFiles at init and again via AddKnownFile in the
	// move reconciliation phase — both of which happen single-threaded before
	// the parallel worker pool starts. During the parallel scan itself the
	// map is read-only, so a regular map is safe without synchronization.
	knownFiles map[string]*models.File

	// movedOrphanIDs holds file IDs matched by the move reconciliation phase.
	// Written once (single-threaded) before the parallel worker pool starts,
	// then read-only during orphan cleanup. Nil means reconciliation did not run.
	movedOrphanIDs map[int]struct{}

	// movedBookIDs holds book IDs whose files were matched by the move
	// reconciliation phase. The parent scan loop merges this into its
	// booksToOrganize set so organize_file_structure runs on these books
	// after the scan (renaming folders back into the structured layout).
	movedBookIDs map[int]struct{}

	aliasLister AliasLister

	// libraryRootPaths caches the library's root paths (from
	// library.LibraryPaths) so syncBookFilepathAfterMove can enforce its
	// "don't set Book.Filepath to a library root" guard without a DB
	// lookup per call. Populated by the scan before reconciliation runs.
	// Nil means the caller should fall back to RetrieveLibrary.
	libraryRootPaths []string

	// Counters for cache hits/misses (atomic for thread safety)
	personCount    atomic.Int64
	genreCount     atomic.Int64
	tagCount       atomic.Int64
	seriesCount    atomic.Int64
	publisherCount atomic.Int64
	imprintCount   atomic.Int64
}

// NewScanCache creates a new ScanCache.
func NewScanCache() *ScanCache {
	return &ScanCache{
		knownFiles: make(map[string]*models.File),
	}
}

// SetAliasLister configures alias lookup so that cache misses pre-populate
// entries for the canonical name and every alias of the resolved resource.
func (c *ScanCache) SetAliasLister(lister AliasLister) {
	c.aliasLister = lister
}

// LoadKnownFiles populates the known files cache from a list of existing files.
// Replaces any previously-set knownFiles map (e.g. from NewScanCache).
func (c *ScanCache) LoadKnownFiles(files []*models.File) {
	c.knownFiles = make(map[string]*models.File, len(files))
	for _, f := range files {
		c.knownFiles[f.Filepath] = f
	}
}

// GetKnownFile returns a known file by path, or nil if not found.
func (c *ScanCache) GetKnownFile(path string) *models.File {
	return c.knownFiles[path]
}

// cacheKey generates a unique key for an entity based on name and library ID.
func cacheKey(name string, libraryID int) string {
	return fmt.Sprintf("%s|%d", name, libraryID)
}

// getMutex retrieves or creates a mutex for the given key.
func getMutex(mutexMap *sync.Map, key any) *sync.Mutex {
	mu, _ := mutexMap.LoadOrStore(key, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// populateAliasEntries stores the resource in the cache under its canonical name
// and every alias name, so subsequent lookups for any variant are cache hits.
func populateAliasEntries(cacheMap *sync.Map, canonicalName string, libraryID int, value any, aliasNames []string) {
	canonKey := cacheKey(canonicalName, libraryID)
	cacheMap.LoadOrStore(canonKey, value)
	for _, alias := range aliasNames {
		aliasKey := cacheKey(alias, libraryID)
		cacheMap.LoadOrStore(aliasKey, value)
	}
}

// GetOrCreatePerson retrieves a person from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same person.
func (c *ScanCache) GetOrCreatePerson(ctx context.Context, name string, libraryID int, svc PersonFinder) (*models.Person, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.persons.Load(key); ok {
		return val.(*models.Person), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.personMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.persons.Load(key); ok {
		return val.(*models.Person), nil
	}

	// Call service
	person, err := svc.FindOrCreatePerson(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.persons.Store(key, person)
	c.personCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListPersonAliases(ctx, person.ID); err == nil {
			populateAliasEntries(&c.persons, person.Name, libraryID, person, aliasNames)
		}
	}

	return person, nil
}

// GetOrCreateGenre retrieves a genre from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same genre.
func (c *ScanCache) GetOrCreateGenre(ctx context.Context, name string, libraryID int, svc GenreFinder) (*models.Genre, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.genres.Load(key); ok {
		return val.(*models.Genre), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.genreMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.genres.Load(key); ok {
		return val.(*models.Genre), nil
	}

	// Call service
	genre, err := svc.FindOrCreateGenre(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.genres.Store(key, genre)
	c.genreCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListGenreAliases(ctx, genre.ID); err == nil {
			populateAliasEntries(&c.genres, genre.Name, libraryID, genre, aliasNames)
		}
	}

	return genre, nil
}

// GetOrCreateTag retrieves a tag from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same tag.
func (c *ScanCache) GetOrCreateTag(ctx context.Context, name string, libraryID int, svc TagFinder) (*models.Tag, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.tags.Load(key); ok {
		return val.(*models.Tag), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.tagMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.tags.Load(key); ok {
		return val.(*models.Tag), nil
	}

	// Call service
	tag, err := svc.FindOrCreateTag(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.tags.Store(key, tag)
	c.tagCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListTagAliases(ctx, tag.ID); err == nil {
			populateAliasEntries(&c.tags, tag.Name, libraryID, tag, aliasNames)
		}
	}

	return tag, nil
}

// GetOrCreateSeries retrieves a series from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same series.
func (c *ScanCache) GetOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string, svc SeriesFinder) (*models.Series, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.series.Load(key); ok {
		return val.(*models.Series), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.seriesMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.series.Load(key); ok {
		return val.(*models.Series), nil
	}

	// Call service
	series, err := svc.FindOrCreateSeries(ctx, name, libraryID, nameSource)
	if err != nil {
		return nil, err
	}

	c.series.Store(key, series)
	c.seriesCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListSeriesAliases(ctx, series.ID); err == nil {
			populateAliasEntries(&c.series, series.Name, libraryID, series, aliasNames)
		}
	}

	return series, nil
}

// PersonCount returns the number of unique persons in the cache.
func (c *ScanCache) PersonCount() int {
	return int(c.personCount.Load())
}

// GenreCount returns the number of unique genres in the cache.
func (c *ScanCache) GenreCount() int {
	return int(c.genreCount.Load())
}

// TagCount returns the number of unique tags in the cache.
func (c *ScanCache) TagCount() int {
	return int(c.tagCount.Load())
}

// SeriesCount returns the number of unique series in the cache.
func (c *ScanCache) SeriesCount() int {
	return int(c.seriesCount.Load())
}

// GetOrCreatePublisher retrieves a publisher from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same publisher.
func (c *ScanCache) GetOrCreatePublisher(ctx context.Context, name string, libraryID int, svc PublisherFinder) (*models.Publisher, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.publishers.Load(key); ok {
		return val.(*models.Publisher), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.publisherMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.publishers.Load(key); ok {
		return val.(*models.Publisher), nil
	}

	// Call service
	publisher, err := svc.FindOrCreatePublisher(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.publishers.Store(key, publisher)
	c.publisherCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListPublisherAliases(ctx, publisher.ID); err == nil {
			populateAliasEntries(&c.publishers, publisher.Name, libraryID, publisher, aliasNames)
		}
	}

	return publisher, nil
}

// GetOrCreateImprint retrieves an imprint from the cache or calls the service to find/create one.
// It uses per-key locking to prevent duplicate concurrent DB calls for the same imprint.
func (c *ScanCache) GetOrCreateImprint(ctx context.Context, name string, libraryID int, svc ImprintFinder) (*models.Imprint, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.imprints.Load(key); ok {
		return val.(*models.Imprint), nil
	}

	// Slow path: acquire per-key mutex
	mu := getMutex(&c.imprintMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.imprints.Load(key); ok {
		return val.(*models.Imprint), nil
	}

	// Call service
	imprint, err := svc.FindOrCreateImprint(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.imprints.Store(key, imprint)
	c.imprintCount.Add(1)

	if c.aliasLister != nil {
		if aliasNames, err := c.aliasLister.ListImprintAliases(ctx, imprint.ID); err == nil {
			populateAliasEntries(&c.imprints, imprint.Name, libraryID, imprint, aliasNames)
		}
	}

	return imprint, nil
}

// PublisherCount returns the number of unique publishers in the cache.
func (c *ScanCache) PublisherCount() int {
	return int(c.publisherCount.Load())
}

// ImprintCount returns the number of unique imprints in the cache.
func (c *ScanCache) ImprintCount() int {
	return int(c.imprintCount.Load())
}

// LockBookPath acquires a lock for the given book path to prevent concurrent book creation.
// Multiple files in the same directory would have the same book path, so we need to serialize
// their book creation to avoid unique constraint violations.
// Returns an unlock function that must be called when done.
func (c *ScanCache) LockBookPath(path string, libraryID int) func() {
	key := cacheKey(path, libraryID)
	mu := getMutex(&c.bookPathMu, key)
	mu.Lock()
	return mu.Unlock
}

// LockBook acquires a lock for the given book ID to prevent concurrent relationship updates.
// Multiple files belonging to the same book may be processed in parallel, so we need to
// serialize their relationship updates (authors, series, genres, tags) to avoid race conditions.
// Returns an unlock function that must be called when done.
func (c *ScanCache) LockBook(bookID int) func() {
	mu := getMutex(&c.bookMu, bookID)
	mu.Lock()
	return mu.Unlock
}

// SetMovedOrphanIDs stores the set of file IDs identified as moved orphans.
// Must be called before the parallel worker pool starts (not thread-safe for writes).
func (c *ScanCache) SetMovedOrphanIDs(ids map[int]struct{}) {
	c.movedOrphanIDs = ids
}

// IsMovedOrphan returns true if the given file ID was matched by the move
// reconciliation phase and should be skipped by orphan cleanup.
func (c *ScanCache) IsMovedOrphan(id int) bool {
	if c.movedOrphanIDs == nil {
		return false
	}
	_, ok := c.movedOrphanIDs[id]
	return ok
}

// SetMovedBookIDs stores the set of book IDs whose files were matched by the
// move reconciliation phase. The scan loop reads this after processing to
// ensure organize_file_structure runs on the moved books.
func (c *ScanCache) SetMovedBookIDs(ids map[int]struct{}) {
	c.movedBookIDs = ids
}

// MovedBookIDs returns the set of book IDs whose files were matched as moves.
// Returns nil if reconciliation did not run or found no matches.
func (c *ScanCache) MovedBookIDs() map[int]struct{} {
	return c.movedBookIDs
}

// SetLibraryRootPaths caches the library's root paths so downstream helpers
// (currently syncBookFilepathAfterMove) can check "is this a library root?"
// without a per-call DB lookup.
func (c *ScanCache) SetLibraryRootPaths(paths []string) {
	c.libraryRootPaths = paths
}

// LibraryRootPaths returns the cached library root paths. Returns nil if
// SetLibraryRootPaths was not called for this scan.
func (c *ScanCache) LibraryRootPaths() []string {
	return c.libraryRootPaths
}

// AddKnownFile adds a file to the known-files cache at its new path. Used after
// move reconciliation updates a file's filepath so the parallel processing loop
// treats the new path as already known (skipping it instead of creating a duplicate).
// NewScanCache guarantees the map is initialized, so no nil check is needed.
func (c *ScanCache) AddKnownFile(f *models.File) {
	c.knownFiles[f.Filepath] = f
}
