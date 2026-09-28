package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// These tests cover the worker and plugin mutation paths that used to leave
// an FTS row stale (#577).

// searchBookIDs returns the IDs of the Books in libraryID that a book search
// for query matches.
func (tc *testContext) searchBookIDs(libraryID int, query string) []int {
	tc.t.Helper()
	results, _, err := tc.searchService.SearchBooks(context.Background(), libraryID, query, nil, 10, 0)
	require.NoError(tc.t, err)
	ids := []int{}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// A move detected through sha256 rewrites files.filepath (and books.filepath
// when the directory changes), so books_fts must drop the old path and pick
// up the new one.
func TestMonitor_FileMoveReindexesBook(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Moveable Feast")
	oldPath := testgen.GenerateEPUB(t, bookDir, "zzoriginal.epub", testgen.EPUBOptions{
		Title:   "Moveable Feast",
		Authors: []string{"Author"},
	})

	m, libID := newTestMonitorWithWorker(tc, libDir)
	injectMonitorEvent(m, oldPath, fsnotify.Create, libID, false)
	m.processPendingEvents()
	files := tc.listFiles()
	require.Len(t, files, 1)
	bookID := files[0].BookID
	require.Equal(t, []int{bookID}, tc.searchBookIDs(libID, "zzoriginal"), "precondition: the old filename matches the Book")

	hash, err := computeFileSHA256(oldPath)
	require.NoError(t, err)
	require.NoError(t, tc.fingerprintService.Insert(tc.ctx, files[0].ID, models.FingerprintAlgorithmSHA256, hash))
	newPath := filepath.Join(bookDir, "zzmoved.epub")
	require.NoError(t, os.Rename(oldPath, newPath))

	injectMonitorEvent(m, oldPath, fsnotify.Remove, libID, false)
	injectMonitorEvent(m, newPath, fsnotify.Create, libID, false)
	m.processPendingEvents()

	require.Equal(t, newPath, tc.listFiles()[0].Filepath, "precondition: the move was detected")
	assert.Equal(t, []int{bookID}, tc.searchBookIDs(libID, "zzmoved"), "the new filename matches the Book")
	assert.Empty(t, tc.searchBookIDs(libID, "zzoriginal"), "the old filename no longer matches the Book")
}

// A File that disappeared from disk is deleted on resync while its Book
// survives through another File. The Book's books_fts row must drop the
// deleted File's path.
func TestResyncMissingFile_ReindexesSurvivingBook(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Twin Book")
	testgen.GenerateEPUB(t, bookDir, "first.epub", testgen.EPUBOptions{Title: "Twin Book", Authors: []string{"Author"}})
	ghostPath := testgen.GenerateEPUB(t, bookDir, "zzghost.epub", testgen.EPUBOptions{Title: "Twin Book", Authors: []string{"Author"}})
	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "precondition: both Files belong to one Book")
	libID := allBooks[0].LibraryID
	require.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(libID, "zzghost"), "precondition: the second File's path matches the Book")

	var ghost *models.File
	for _, f := range tc.listFiles() {
		if f.Filepath == ghostPath {
			ghost = f
		}
	}
	require.NotNil(t, ghost)
	require.NoError(t, os.Remove(ghostPath))

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: ghost.ID}, nil)
	require.NoError(t, err)
	require.True(t, result.FileDeleted)
	require.False(t, result.BookDeleted, "precondition: the Book survives through its other File")

	assert.Empty(t, tc.searchBookIDs(libID, "zzghost"), "the deleted File's path no longer matches the Book")
	assert.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(libID, "Twin"), "the surviving Book stays in the index")
}

// A File that disappeared from disk is deleted on resync, and with it its
// Book when no main File remains. The Series that held the Book must drop its
// title; CASCADE removes the book_series row, so the Series is only reachable
// through what was collected before the delete.
func TestResyncMissingLastFile_ReindexesSeriesOfDeletedBook(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	one, two := 1.0, 2.0
	testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libDir, "Doomed Dirigible"), "book.epub", testgen.EPUBOptions{
		Title: "Doomed Dirigible", Authors: []string{"Author"}, Series: "Tandem Saga", SeriesNumber: &one,
	})
	doomedPath := filepath.Join(libDir, "Doomed Dirigible", "book.epub")
	testgen.GenerateEPUB(t, testgen.CreateSubDir(t, libDir, "Steady Sextant"), "book.epub", testgen.EPUBOptions{
		Title: "Steady Sextant", Authors: []string{"Author"}, Series: "Tandem Saga", SeriesNumber: &two,
	})
	require.NoError(t, tc.runScan())
	allSeries := tc.listSeries()
	require.Len(t, allSeries, 1, "precondition: both Books share one Series")
	libID := allSeries[0].LibraryID
	require.Equal(t, []int{allSeries[0].ID}, tc.searchSeriesIDs(libID, "Dirigible"), "precondition: the Series lists the doomed Book")

	var doomed *models.File
	for _, f := range tc.listFiles() {
		if f.Filepath == doomedPath {
			doomed = f
		}
	}
	require.NotNil(t, doomed)
	require.NoError(t, os.Remove(doomedPath))

	result, err := tc.worker.scanInternal(tc.ctx, ScanOptions{FileID: doomed.ID}, nil)
	require.NoError(t, err)
	require.True(t, result.BookDeleted, "precondition: the Book goes with its last File")

	assert.Empty(t, tc.searchSeriesIDs(libID, "Dirigible"), "the deleted Book's title no longer matches the Series")
	assert.Equal(t, []int{allSeries[0].ID}, tc.searchSeriesIDs(libID, "Sextant"), "the surviving Book's title still matches the Series")
	assert.Empty(t, tc.searchBookIDs(libID, "Dirigible"), "the deleted Book leaves book search")
}

const ftsApplyEnricherManifest = `{
  "manifestVersion": 1,
  "id": "fts-enricher",
  "name": "FTS Enricher",
  "version": "1.0.0",
  "capabilities": {
    "metadataEnricher": {
      "fileTypes": ["epub"],
      "fields": ["title", "series", "seriesNumber"]
    }
  }
}`

// newFTSApplyFixture scans one EPUB into a library and installs an enricher
// the apply route can name. The enricher is not in the hook order, so the
// scan never runs it.
func newFTSApplyFixture(t *testing.T, organize bool) (*testContext, *models.Book, *models.File) {
	t.Helper()
	pluginDir := t.TempDir()
	tc := newTestContextWithPlugins(t, pluginDir)
	installTestPlugin(t, tc, pluginDir, "fts-enricher", ftsApplyEnricherManifest,
		`var plugin = {metadataEnricher: {search: function(ctx) {return {results: []};}}};`)
	require.NoError(t, tc.worker.pluginManager.LoadAll(tc.ctx))

	libDir := t.TempDir()
	tc.createLibraryWithOptions([]string{libDir}, organize)
	bookDir := testgen.CreateSubDir(t, libDir, "Alpha Draft")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Alpha Draft", Authors: []string{"Some Author"}})
	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &allBooks[0].ID})
	require.NoError(t, err)
	require.Len(t, book.Files, 1)
	return tc, book, book.Files[0]
}

func postFTSApply(t *testing.T, tc *testContext, payload plugins.PluginApplyPayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/plugins/apply", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newIdentifyApplyServer(t, tc).ServeHTTP(rec, req)
	return rec
}

// Applying a title in an organizing library moves the Book's directory after
// the metadata is persisted, so books_fts must be written after the move.
func TestPluginApply_TitleChangeInOrganizedLibraryIndexesFinalPath(t *testing.T) {
	t.Parallel()
	tc, book, file := newFTSApplyFixture(t, true)

	rec := postFTSApply(t, tc, plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields:      map[string]any{"title": "Zeta Rising"},
		PluginScope: "test", PluginID: "fts-enricher",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	updated, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.NotEqual(t, book.Filepath, updated.Filepath, "precondition: the apply moved the Book's directory")
	var indexed struct {
		Filepath  string `bun:"filepath"`
		Filenames string `bun:"filenames"`
	}
	require.NoError(t, tc.db.NewRaw("SELECT filepath, filenames FROM books_fts WHERE rowid = ?", book.ID).Scan(tc.ctx, &indexed))
	assert.Equal(t, updated.Filepath, indexed.Filepath, "books_fts.filepath follows the organized directory")
	assert.Equal(t, updated.Files[0].Filepath, indexed.Filenames, "books_fts.filenames follows the organized File path")
}

// An apply that fails after the title committed still re-indexes the Book.
func TestPluginApply_PartialFailureReindexesCommittedTitle(t *testing.T) {
	t.Parallel()
	tc, book, file := newFTSApplyFixture(t, false)

	rec := postFTSApply(t, tc, plugins.PluginApplyPayload{
		BookID: book.ID, FileID: &file.ID,
		Fields: map[string]any{
			"title":  "Zeta",
			"series": []map[string]any{{"name": "Dup Series", "number": 1}, {"name": "Dup Series", "number": 2}},
		},
		PluginScope: "test", PluginID: "fts-enricher",
	})
	require.GreaterOrEqual(t, rec.Code, 400, "the duplicate series entries are rejected: %s", rec.Body.String())

	updated, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.Equal(t, "Zeta", updated.Title, "precondition: the title committed before the failure")
	assert.Equal(t, []int{book.ID}, tc.searchBookIDs(book.LibraryID, "Zeta"), "the committed title matches the Book")
}

// cancelAfterBookCommit cancels a context once the transaction that inserted
// a Book commits, so a scan stops right after creating its first Book.
type cancelAfterBookCommit struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	sawInsert  bool
	cancelled  bool
	cancelOnce sync.Once
}

func (h *cancelAfterBookCommit) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *cancelAfterBookCommit) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	query := strings.TrimSpace(event.Query)
	if strings.HasPrefix(query, `INSERT INTO "books"`) {
		h.sawInsert = true
		return
	}
	if h.sawInsert && query == "COMMIT" && event.Err == nil {
		h.cancelOnce.Do(func() {
			h.cancelled = true
			h.cancel()
		})
	}
}

// A scan cancelled by shutdown never reaches the rebuild at its end, and in
// FilePath mode nothing indexed the Books it had already created. It must not
// rebuild on the way out either, because that would race the shutdown
// deadline. The next Start sees the scan job did not complete and rebuilds.
func TestProcessScanJob_CancelledScanIsIndexedAtNextStart(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Cancelled Cartography")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Cancelled Cartography", Authors: []string{"Author"}})

	ctx, cancel := context.WithCancel(tc.ctx)
	defer cancel()
	hook := &cancelAfterBookCommit{cancel: cancel}
	tc.db.AddQueryHook(hook)
	job := &models.Job{Type: models.JobTypeScan, Status: models.JobStatusInProgress, DataParsed: &models.JobScanData{}}
	require.NoError(t, tc.jobService.CreateJob(tc.ctx, job))

	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, job.ID, logger.FromContext(tc.ctx))
	err := tc.worker.ProcessScanJob(ctx, job, jobLog)
	require.ErrorIs(t, err, context.Canceled, "precondition: the scan stopped early")
	hook.mu.Lock()
	require.True(t, hook.cancelled, "precondition: the scan created a Book before stopping")
	hook.mu.Unlock()

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "precondition: the Book survives the cancelled scan")
	libID := allBooks[0].LibraryID
	require.Empty(t, tc.searchBookIDs(libID, "Cartography"), "precondition: a cancelled scan does not rebuild on the way out")

	// The job stays in progress (or is marked failed); either way the next
	// Start rebuilds before fetching jobs.
	tc.worker.rebuildSearchAfterIncompleteScan(tc.ctx)

	assert.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(libID, "Cartography"), "the created Book is searchable after the restart")
}

// The startup rebuild runs only when the latest scan did not complete.
func TestRebuildSearchAfterIncompleteScan_SkipsCompletedScan(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	tc.createLibrary([]string{t.TempDir()})
	libs, err := tc.libraryService.ListLibraries(tc.ctx, libraries.ListLibrariesOptions{})
	require.NoError(t, err)
	book := &models.Book{
		LibraryID: libs[0].ID, Filepath: "/library/Unindexed", Title: "Unindexed Umbra", TitleSource: models.DataSourceFilepath,
		SortTitle: "Unindexed Umbra", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath,
	}
	_, err = tc.db.NewInsert().Model(book).Exec(tc.ctx)
	require.NoError(t, err)

	tc.worker.rebuildSearchAfterIncompleteScan(tc.ctx)
	assert.Empty(t, tc.searchBookIDs(book.LibraryID, "Umbra"), "no scan has run, so there is nothing to recover")

	job := &models.Job{Type: models.JobTypeScan, Status: models.JobStatusCompleted, DataParsed: &models.JobScanData{}}
	require.NoError(t, tc.jobService.CreateJob(tc.ctx, job))
	tc.worker.rebuildSearchAfterIncompleteScan(tc.ctx)
	assert.Empty(t, tc.searchBookIDs(book.LibraryID, "Umbra"), "the last scan completed and rebuilt, so startup does not")

	failed := &models.Job{Type: models.JobTypeScan, Status: models.JobStatusFailed, DataParsed: &models.JobScanData{}, CreatedAt: job.CreatedAt.Add(time.Second)}
	require.NoError(t, tc.jobService.CreateJob(tc.ctx, failed))
	tc.worker.rebuildSearchAfterIncompleteScan(tc.ctx)
	assert.Equal(t, []int{book.ID}, tc.searchBookIDs(book.LibraryID, "Umbra"), "the last scan failed, so startup rebuilds")
}

// A scan that fails for a reason other than shutdown rebuilds on the way out,
// so the Books it created in the libraries it finished are searchable.
func TestProcessScanJob_FailedScanIndexesCreatedBooks(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Failed Firmament")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Failed Firmament", Authors: []string{"Author"}})
	// A second library whose path is missing makes the walk fail after the
	// first library's Book was created.
	tc.createLibrary([]string{filepath.Join(t.TempDir(), "missing")})

	require.Error(t, tc.runScan(), "precondition: the scan fails on the missing library path")

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1, "precondition: the first library's Book was created")
	assert.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(allBooks[0].LibraryID, "Firmament"), "the created Book is searchable")
}

// searchSeriesIDs returns the IDs of the Series in libraryID that a series
// search for query matches.
func (tc *testContext) searchSeriesIDs(libraryID int, query string) []int {
	tc.t.Helper()
	results, _, err := tc.searchService.SearchSeries(context.Background(), libraryID, query, 10, 0)
	require.NoError(tc.t, err)
	ids := []int{}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// The apply route reindexes the Series a Book joins, the ones it leaves, and
// the ones it stays in when its authors change.
func TestPluginApply_ReindexesJoinedLeftAndKeptSeries(t *testing.T) {
	t.Parallel()
	tc, book, file := newFTSApplyFixture(t, false)
	apply := func(fields map[string]any) {
		t.Helper()
		rec := postFTSApply(t, tc, plugins.PluginApplyPayload{
			BookID: book.ID, FileID: &file.ID, Fields: fields,
			PluginScope: "test", PluginID: "fts-enricher",
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}
	seriesID := func(name string) int {
		t.Helper()
		var id int
		require.NoError(t, tc.db.NewSelect().Model((*models.Series)(nil)).Column("id").Where("name = ?", name).Scan(tc.ctx, &id))
		return id
	}

	apply(map[string]any{"series": []map[string]any{{"name": "Lanternfall Cycle", "number": 1}}})
	first := seriesID("Lanternfall Cycle")
	assert.Equal(t, []int{first}, tc.searchSeriesIDs(book.LibraryID, "Lanternfall"), "the Series the apply created is searchable")
	assert.Equal(t, []int{first}, tc.searchSeriesIDs(book.LibraryID, "Alpha"), "the joined Series lists the Book's title")

	apply(map[string]any{"series": []map[string]any{{"name": "Moonwake Saga", "number": 1}}})
	second := seriesID("Moonwake Saga")
	assert.Equal(t, []int{second}, tc.searchSeriesIDs(book.LibraryID, "Alpha"), "only the joined Series lists the Book's title, not the one it left")

	apply(map[string]any{"authors": []map[string]any{{"name": "Barnaby Thistlewood"}}})
	assert.Equal(t, []int{second}, tc.searchSeriesIDs(book.LibraryID, "Thistlewood"), "the kept Series lists the new author")
	assert.Equal(t, []int{book.ID}, tc.searchBookIDs(book.LibraryID, "Moonwake"), "the Book lists its Series")
}

// ftsRowDeleteRecorder records per-row deletes from books_fts, which only
// the per-entity reindex issues; RebuildAllIndexes clears the table whole.
type ftsRowDeleteRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *ftsRowDeleteRecorder) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (r *ftsRowDeleteRecorder) AfterQuery(_ context.Context, event *bun.QueryEvent) {
	if strings.HasPrefix(strings.TrimSpace(event.Query), "DELETE FROM books_fts WHERE") {
		r.mu.Lock()
		r.queries = append(r.queries, event.Query)
		r.mu.Unlock()
	}
}

// A full scan hands a changed file to scanFileByID, which reindexes per file
// on a single-file resync. Inside a full scan that work is redundant with the
// closing RebuildAllIndexes, so it is skipped.
func TestProcessScanJob_ChangedFileSkipsPerFileReindex(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t)
	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	bookDir := testgen.CreateSubDir(t, libDir, "Shifting Shoreline")
	path := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Shifting Shoreline", Authors: []string{"Author"}})
	require.NoError(t, tc.runScan())

	// Rewrite the file with different content and a later mtime so the
	// next scan treats it as changed.
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Shifting Shoreline", Authors: []string{"Author"}, Description: "A longer description that changes the file size."})
	later := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, later, later))

	recorder := &ftsRowDeleteRecorder{}
	tc.db.AddQueryHook(recorder)
	require.NoError(t, tc.runScan())

	recorder.mu.Lock()
	assert.Empty(t, recorder.queries, "a full scan does not reindex a changed file on its own")
	recorder.mu.Unlock()
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	assert.Equal(t, []int{allBooks[0].ID}, tc.searchBookIDs(allBooks[0].LibraryID, "Shoreline"), "the closing rebuild indexes the Book")
}
