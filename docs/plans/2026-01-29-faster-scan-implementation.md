# Faster Library Scan Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Optimize library scans by introducing parallel file processing and batched database operations, reducing scan time by 5-10x for libraries with 100+ books.

**Architecture:** A worker pool processes files in parallel, each worker uses a shared thread-safe cache (`sync.Map`) for person/genre/tag/series lookups to avoid redundant DB queries. Relationship updates (authors, narrators, genres, tags, series) are batched into single bulk INSERT operations per book within a transaction.

**Tech Stack:** Go, Bun ORM, sync.Map for thread-safe caching

---

## Task 1: Add db field to Worker struct

**Files:**
- Modify: `pkg/worker/worker.go:37-65`

**Step 1: Write the failing test**

No test needed - this is a simple structural change that will be validated by compilation and subsequent tasks.

**Step 2: Add db field to Worker struct**

In `pkg/worker/worker.go`, add a `db` field to the Worker struct:

```go
type Worker struct {
	config *config.Config
	log    logger.Logger
	db     *bun.DB  // Add this line

	processFuncs map[string]func(ctx context.Context, job *models.Job, jobLog *joblogs.JobLogger) error

	// ... rest of fields unchanged
}
```

**Step 3: Store db in constructor**

In `pkg/worker/worker.go`, update `New()` function to store the db:

```go
func New(cfg *config.Config, db *bun.DB, pm *plugins.Manager) *Worker {
	// ... existing service creation ...

	w := &Worker{
		config: cfg,
		log:    logger.New(),
		db:     db,  // Add this line

		// ... rest of fields unchanged
	}
	// ...
}
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Success with no errors

**Step 5: Commit**

```bash
git add pkg/worker/worker.go
git commit -m "$(cat <<'EOF'
[Backend] Add db field to Worker struct for batch operations

Preparation for parallel scan optimization - the Worker needs direct
database access for running transactions with bulk inserts.
EOF
)"
```

---

## Task 2: Create ScanCache with thread-safe lookups

**Files:**
- Create: `pkg/worker/scan_cache.go`
- Create: `pkg/worker/scan_cache_test.go`

**Step 1: Write the failing test**

Create `pkg/worker/scan_cache_test.go`:

```go
package worker

import (
	"context"
	"sync"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPersonService implements the PersonFinder interface for testing
type mockPersonService struct {
	mu      sync.Mutex
	calls   int
	persons map[string]*models.Person
}

func newMockPersonService() *mockPersonService {
	return &mockPersonService{
		persons: make(map[string]*models.Person),
	}
}

func (m *mockPersonService) FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++

	key := name + "|" + string(rune(libraryID))
	if p, ok := m.persons[key]; ok {
		return p, nil
	}
	p := &models.Person{ID: m.calls, Name: name, LibraryID: libraryID}
	m.persons[key] = p
	return p, nil
}

func TestScanCache_GetOrCreatePerson(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cache := NewScanCache()
	mock := newMockPersonService()

	// First call should hit the service
	p1, err := cache.GetOrCreatePerson(ctx, "Author Name", 1, mock)
	require.NoError(t, err)
	assert.Equal(t, "Author Name", p1.Name)
	assert.Equal(t, 1, mock.calls)

	// Second call with same name should use cache
	p2, err := cache.GetOrCreatePerson(ctx, "Author Name", 1, mock)
	require.NoError(t, err)
	assert.Equal(t, p1.ID, p2.ID)
	assert.Equal(t, 1, mock.calls) // Still 1 - cache hit

	// Different name should hit service again
	p3, err := cache.GetOrCreatePerson(ctx, "Other Author", 1, mock)
	require.NoError(t, err)
	assert.NotEqual(t, p1.ID, p3.ID)
	assert.Equal(t, 2, mock.calls)

	// Same name but different library should hit service
	p4, err := cache.GetOrCreatePerson(ctx, "Author Name", 2, mock)
	require.NoError(t, err)
	assert.NotEqual(t, p1.ID, p4.ID)
	assert.Equal(t, 3, mock.calls)
}

func TestScanCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cache := NewScanCache()
	mock := newMockPersonService()

	var wg sync.WaitGroup
	results := make([]*models.Person, 100)

	// 100 goroutines all requesting the same person
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p, err := cache.GetOrCreatePerson(ctx, "Same Author", 1, mock)
			require.NoError(t, err)
			results[idx] = p
		}(i)
	}

	wg.Wait()

	// All should get the same person
	for i := 1; i < 100; i++ {
		assert.Equal(t, results[0].ID, results[i].ID)
	}

	// Service should only have been called once (cache deduplication)
	// Note: Due to race, it might be called more than once but should be minimal
	assert.LessOrEqual(t, mock.calls, 5, "Expected minimal service calls due to caching")
}

func TestScanCache_Stats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cache := NewScanCache()
	mock := newMockPersonService()

	assert.Equal(t, 0, cache.PersonCount())

	_, _ = cache.GetOrCreatePerson(ctx, "Author 1", 1, mock)
	assert.Equal(t, 1, cache.PersonCount())

	_, _ = cache.GetOrCreatePerson(ctx, "Author 2", 1, mock)
	assert.Equal(t, 2, cache.PersonCount())

	// Same person again shouldn't increase count
	_, _ = cache.GetOrCreatePerson(ctx, "Author 1", 1, mock)
	assert.Equal(t, 2, cache.PersonCount())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/worker -run TestScanCache -v`
Expected: FAIL - "undefined: NewScanCache"

**Step 3: Write the ScanCache implementation**

Create `pkg/worker/scan_cache.go`:

```go
package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/shishobooks/shisho/pkg/models"
)

// PersonFinder is the interface for finding or creating persons.
// This matches the signature of people.Service.FindOrCreatePerson.
type PersonFinder interface {
	FindOrCreatePerson(ctx context.Context, name string, libraryID int) (*models.Person, error)
}

// GenreFinder is the interface for finding or creating genres.
type GenreFinder interface {
	FindOrCreateGenre(ctx context.Context, name string, libraryID int) (*models.Genre, error)
}

// TagFinder is the interface for finding or creating tags.
type TagFinder interface {
	FindOrCreateTag(ctx context.Context, name string, libraryID int) (*models.Tag, error)
}

// SeriesFinder is the interface for finding or creating series.
type SeriesFinder interface {
	FindOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string) (*models.Series, error)
}

// ScanCache provides thread-safe caching for entity lookups during parallel scans.
// It caches persons, genres, tags, and series by name+libraryID to avoid redundant
// database queries when the same entity appears in multiple books.
type ScanCache struct {
	persons sync.Map // key: "name|libraryID" -> *models.Person
	genres  sync.Map // key: "name|libraryID" -> *models.Genre
	tags    sync.Map // key: "name|libraryID" -> *models.Tag
	series  sync.Map // key: "name|libraryID" -> *models.Series

	// Mutexes for preventing duplicate concurrent DB calls for the same key.
	// While sync.Map handles concurrent access, we need these to ensure
	// only one goroutine calls FindOrCreate for a given key.
	personMu sync.Map // key -> *sync.Mutex
	genreMu  sync.Map
	tagMu    sync.Map
	seriesMu sync.Map
}

// NewScanCache creates a new cache for use during a library scan.
func NewScanCache() *ScanCache {
	return &ScanCache{}
}

// cacheKey generates a unique key for name+libraryID combinations.
func cacheKey(name string, libraryID int) string {
	return fmt.Sprintf("%s|%d", name, libraryID)
}

// getMutex returns a per-key mutex, creating one if needed.
func getMutex(mutexMap *sync.Map, key string) *sync.Mutex {
	mu, _ := mutexMap.LoadOrStore(key, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// GetOrCreatePerson returns a cached person or fetches from DB on cache miss.
// Thread-safe for concurrent access from multiple goroutines.
func (c *ScanCache) GetOrCreatePerson(ctx context.Context, name string, libraryID int, svc PersonFinder) (*models.Person, error) {
	key := cacheKey(name, libraryID)

	// Fast path: check cache first
	if val, ok := c.persons.Load(key); ok {
		return val.(*models.Person), nil
	}

	// Slow path: acquire per-key mutex to prevent duplicate DB calls
	mu := getMutex(&c.personMu, key)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if val, ok := c.persons.Load(key); ok {
		return val.(*models.Person), nil
	}

	// Fetch from DB
	person, err := svc.FindOrCreatePerson(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.persons.Store(key, person)
	return person, nil
}

// GetOrCreateGenre returns a cached genre or fetches from DB on cache miss.
func (c *ScanCache) GetOrCreateGenre(ctx context.Context, name string, libraryID int, svc GenreFinder) (*models.Genre, error) {
	key := cacheKey(name, libraryID)

	if val, ok := c.genres.Load(key); ok {
		return val.(*models.Genre), nil
	}

	mu := getMutex(&c.genreMu, key)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := c.genres.Load(key); ok {
		return val.(*models.Genre), nil
	}

	genre, err := svc.FindOrCreateGenre(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.genres.Store(key, genre)
	return genre, nil
}

// GetOrCreateTag returns a cached tag or fetches from DB on cache miss.
func (c *ScanCache) GetOrCreateTag(ctx context.Context, name string, libraryID int, svc TagFinder) (*models.Tag, error) {
	key := cacheKey(name, libraryID)

	if val, ok := c.tags.Load(key); ok {
		return val.(*models.Tag), nil
	}

	mu := getMutex(&c.tagMu, key)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := c.tags.Load(key); ok {
		return val.(*models.Tag), nil
	}

	tag, err := svc.FindOrCreateTag(ctx, name, libraryID)
	if err != nil {
		return nil, err
	}

	c.tags.Store(key, tag)
	return tag, nil
}

// GetOrCreateSeries returns a cached series or fetches from DB on cache miss.
func (c *ScanCache) GetOrCreateSeries(ctx context.Context, name string, libraryID int, nameSource string, svc SeriesFinder) (*models.Series, error) {
	key := cacheKey(name, libraryID)

	if val, ok := c.series.Load(key); ok {
		return val.(*models.Series), nil
	}

	mu := getMutex(&c.seriesMu, key)
	mu.Lock()
	defer mu.Unlock()

	if val, ok := c.series.Load(key); ok {
		return val.(*models.Series), nil
	}

	series, err := svc.FindOrCreateSeries(ctx, name, libraryID, nameSource)
	if err != nil {
		return nil, err
	}

	c.series.Store(key, series)
	return series, nil
}

// PersonCount returns the number of cached persons.
func (c *ScanCache) PersonCount() int {
	count := 0
	c.persons.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// GenreCount returns the number of cached genres.
func (c *ScanCache) GenreCount() int {
	count := 0
	c.genres.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// TagCount returns the number of cached tags.
func (c *ScanCache) TagCount() int {
	count := 0
	c.tags.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// SeriesCount returns the number of cached series.
func (c *ScanCache) SeriesCount() int {
	count := 0
	c.series.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/worker -run TestScanCache -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/worker/scan_cache.go pkg/worker/scan_cache_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add ScanCache for thread-safe entity caching during parallel scans

Introduces a cache using sync.Map to avoid redundant FindOrCreate calls
for persons, genres, tags, and series during library scans. Uses per-key
mutexes to prevent duplicate concurrent DB queries for the same entity.
EOF
)"
```

---

## Task 3: Add bulk insert methods to books service

**Files:**
- Modify: `pkg/books/service.go`
- Create: `pkg/books/service_bulk_test.go`

**Step 1: Write the failing test**

Create `pkg/books/service_bulk_test.go`:

```go
package books_test

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_BulkCreateAuthors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Create library, book, and persons
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	person1 := testutils.CreatePerson(t, db, library.ID, "Author One")
	person2 := testutils.CreatePerson(t, db, library.ID, "Author Two")

	authors := []*models.Author{
		{BookID: book.ID, PersonID: person1.ID, SortOrder: 1},
		{BookID: book.ID, PersonID: person2.ID, SortOrder: 2},
	}

	err := svc.BulkCreateAuthors(ctx, authors)
	require.NoError(t, err)

	// Verify authors were created
	retrievedBook, err := svc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Len(t, retrievedBook.Authors, 2)
}

func TestService_BulkCreateNarrators(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Create library, book, file, and persons
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	file := testutils.CreateFile(t, db, library.ID, book.ID)
	person1 := testutils.CreatePerson(t, db, library.ID, "Narrator One")
	person2 := testutils.CreatePerson(t, db, library.ID, "Narrator Two")

	narrators := []*models.Narrator{
		{FileID: file.ID, PersonID: person1.ID, SortOrder: 1},
		{FileID: file.ID, PersonID: person2.ID, SortOrder: 2},
	}

	err := svc.BulkCreateNarrators(ctx, narrators)
	require.NoError(t, err)

	// Verify narrators were created
	files, err := svc.ListFiles(ctx, books.ListFilesOptions{BookID: &book.ID})
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Len(t, files[0].Narrators, 2)
}

func TestService_BulkCreateBookGenres(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Create library, book, and genres
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	genre1 := testutils.CreateGenre(t, db, library.ID, "Fiction")
	genre2 := testutils.CreateGenre(t, db, library.ID, "Sci-Fi")

	bookGenres := []*models.BookGenre{
		{BookID: book.ID, GenreID: genre1.ID},
		{BookID: book.ID, GenreID: genre2.ID},
	}

	err := svc.BulkCreateBookGenres(ctx, bookGenres)
	require.NoError(t, err)

	// Verify genres were created
	retrievedBook, err := svc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Len(t, retrievedBook.BookGenres, 2)
}

func TestService_BulkCreateBookTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Create library, book, and tags
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	tag1 := testutils.CreateTag(t, db, library.ID, "Favorite")
	tag2 := testutils.CreateTag(t, db, library.ID, "To Read")

	bookTags := []*models.BookTag{
		{BookID: book.ID, TagID: tag1.ID},
		{BookID: book.ID, TagID: tag2.ID},
	}

	err := svc.BulkCreateBookTags(ctx, bookTags)
	require.NoError(t, err)

	// Verify tags were created
	retrievedBook, err := svc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Len(t, retrievedBook.BookTags, 2)
}

func TestService_BulkCreateBookSeries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Create library, book, and series
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	series1 := testutils.CreateSeries(t, db, library.ID, "Series One")
	seriesNum := 1.0

	bookSeries := []*models.BookSeries{
		{BookID: book.ID, SeriesID: series1.ID, SeriesNumber: &seriesNum, SortOrder: 1},
	}

	err := svc.BulkCreateBookSeries(ctx, bookSeries)
	require.NoError(t, err)

	// Verify series were created
	retrievedBook, err := svc.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Len(t, retrievedBook.BookSeries, 1)
}

func TestService_BulkCreate_EmptySlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	svc := books.NewService(db)

	// Empty slices should not error
	err := svc.BulkCreateAuthors(ctx, nil)
	assert.NoError(t, err)

	err = svc.BulkCreateAuthors(ctx, []*models.Author{})
	assert.NoError(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/books -run TestService_BulkCreate -v`
Expected: FAIL - "svc.BulkCreateAuthors undefined"

**Step 3: Write the bulk insert methods**

Add to `pkg/books/service.go` (at the end of the file, before the closing):

```go
// BulkCreateAuthors creates multiple book-author associations in a single query.
// Returns nil if the slice is empty.
func (svc *Service) BulkCreateAuthors(ctx context.Context, authors []*models.Author) error {
	if len(authors) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&authors).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateNarrators creates multiple file-narrator associations in a single query.
func (svc *Service) BulkCreateNarrators(ctx context.Context, narrators []*models.Narrator) error {
	if len(narrators) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&narrators).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookGenres creates multiple book-genre associations in a single query.
func (svc *Service) BulkCreateBookGenres(ctx context.Context, bookGenres []*models.BookGenre) error {
	if len(bookGenres) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookGenres).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookTags creates multiple book-tag associations in a single query.
func (svc *Service) BulkCreateBookTags(ctx context.Context, bookTags []*models.BookTag) error {
	if len(bookTags) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookTags).Exec(ctx)
	return errors.WithStack(err)
}

// BulkCreateBookSeries creates multiple book-series associations in a single query.
func (svc *Service) BulkCreateBookSeries(ctx context.Context, bookSeries []*models.BookSeries) error {
	if len(bookSeries) == 0 {
		return nil
	}
	_, err := svc.db.NewInsert().Model(&bookSeries).Exec(ctx)
	return errors.WithStack(err)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/books -run TestService_BulkCreate -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_bulk_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add bulk insert methods for book relationships

Adds BulkCreateAuthors, BulkCreateNarrators, BulkCreateBookGenres,
BulkCreateBookTags, and BulkCreateBookSeries methods that insert
multiple records in a single query for better scan performance.
EOF
)"
```

---

## Task 4: Add updateBookRelationships transaction helper

**Files:**
- Modify: `pkg/worker/scan_unified.go`
- Create: `pkg/worker/scan_relationships_test.go`

**Step 1: Write the failing test**

Create `pkg/worker/scan_relationships_test.go`:

```go
package worker

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationshipUpdates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testutils.NewTestDB(t)
	w := testutils.NewTestWorker(t, db)

	// Create library, book, and persons
	library := testutils.CreateLibrary(t, db)
	book := testutils.CreateBook(t, db, library.ID)
	person1 := testutils.CreatePerson(t, db, library.ID, "Author One")
	person2 := testutils.CreatePerson(t, db, library.ID, "Author Two")
	genre := testutils.CreateGenre(t, db, library.ID, "Fiction")

	updates := RelationshipUpdates{
		Authors: []*models.Author{
			{BookID: book.ID, PersonID: person1.ID, SortOrder: 1},
			{BookID: book.ID, PersonID: person2.ID, SortOrder: 2},
		},
		BookGenres: []*models.BookGenre{
			{BookID: book.ID, GenreID: genre.ID},
		},
		DeleteAuthors: true,
		DeleteGenres:  true,
	}

	err := w.UpdateBookRelationships(ctx, book.ID, updates)
	require.NoError(t, err)

	// Verify authors were created
	retrievedBook, err := w.BookService().RetrieveBook(ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	assert.Len(t, retrievedBook.Authors, 2)
	assert.Len(t, retrievedBook.BookGenres, 1)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/worker -run TestRelationshipUpdates -v`
Expected: FAIL - "undefined: RelationshipUpdates"

**Step 3: Write the RelationshipUpdates struct and method**

Add to `pkg/worker/scan_unified.go` (near the top, after the imports and before ScanOptions):

```go
// RelationshipUpdates holds all relationship data to be updated for a book in a single transaction.
// This enables bulk inserts and atomic updates for better scan performance.
type RelationshipUpdates struct {
	Authors    []*models.Author
	Narrators  []*models.Narrator
	BookGenres []*models.BookGenre
	BookTags   []*models.BookTag
	BookSeries []*models.BookSeries

	// Delete flags indicate which relationships should be cleared before inserting new ones
	DeleteAuthors   bool
	DeleteNarrators bool
	DeleteGenres    bool
	DeleteTags      bool
	DeleteSeries    bool

	// FileID is required when DeleteNarrators is true (narrators are per-file)
	FileID int
}

// UpdateBookRelationships performs all relationship updates in a single transaction.
// It first deletes existing relationships (if flagged) then bulk inserts new ones.
func (w *Worker) UpdateBookRelationships(ctx context.Context, bookID int, updates RelationshipUpdates) error {
	return w.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Delete existing relationships if flagged
		if updates.DeleteAuthors {
			if _, err := tx.NewDelete().Model((*models.Author)(nil)).Where("book_id = ?", bookID).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to delete authors")
			}
		}
		if updates.DeleteNarrators && updates.FileID != 0 {
			if _, err := tx.NewDelete().Model((*models.Narrator)(nil)).Where("file_id = ?", updates.FileID).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to delete narrators")
			}
		}
		if updates.DeleteGenres {
			if _, err := tx.NewDelete().Model((*models.BookGenre)(nil)).Where("book_id = ?", bookID).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to delete genres")
			}
		}
		if updates.DeleteTags {
			if _, err := tx.NewDelete().Model((*models.BookTag)(nil)).Where("book_id = ?", bookID).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to delete tags")
			}
		}
		if updates.DeleteSeries {
			if _, err := tx.NewDelete().Model((*models.BookSeries)(nil)).Where("book_id = ?", bookID).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to delete series")
			}
		}

		// Bulk insert new relationships
		if len(updates.Authors) > 0 {
			if _, err := tx.NewInsert().Model(&updates.Authors).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to insert authors")
			}
		}
		if len(updates.Narrators) > 0 {
			if _, err := tx.NewInsert().Model(&updates.Narrators).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to insert narrators")
			}
		}
		if len(updates.BookGenres) > 0 {
			if _, err := tx.NewInsert().Model(&updates.BookGenres).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to insert genres")
			}
		}
		if len(updates.BookTags) > 0 {
			if _, err := tx.NewInsert().Model(&updates.BookTags).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to insert tags")
			}
		}
		if len(updates.BookSeries) > 0 {
			if _, err := tx.NewInsert().Model(&updates.BookSeries).Exec(ctx); err != nil {
				return errors.Wrap(err, "failed to insert series")
			}
		}

		return nil
	})
}
```

Also need to add `bun.Tx` import if not present:
```go
import (
	// ... existing imports ...
	"github.com/uptrace/bun"
)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/worker -run TestRelationshipUpdates -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan_relationships_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add UpdateBookRelationships for atomic batch updates

Single transaction helper that deletes old relationships and bulk inserts
new ones atomically. This replaces multiple individual DB calls with a
single transaction containing bulk operations.
EOF
)"
```

---

## Task 5: Update scanInternal to accept optional cache parameter

**Files:**
- Modify: `pkg/worker/scan_unified.go`

This task modifies `scanInternal` and `scanFileCore` to accept an optional `*ScanCache` parameter. When the cache is provided, entity lookups (persons, genres, tags, series) use the cache instead of direct service calls.

**Step 1: Update scanInternal signature**

In `pkg/worker/scan_unified.go`, update the `scanInternal` function signature:

```go
// scanInternal is the unified entry point for all scan operations using internal types.
// When cache is non-nil, entity lookups use the cache (parallel scan mode).
// When cache is nil, direct service calls are used (single-file operations).
func (w *Worker) scanInternal(ctx context.Context, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
```

**Step 2: Update all callers of scanInternal to pass nil**

Find all calls to `scanInternal` and add `nil` as the third argument. There are several call sites:
- `scanFileByPath` -> `scanFileByID` (indirect)
- `scanFileByID` -> `scanFileCore`
- `scanBook` -> `scanFileByID`
- `ProcessScanJob` (in scan.go)

Update each call to pass `nil` for the cache parameter.

**Step 3: Update scanFileCore to use cache when provided**

This is a large change. The key modifications in `scanFileCore`:

```go
func (w *Worker) scanFileCore(
	ctx context.Context,
	file *models.File,
	book *models.Book,
	metadata *mediafile.ParsedMetadata,
	forceRefresh bool,
	isResync bool,
	jobLog *joblogs.JobLogger,
	cache *ScanCache, // Add this parameter
) (*ScanResult, error) {
```

Then within the function, replace direct service calls with cache-aware calls:

```go
// Example: author creation
var person *models.Person
var err error
if cache != nil {
	person, err = cache.GetOrCreatePerson(ctx, parsedAuthor.Name, book.LibraryID, w.personService)
} else {
	person, err = w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name, book.LibraryID)
}
```

Apply the same pattern for:
- `w.genreService.FindOrCreateGenre`
- `w.tagService.FindOrCreateTag`
- `w.seriesService.FindOrCreateSeries`

**Step 4: Verify compilation and existing tests pass**

Run: `go build ./... && go test ./pkg/worker/... -v`
Expected: All existing tests pass

**Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Backend] Add optional cache parameter to scanInternal

When cache is provided, entity lookups use the thread-safe cache.
When nil, direct service calls are used (backwards compatible).
This prepares for parallel scan mode where multiple goroutines
share a cache to avoid redundant DB queries.
EOF
)"
```

---

## Task 6: Implement worker pool in ProcessScanJob

**Files:**
- Modify: `pkg/worker/scan.go`

**Step 1: Define scanResult type**

Add near the top of `scan.go`:

```go
// scanResult holds the result of a single file scan for the worker pool.
type scanResult struct {
	BookID int
	Path   string
	Err    error
}
```

**Step 2: Replace sequential scan loop with worker pool**

In `ProcessScanJob`, replace the sequential scan loop (lines ~327-340):

```go
// OLD (sequential):
// for _, path := range filesToScan {
//     result, err := w.scanInternal(ctx, ScanOptions{...}, nil)
//     ...
// }

// NEW (parallel):
workerCount := max(runtime.NumCPU(), 4)
jobLog.Info("starting parallel scan", logger.Data{
	"worker_count":  workerCount,
	"files_to_scan": len(filesToScan),
})

cache := NewScanCache()
fileChan := make(chan string, len(filesToScan))
resultChan := make(chan scanResult, len(filesToScan))

// Start workers
var wg sync.WaitGroup
for i := 0; i < workerCount; i++ {
	wg.Add(1)
	go func(workerID int) {
		defer wg.Done()
		for path := range fileChan {
			result, err := w.scanInternal(ctx, ScanOptions{
				FilePath:  path,
				LibraryID: library.ID,
				JobLog:    jobLog,
			}, cache)

			sr := scanResult{Path: path}
			if err != nil {
				sr.Err = err
			} else if result != nil && result.Book != nil {
				sr.BookID = result.Book.ID
			}
			resultChan <- sr
		}
	}(i)
}

// Dispatch files
for _, path := range filesToScan {
	fileChan <- path
}
close(fileChan)

// Collect results in background
go func() {
	wg.Wait()
	close(resultChan)
}()

// Process results
for result := range resultChan {
	if result.Err != nil {
		jobLog.Warn("failed to scan file", logger.Data{"path": result.Path, "error": result.Err.Error()})
		continue
	}
	if result.BookID != 0 {
		booksToOrganize[result.BookID] = struct{}{}
	}
}

jobLog.Info("parallel scan complete", logger.Data{
	"persons_cached": cache.PersonCount(),
	"genres_cached":  cache.GenreCount(),
	"tags_cached":    cache.TagCount(),
	"series_cached":  cache.SeriesCount(),
})
```

**Step 3: Add required imports**

```go
import (
	"runtime"
	"sync"
	// ... existing imports
)
```

**Step 4: Add max helper function**

Go 1.21+ has `max()` built-in. If using older Go, add:

```go
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

**Step 5: Run tests**

Run: `go test ./pkg/worker/... -v`
Expected: All tests pass

**Step 6: Commit**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Backend] Implement parallel file processing with worker pool

ProcessScanJob now uses a worker pool (max(NumCPU, 4) workers) to
process files in parallel. Workers share a ScanCache for entity
lookups, avoiding redundant DB queries for repeated authors/genres.

This significantly reduces scan time for large libraries.
EOF
)"
```

---

## Task 7: Integration testing and manual verification

**Files:**
- None (manual testing)

**Step 1: Run full test suite**

Run: `make check`
Expected: All tests pass, no linting errors

**Step 2: Manual verification**

1. Start the dev server: `make start`
2. Create or use a library with 50+ books
3. Trigger a library scan
4. Observe:
   - Scan completes faster than before
   - Job logs show worker count and cache stats
   - All books are correctly scanned with proper metadata

**Step 3: Verify logging**

Check that job logs include:
- `"starting parallel scan"` with worker_count and files_to_scan
- `"parallel scan complete"` with cache stats
- Individual file warnings if any

**Step 4: Final commit (if any fixes needed)**

If fixes are needed during testing, commit them with appropriate messages.

---

## Summary

The implementation consists of 7 tasks:

1. **Add db field to Worker** - Structural preparation
2. **Create ScanCache** - Thread-safe caching for parallel access
3. **Add bulk insert methods** - Batch database operations
4. **Add UpdateBookRelationships** - Transaction wrapper for atomic updates
5. **Update scanInternal for caching** - Cache-aware entity lookups
6. **Implement worker pool** - Parallel file processing
7. **Testing** - Verify everything works together

The changes are backward compatible - single-file operations continue to work by passing `nil` for the cache parameter.
