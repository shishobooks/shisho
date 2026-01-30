# Faster Library Scan Design

## Problem

Library scans with ~100 books take a noticeable amount of time. Analysis shows:

1. **Sequential file processing** - files processed one at a time
2. **Individual DB operations** - ~20+ round-trips per book (delete + insert for each author/narrator/genre/tag)
3. **Repeated lookups** - `FindOrCreatePerson` called for every author, even duplicates across books

With 100 books, this results in ~2,000+ database operations.

## Solution

Two optimizations working together:

1. **Parallel file processing** - worker pool with `max(runtime.NumCPU(), 4)` goroutines
2. **Batch database operations** - transaction batching for relationship updates + in-memory cache for lookups

## Architecture

### Current Flow (Sequential)
```
WalkDir → [file1, file2, ...fileN]
    ↓
for each file:
    scanInternal() → 20+ DB ops per book
```

### New Flow (Parallel + Batched)
```
WalkDir → [file1, file2, ...fileN]
    ↓
Dispatch to worker pool (max(NumCPU, 4) workers)
    ↓
Each worker:
    1. Parse file metadata
    2. FindOrCreate persons/genres/tags (using shared sync.Map cache)
    3. Build relationship structs in memory
    4. Single transaction: delete old + bulk insert new relationships
    5. Send result to channel
    ↓
Main goroutine:
    - Collects results from channel
    - Tracks booksToOrganize
    - Logs errors
```

## Components

### 1. ScanCache (`pkg/worker/scan_cache.go`)

Thread-safe cache using `sync.Map` for persons, genres, tags, and series lookups during a scan.

```go
type ScanCache struct {
    persons sync.Map  // key: "name|libraryID" → *models.Person
    genres  sync.Map  // key: "name|libraryID" → *models.Genre
    tags    sync.Map  // key: "name|libraryID" → *models.Tag
    series  sync.Map  // key: "name|libraryID" → *models.Series
}

func NewScanCache() *ScanCache

// Thread-safe lookup with DB fallback on cache miss
func (c *ScanCache) GetOrCreatePerson(ctx, name, libraryID, personService) (*models.Person, error)
func (c *ScanCache) GetOrCreateGenre(ctx, name, libraryID, genreService) (*models.Genre, error)
func (c *ScanCache) GetOrCreateTag(ctx, name, libraryID, tagService) (*models.Tag, error)
func (c *ScanCache) GetOrCreateSeries(ctx, name, libraryID, nameSource, seriesService) (*models.Series, error)

// Stats for logging
func (c *ScanCache) PersonCount() int
func (c *ScanCache) GenreCount() int
func (c *ScanCache) TagCount() int
func (c *ScanCache) SeriesCount() int
```

### 2. Bulk Service Methods (`pkg/books/service.go`)

New methods for inserting multiple records in a single query:

```go
func (svc *Service) BulkCreateAuthors(ctx context.Context, authors []*models.Author) error
func (svc *Service) BulkCreateNarrators(ctx context.Context, narrators []*models.Narrator) error
func (svc *Service) BulkCreateBookGenres(ctx context.Context, bookGenres []*models.BookGenre) error
func (svc *Service) BulkCreateBookTags(ctx context.Context, bookTags []*models.BookTag) error
func (svc *Service) BulkCreateBookSeries(ctx context.Context, bookSeries []*models.BookSeries) error
```

### 3. Transaction Wrapper (`pkg/worker/scan_unified.go`)

Single transaction for all relationship updates:

```go
type RelationshipUpdates struct {
    Authors    []*models.Author
    Narrators  []*models.Narrator
    BookGenres []*models.BookGenre
    BookTags   []*models.BookTag
    BookSeries []*models.BookSeries

    DeleteAuthors   bool
    DeleteNarrators bool
    DeleteGenres    bool
    DeleteTags      bool
    DeleteSeries    bool

    FileID int  // for narrator deletion scope
}

func (w *Worker) updateBookRelationships(ctx, bookID, updates) error {
    return w.db.RunInTx(ctx, nil, func(ctx, tx) error {
        // All deletes
        // All bulk inserts
        // Atomic commit
    })
}
```

### 4. Worker Pool (`pkg/worker/scan.go`)

```go
func (w *Worker) ProcessScanJob(ctx, job, jobLog) error {
    // ... existing discovery code ...

    workerCount := max(runtime.NumCPU(), 4)
    jobLog.Info("starting parallel scan", logger.Data{
        "worker_count":   workerCount,
        "files_to_scan":  len(filesToScan),
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
                result, err := w.scanInternal(ctx, ScanOptions{...}, cache)
                resultChan <- scanResult{BookID, Path, Error}
            }
        }(i)
    }

    // Dispatch files
    for _, path := range filesToScan {
        fileChan <- path
    }
    close(fileChan)

    // Collect results
    go func() { wg.Wait(); close(resultChan) }()

    for result := range resultChan {
        // Track booksToOrganize, log errors
    }

    jobLog.Info("parallel scan complete", logger.Data{...})
    jobLog.Info("scan cache stats", logger.Data{
        "persons_cached": cache.PersonCount(),
        "genres_cached":  cache.GenreCount(),
        "tags_cached":    cache.TagCount(),
        "series_cached":  cache.SeriesCount(),
    })
}

type scanResult struct {
    BookID int
    Path   string
    Error  error
}
```

### 5. Modified scanInternal (`pkg/worker/scan_unified.go`)

Updated signature accepts optional cache:

```go
func (w *Worker) scanInternal(ctx, opts ScanOptions, cache *ScanCache) (*ScanResult, error) {
    // cache == nil: direct DB calls (single-file operations)
    // cache != nil: use cache (parallel scan)

    // Build relationships in memory
    var authors []*models.Author
    for i, parsedAuthor := range metadata.Authors {
        var person *models.Person
        if cache != nil {
            person, err = cache.GetOrCreatePerson(ctx, name, libraryID, w.personService)
        } else {
            person, err = w.personService.FindOrCreatePerson(ctx, name, libraryID)
        }
        authors = append(authors, &models.Author{...})
    }

    // Single transaction for all updates
    updates := RelationshipUpdates{Authors: authors, ...}
    if err := w.updateBookRelationships(ctx, book.ID, updates); err != nil {
        return nil, err
    }
}
```

## Files to Modify

### New Files
| File | Purpose |
|------|---------|
| `pkg/worker/scan_cache.go` | ScanCache implementation |

### Modified Files
| File | Changes |
|------|---------|
| `pkg/worker/scan.go` | Worker pool in `ProcessScanJob`, result collection, logging |
| `pkg/worker/scan_unified.go` | Add cache parameter to `scanInternal`, `updateBookRelationships`, build relationships in memory |
| `pkg/books/service.go` | Add bulk create methods |

## Implementation Order

1. **Add ScanCache** (`scan_cache.go`) - independent, testable in isolation
2. **Add bulk service methods** (`service.go`) - independent, testable in isolation
3. **Add `updateBookRelationships`** (`scan_unified.go`) - uses bulk methods
4. **Update `scanInternal` signature** (`scan_unified.go`) - add cache parameter, refactor to use cache + batching
5. **Update all `scanInternal` callers** - pass `nil` for single-file operations
6. **Add worker pool to `ProcessScanJob`** (`scan.go`) - wire everything together
7. **Testing** - existing test suite + manual NAS test

## Logging

Key log points:
- Scan start: worker count, files to scan
- Scan complete: books scanned, errors
- Cache stats: persons/genres/tags/series cached
- Individual file errors (warn level)

## Expected Performance Improvement

| Metric | Before | After |
|--------|--------|-------|
| DB ops per book | ~20+ | ~4 (1 tx with bulk ops) |
| File processing | Sequential | Parallel (4+ workers) |
| Person/genre lookups | DB every time | Cached after first |

For 100 books with repeated authors/genres, expect 5-10x faster scans.
