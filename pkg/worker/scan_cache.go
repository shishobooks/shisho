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
	return &ScanCache{}
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
