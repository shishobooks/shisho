package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPersonFinder is a mock implementation of PersonFinder for testing.
type mockPersonFinder struct {
	mu        sync.Mutex
	callCount int
	persons   map[string]*models.Person
}

func newMockPersonFinder() *mockPersonFinder {
	return &mockPersonFinder{
		persons: make(map[string]*models.Person),
	}
}

func (m *mockPersonFinder) FindOrCreatePerson(_ context.Context, name string, libraryID int) (*models.Person, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	key := fmt.Sprintf("%s|%d", name, libraryID)
	if p, ok := m.persons[key]; ok {
		return p, nil
	}

	p := &models.Person{
		ID:        m.callCount,
		Name:      name,
		LibraryID: libraryID,
	}
	m.persons[key] = p
	return p, nil
}

func (m *mockPersonFinder) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// mockGenreFinder is a mock implementation of GenreFinder for testing.
type mockGenreFinder struct {
	mu        sync.Mutex
	callCount int
	genres    map[string]*models.Genre
}

func newMockGenreFinder() *mockGenreFinder {
	return &mockGenreFinder{
		genres: make(map[string]*models.Genre),
	}
}

func (m *mockGenreFinder) FindOrCreateGenre(_ context.Context, name string, libraryID int) (*models.Genre, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	key := fmt.Sprintf("%s|%d", name, libraryID)
	if g, ok := m.genres[key]; ok {
		return g, nil
	}

	g := &models.Genre{
		ID:        m.callCount,
		Name:      name,
		LibraryID: libraryID,
	}
	m.genres[key] = g
	return g, nil
}

func (m *mockGenreFinder) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// mockTagFinder is a mock implementation of TagFinder for testing.
type mockTagFinder struct {
	mu        sync.Mutex
	callCount int
	tags      map[string]*models.Tag
}

func newMockTagFinder() *mockTagFinder {
	return &mockTagFinder{
		tags: make(map[string]*models.Tag),
	}
}

func (m *mockTagFinder) FindOrCreateTag(_ context.Context, name string, libraryID int) (*models.Tag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	key := fmt.Sprintf("%s|%d", name, libraryID)
	if t, ok := m.tags[key]; ok {
		return t, nil
	}

	t := &models.Tag{
		ID:        m.callCount,
		Name:      name,
		LibraryID: libraryID,
	}
	m.tags[key] = t
	return t, nil
}

func (m *mockTagFinder) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// mockSeriesFinder is a mock implementation of SeriesFinder for testing.
type mockSeriesFinder struct {
	mu        sync.Mutex
	callCount int
	series    map[string]*models.Series
}

func newMockSeriesFinder() *mockSeriesFinder {
	return &mockSeriesFinder{
		series: make(map[string]*models.Series),
	}
}

func (m *mockSeriesFinder) FindOrCreateSeries(_ context.Context, name string, libraryID int, nameSource string) (*models.Series, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	key := fmt.Sprintf("%s|%d", name, libraryID)
	if s, ok := m.series[key]; ok {
		return s, nil
	}

	s := &models.Series{
		ID:         m.callCount,
		Name:       name,
		LibraryID:  libraryID,
		NameSource: nameSource,
	}
	m.series[key] = s
	return s, nil
}

func (m *mockSeriesFinder) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestScanCache_GetOrCreatePerson(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockPersonFinder()

	// First call should hit the service
	person1, err := cache.GetOrCreatePerson(ctx, "John Doe", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, "John Doe", person1.Name)
	assert.Equal(t, 1, person1.LibraryID)
	assert.Equal(t, 1, mockSvc.CallCount())

	// Second call with same name and library should hit cache
	person2, err := cache.GetOrCreatePerson(ctx, "John Doe", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, person1.ID, person2.ID)
	assert.Equal(t, 1, mockSvc.CallCount()) // No additional service call

	// Call with different library should hit service
	person3, err := cache.GetOrCreatePerson(ctx, "John Doe", 2, mockSvc)
	require.NoError(t, err)
	assert.NotEqual(t, person1.ID, person3.ID)
	assert.Equal(t, 2, person3.LibraryID)
	assert.Equal(t, 2, mockSvc.CallCount())

	// Call with different name should hit service
	person4, err := cache.GetOrCreatePerson(ctx, "Jane Doe", 1, mockSvc)
	require.NoError(t, err)
	assert.NotEqual(t, person1.ID, person4.ID)
	assert.Equal(t, "Jane Doe", person4.Name)
	assert.Equal(t, 3, mockSvc.CallCount())
}

func TestScanCache_GetOrCreateGenre(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockGenreFinder()

	// First call should hit the service
	genre1, err := cache.GetOrCreateGenre(ctx, "Fiction", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, "Fiction", genre1.Name)
	assert.Equal(t, 1, mockSvc.CallCount())

	// Second call with same name and library should hit cache
	genre2, err := cache.GetOrCreateGenre(ctx, "Fiction", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, genre1.ID, genre2.ID)
	assert.Equal(t, 1, mockSvc.CallCount())
}

func TestScanCache_GetOrCreateTag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockTagFinder()

	// First call should hit the service
	tag1, err := cache.GetOrCreateTag(ctx, "bestseller", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, "bestseller", tag1.Name)
	assert.Equal(t, 1, mockSvc.CallCount())

	// Second call with same name and library should hit cache
	tag2, err := cache.GetOrCreateTag(ctx, "bestseller", 1, mockSvc)
	require.NoError(t, err)
	assert.Equal(t, tag1.ID, tag2.ID)
	assert.Equal(t, 1, mockSvc.CallCount())
}

func TestScanCache_GetOrCreateSeries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockSeriesFinder()

	// First call should hit the service
	series1, err := cache.GetOrCreateSeries(ctx, "Harry Potter", 1, "metadata", mockSvc)
	require.NoError(t, err)
	assert.Equal(t, "Harry Potter", series1.Name)
	assert.Equal(t, "metadata", series1.NameSource)
	assert.Equal(t, 1, mockSvc.CallCount())

	// Second call with same name and library should hit cache
	series2, err := cache.GetOrCreateSeries(ctx, "Harry Potter", 1, "filepath", mockSvc)
	require.NoError(t, err)
	assert.Equal(t, series1.ID, series2.ID)
	assert.Equal(t, 1, mockSvc.CallCount())
}

func TestScanCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockPersonFinder()

	const numGoroutines = 100
	const personName = "Concurrent Author"
	const libraryID = 1

	var wg sync.WaitGroup
	var successCount atomic.Int32
	results := make(chan *models.Person, numGoroutines)

	// Launch 100 goroutines all trying to get the same person
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			person, err := cache.GetOrCreatePerson(ctx, personName, libraryID, mockSvc)
			if err == nil {
				successCount.Add(1)
				results <- person
			}
		}()
	}

	wg.Wait()
	close(results)

	// All goroutines should succeed
	assert.Equal(t, int32(numGoroutines), successCount.Load())

	// Service should only be called once due to per-key locking
	assert.Equal(t, 1, mockSvc.CallCount())

	// All results should be the same person
	var firstPerson *models.Person
	for person := range results {
		if firstPerson == nil {
			firstPerson = person
		} else {
			assert.Equal(t, firstPerson.ID, person.ID)
		}
	}
}

func TestScanCache_ConcurrentDifferentKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	mockSvc := newMockPersonFinder()

	const numGoroutines = 100
	const libraryID = 1

	var wg sync.WaitGroup
	var successCount atomic.Int32

	// Launch 100 goroutines each trying to get a different person
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			personName := fmt.Sprintf("Author %d", idx)
			_, err := cache.GetOrCreatePerson(ctx, personName, libraryID, mockSvc)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// All goroutines should succeed
	assert.Equal(t, int32(numGoroutines), successCount.Load())

	// Service should be called once for each unique person
	assert.Equal(t, numGoroutines, mockSvc.CallCount())

	// Cache should have all persons
	assert.Equal(t, numGoroutines, cache.PersonCount())
}

func TestScanCache_Stats(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	personSvc := newMockPersonFinder()
	genreSvc := newMockGenreFinder()
	tagSvc := newMockTagFinder()
	seriesSvc := newMockSeriesFinder()

	// Initially all counts should be 0
	assert.Equal(t, 0, cache.PersonCount())
	assert.Equal(t, 0, cache.GenreCount())
	assert.Equal(t, 0, cache.TagCount())
	assert.Equal(t, 0, cache.SeriesCount())

	// Add some entities
	_, err := cache.GetOrCreatePerson(ctx, "Author 1", 1, personSvc)
	require.NoError(t, err)
	_, err = cache.GetOrCreatePerson(ctx, "Author 2", 1, personSvc)
	require.NoError(t, err)
	_, err = cache.GetOrCreatePerson(ctx, "Author 1", 1, personSvc) // Cache hit
	require.NoError(t, err)

	_, err = cache.GetOrCreateGenre(ctx, "Fiction", 1, genreSvc)
	require.NoError(t, err)
	_, err = cache.GetOrCreateGenre(ctx, "Fiction", 2, genreSvc) // Different library
	require.NoError(t, err)

	_, err = cache.GetOrCreateTag(ctx, "bestseller", 1, tagSvc)
	require.NoError(t, err)

	_, err = cache.GetOrCreateSeries(ctx, "Series A", 1, "metadata", seriesSvc)
	require.NoError(t, err)
	_, err = cache.GetOrCreateSeries(ctx, "Series B", 1, "filepath", seriesSvc)
	require.NoError(t, err)
	_, err = cache.GetOrCreateSeries(ctx, "Series C", 1, "metadata", seriesSvc)
	require.NoError(t, err)

	// Verify counts
	assert.Equal(t, 2, cache.PersonCount())
	assert.Equal(t, 2, cache.GenreCount())
	assert.Equal(t, 1, cache.TagCount())
	assert.Equal(t, 3, cache.SeriesCount())
}

func TestScanCache_ConcurrentMixedTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := NewScanCache()
	personSvc := newMockPersonFinder()
	genreSvc := newMockGenreFinder()
	tagSvc := newMockTagFinder()
	seriesSvc := newMockSeriesFinder()

	const numGoroutines = 25 // 25 goroutines per type = 100 total

	var wg sync.WaitGroup

	// Launch goroutines for each entity type
	for i := 0; i < numGoroutines; i++ {
		wg.Add(4)

		go func(idx int) {
			defer wg.Done()
			_, _ = cache.GetOrCreatePerson(ctx, fmt.Sprintf("Author %d", idx), 1, personSvc)
		}(i)

		go func(idx int) {
			defer wg.Done()
			_, _ = cache.GetOrCreateGenre(ctx, fmt.Sprintf("Genre %d", idx), 1, genreSvc)
		}(i)

		go func(idx int) {
			defer wg.Done()
			_, _ = cache.GetOrCreateTag(ctx, fmt.Sprintf("Tag %d", idx), 1, tagSvc)
		}(i)

		go func(idx int) {
			defer wg.Done()
			_, _ = cache.GetOrCreateSeries(ctx, fmt.Sprintf("Series %d", idx), 1, "metadata", seriesSvc)
		}(i)
	}

	wg.Wait()

	// Verify all entities were cached
	assert.Equal(t, numGoroutines, cache.PersonCount())
	assert.Equal(t, numGoroutines, cache.GenreCount())
	assert.Equal(t, numGoroutines, cache.TagCount())
	assert.Equal(t, numGoroutines, cache.SeriesCount())

	// Verify service calls match
	assert.Equal(t, numGoroutines, personSvc.CallCount())
	assert.Equal(t, numGoroutines, genreSvc.CallCount())
	assert.Equal(t, numGoroutines, tagSvc.CallCount())
	assert.Equal(t, numGoroutines, seriesSvc.CallCount())
}

func TestScanCache_LockBookPath(t *testing.T) {
	t.Parallel()

	cache := NewScanCache()

	const numGoroutines = 50
	const bookPath = "/library/books/TestBook"
	const libraryID = 1

	var wg sync.WaitGroup
	var counter atomic.Int32
	var maxConcurrent atomic.Int32

	// Launch goroutines all trying to lock the same path
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := cache.LockBookPath(bookPath, libraryID)
			defer unlock()

			// Increment counter while holding lock
			current := counter.Add(1)

			// Track max concurrent (should always be 1 with proper locking)
			for {
				oldMax := maxConcurrent.Load()
				if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
					break
				}
			}

			// Simulate some work
			time.Sleep(time.Microsecond)

			counter.Add(-1)
		}()
	}

	wg.Wait()

	// With proper locking, max concurrent should be 1
	assert.Equal(t, int32(1), maxConcurrent.Load(), "lock should serialize access")
}

func TestScanCache_LockBook(t *testing.T) {
	t.Parallel()

	cache := NewScanCache()

	const numGoroutines = 50
	const bookID = 123

	var wg sync.WaitGroup
	var counter atomic.Int32
	var maxConcurrent atomic.Int32

	// Launch goroutines all trying to lock the same book
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := cache.LockBook(bookID)
			defer unlock()

			// Increment counter while holding lock
			current := counter.Add(1)

			// Track max concurrent (should always be 1 with proper locking)
			for {
				oldMax := maxConcurrent.Load()
				if current <= oldMax || maxConcurrent.CompareAndSwap(oldMax, current) {
					break
				}
			}

			// Simulate some work
			time.Sleep(time.Microsecond)

			counter.Add(-1)
		}()
	}

	wg.Wait()

	// With proper locking, max concurrent should be 1
	assert.Equal(t, int32(1), maxConcurrent.Load(), "lock should serialize access")
}

func TestScanCache_LockBook_DifferentBooks(t *testing.T) {
	t.Parallel()

	cache := NewScanCache()

	const numBooks = 10
	const goroutinesPerBook = 5

	var wg sync.WaitGroup
	results := make([]atomic.Int32, numBooks)

	// Launch goroutines for different books - they should be able to run concurrently
	for bookID := 0; bookID < numBooks; bookID++ {
		for i := 0; i < goroutinesPerBook; i++ {
			wg.Add(1)
			go func(bid int) {
				defer wg.Done()
				unlock := cache.LockBook(bid)
				defer unlock()

				results[bid].Add(1)
				time.Sleep(time.Microsecond)
			}(bookID)
		}
	}

	wg.Wait()

	// Each book should have had all its goroutines complete
	for i := 0; i < numBooks; i++ {
		assert.Equal(t, int32(goroutinesPerBook), results[i].Load(),
			"all goroutines for book %d should complete", i)
	}
}
