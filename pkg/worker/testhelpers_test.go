package worker

import (
	"context"
	"database/sql"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/genres"
	"github.com/shishobooks/shisho/pkg/imprints"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/people"
	"github.com/shishobooks/shisho/pkg/publishers"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/series"
	"github.com/shishobooks/shisho/pkg/tags"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// testContext holds all the dependencies needed for testing the worker.
type testContext struct {
	t              *testing.T
	ctx            context.Context
	db             *bun.DB
	worker         *Worker
	bookService    *books.Service
	libraryService *libraries.Service
	jobService     *jobs.Service
	jobLogService  *joblogs.Service
	personService  *people.Service
	seriesService  *series.Service
	searchService  *search.Service // may be nil if not initialized
}

// newTestContext creates a new test context with an in-memory SQLite database
// and all necessary services initialized.
func newTestContext(t *testing.T) *testContext {
	t.Helper()

	// Create in-memory SQLite database
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Run migrations
	_, err = migrations.BringUpToDate(context.Background(), db)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create services
	bookService := books.NewService(db)
	libraryService := libraries.NewService(db)
	jobService := jobs.NewService(db)
	jobLogService := joblogs.NewService(db)
	personService := people.NewService(db)
	seriesService := series.NewService(db)
	genreService := genres.NewService(db)
	tagService := tags.NewService(db)
	publisherService := publishers.NewService(db)
	imprintService := imprints.NewService(db)

	// Create worker
	cfg := &config.Config{
		WorkerProcesses:           1,
		SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
	}
	w := &Worker{
		config:           cfg,
		log:              logger.New(),
		bookService:      bookService,
		libraryService:   libraryService,
		jobService:       jobService,
		jobLogService:    jobLogService,
		personService:    personService,
		seriesService:    seriesService,
		genreService:     genreService,
		tagService:       tagService,
		publisherService: publisherService,
		imprintService:   imprintService,
	}

	// Create context with logger
	ctx := logger.New().WithContext(context.Background())

	tc := &testContext{
		t:              t,
		ctx:            ctx,
		db:             db,
		worker:         w,
		bookService:    bookService,
		libraryService: libraryService,
		jobService:     jobService,
		jobLogService:  jobLogService,
		personService:  personService,
		seriesService:  seriesService,
	}

	t.Cleanup(func() {
		db.Close()
	})

	return tc
}

// createLibrary creates a test library with the given paths.
func (tc *testContext) createLibrary(paths []string) {
	tc.t.Helper()

	libraryPaths := make([]*models.LibraryPath, len(paths))
	for i, p := range paths {
		libraryPaths[i] = &models.LibraryPath{
			Filepath: p,
		}
	}

	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
		LibraryPaths:     libraryPaths,
	}

	err := tc.libraryService.CreateLibrary(tc.ctx, library)
	if err != nil {
		tc.t.Fatalf("failed to create library: %v", err)
	}
}

// createLibraryWithOptions creates a test library with custom options.
func (tc *testContext) createLibraryWithOptions(paths []string, organizeFileStructure bool) {
	tc.t.Helper()

	libraryPaths := make([]*models.LibraryPath, len(paths))
	for i, p := range paths {
		libraryPaths[i] = &models.LibraryPath{
			Filepath: p,
		}
	}

	library := &models.Library{
		Name:                  "Test Library",
		OrganizeFileStructure: organizeFileStructure,
		CoverAspectRatio:      "book",
		LibraryPaths:          libraryPaths,
	}

	err := tc.libraryService.CreateLibrary(tc.ctx, library)
	if err != nil {
		tc.t.Fatalf("failed to create library: %v", err)
	}
}

// listBooks returns all books in the database.
func (tc *testContext) listBooks() []*models.Book {
	tc.t.Helper()

	allBooks, err := tc.bookService.ListBooks(tc.ctx, books.ListBooksOptions{})
	if err != nil {
		tc.t.Fatalf("failed to list books: %v", err)
	}
	return allBooks
}

// listFiles returns all files in the database.
func (tc *testContext) listFiles() []*models.File {
	tc.t.Helper()

	files, err := tc.bookService.ListFiles(tc.ctx, books.ListFilesOptions{})
	if err != nil {
		tc.t.Fatalf("failed to list files: %v", err)
	}
	return files
}

// runScan executes the scan job for all libraries.
func (tc *testContext) runScan() error {
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	return tc.worker.ProcessScanJob(tc.ctx, nil, jobLog)
}

// listSeries returns all series in the database.
func (tc *testContext) listSeries() []*models.Series {
	tc.t.Helper()

	allSeries, err := tc.seriesService.ListSeries(tc.ctx, series.ListSeriesOptions{})
	if err != nil {
		tc.t.Fatalf("failed to list series: %v", err)
	}
	return allSeries
}

// newTestContextWithSearchService creates a test context with a real search service
// for testing search index functionality.
func newTestContextWithSearchService(t *testing.T) *testContext {
	t.Helper()

	// Create in-memory SQLite database
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Run migrations
	_, err = migrations.BringUpToDate(context.Background(), db)
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create services
	bookService := books.NewService(db)
	libraryService := libraries.NewService(db)
	jobService := jobs.NewService(db)
	jobLogService := joblogs.NewService(db)
	personService := people.NewService(db)
	seriesService := series.NewService(db)
	genreService := genres.NewService(db)
	tagService := tags.NewService(db)
	searchService := search.NewService(db)
	publisherService := publishers.NewService(db)
	imprintService := imprints.NewService(db)

	// Create worker with search service
	cfg := &config.Config{
		WorkerProcesses:           1,
		SupplementExcludePatterns: []string{".*", ".DS_Store", "Thumbs.db", "desktop.ini"},
	}
	w := &Worker{
		config:           cfg,
		log:              logger.New(),
		bookService:      bookService,
		libraryService:   libraryService,
		jobService:       jobService,
		jobLogService:    jobLogService,
		personService:    personService,
		seriesService:    seriesService,
		genreService:     genreService,
		tagService:       tagService,
		searchService:    searchService,
		publisherService: publisherService,
		imprintService:   imprintService,
	}

	// Create context with logger
	ctx := logger.New().WithContext(context.Background())

	tc := &testContext{
		t:              t,
		ctx:            ctx,
		db:             db,
		worker:         w,
		bookService:    bookService,
		libraryService: libraryService,
		jobService:     jobService,
		jobLogService:  jobLogService,
		personService:  personService,
		seriesService:  seriesService,
		searchService:  searchService,
	}

	t.Cleanup(func() {
		db.Close()
	})

	return tc
}
