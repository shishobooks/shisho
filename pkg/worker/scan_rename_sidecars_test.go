package worker

import (
	"os"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/jobs"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renaming a resource marks the sidecars of the Books that hold it stale and
// queues a job that rewrites them, so the next scan doesn't read the old name
// back. Without that, a scan of a library with organizing off re-created the
// old name for every Book and undid the rename. A scan that reaches a marked
// Book before the job rewrites its sidecars first.

type renameFixture struct {
	tc       *testContext
	book     *models.Book
	file     *models.File
	epubPath string
}

func newRenameFixture(t *testing.T) *renameFixture {
	t.Helper()
	tc := newTestContext(t)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Saga One")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:        "Saga One",
		Authors:      []string{"Jon Smith"},
		Series:       "Old Saga",
		SeriesNumber: pointerutil.Float64(1),
		Publisher:    "Old Press",
	})
	require.NoError(t, tc.runScan())
	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	book := allBooks[0]
	require.Len(t, book.Files, 1)
	file, err := tc.bookService.RetrieveFileWithRelations(tc.ctx, book.Files[0].ID)
	require.NoError(t, err)
	return &renameFixture{tc: tc, book: book, file: file, epubPath: epubPath}
}

func (f *renameFixture) rename(t *testing.T, table string, id int, name string) {
	t.Helper()
	_, err := f.tc.db.NewRaw("UPDATE "+table+" SET name = ? WHERE id = ?", name, id).Exec(f.tc.ctx)
	require.NoError(t, err)
}

func (f *renameFixture) bookSidecar(t *testing.T) *sidecar.BookSidecar {
	t.Helper()
	book := f.reload(t)
	s, err := sidecar.ReadBookSidecarFromModel(book, nil)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s
}

func (f *renameFixture) fileSidecar(t *testing.T) *sidecar.FileSidecar {
	t.Helper()
	s, err := sidecar.ReadFileSidecar(f.epubPath)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s
}

// rewriteStale runs the sidecar rewrite job's work.
func (f *renameFixture) rewriteStale(t *testing.T) {
	t.Helper()
	_, err := f.tc.bookService.RewriteStaleSidecars(f.tc.ctx, noWarn)
	require.NoError(t, err)
}

func (f *renameFixture) reload(t *testing.T) *models.Book {
	t.Helper()
	allBooks := f.tc.listBooks()
	require.Len(t, allBooks, 1)
	return allBooks[0]
}

func TestWriteSidecars_PersonRenameSurvivesScan(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	personID := f.book.Authors[0].PersonID

	// The rename dialog keeps the old name as an alias, which is how the
	// file's embedded "Jon Smith" still finds this person.
	f.rename(t, "persons", personID, "John Smith")
	require.NoError(t, f.tc.worker.aliasService.SyncAliases(f.tc.ctx, aliases.PersonConfig, personID, f.book.LibraryID, []string{"Jon Smith"}))
	f.tc.bookService.MarkSidecarsStaleForPerson(f.tc.ctx, personID)
	f.rewriteStale(t)

	s := f.bookSidecar(t)
	require.Len(t, s.Authors, 1)
	assert.Equal(t, "John Smith", s.Authors[0].Name)

	libraryID := f.book.LibraryID
	require.NoError(t, f.tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))
	book := f.reload(t)
	require.Len(t, book.Authors, 1)
	assert.Equal(t, personID, book.Authors[0].PersonID, "the scan must keep the renamed person")
	assert.NotEqual(t, models.DataSourceSidecar, book.AuthorSource)
}

func TestWriteSidecars_SeriesRename(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	require.Len(t, f.book.BookSeries, 1)
	seriesID := f.book.BookSeries[0].SeriesID

	f.rename(t, "series", seriesID, "New Saga")
	f.tc.bookService.MarkSidecarsStaleForSeries(f.tc.ctx, seriesID)
	f.rewriteStale(t)

	s := f.bookSidecar(t)
	require.Len(t, s.Series, 1)
	assert.Equal(t, "New Saga", s.Series[0].Name)
}

func TestWriteSidecars_PublisherRename(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	require.NotNil(t, f.file.PublisherID)
	publisherID := *f.file.PublisherID

	f.rename(t, "publishers", publisherID, "New Press")
	f.tc.bookService.MarkSidecarsStaleForPublisher(f.tc.ctx, publisherID)
	f.rewriteStale(t)

	s := f.fileSidecar(t)
	require.NotNil(t, s.Publisher)
	assert.Equal(t, "New Press", *s.Publisher)
}

func TestWriteSidecars_GenreAndTagRename(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	ctx := f.tc.ctx
	genre := &models.Genre{LibraryID: f.book.LibraryID, Name: "Old Genre"}
	_, err := f.tc.db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)
	tag := &models.Tag{LibraryID: f.book.LibraryID, Name: "Old Tag"}
	_, err = f.tc.db.NewInsert().Model(tag).Exec(ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewInsert().Model(&models.BookGenre{BookID: f.book.ID, GenreID: genre.ID}).Exec(ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewInsert().Model(&models.BookTag{BookID: f.book.ID, TagID: tag.ID}).Exec(ctx)
	require.NoError(t, err)

	f.rename(t, "genres", genre.ID, "New Genre")
	f.tc.bookService.MarkSidecarsStaleForGenre(ctx, genre.ID)
	f.rename(t, "tags", tag.ID, "New Tag")
	f.tc.bookService.MarkSidecarsStaleForTag(ctx, tag.ID)
	f.rewriteStale(t)

	s := f.bookSidecar(t)
	assert.Equal(t, []string{"New Genre"}, s.Genres)
	assert.Equal(t, []string{"New Tag"}, s.Tags)
}

// A scan that reaches a marked Book before the job rewrites its sidecars
// first, so it never reads the old name back.
func TestWriteSidecars_ScanBeforeJobRewritesFirst(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	personID := f.book.Authors[0].PersonID
	f.rename(t, "persons", personID, "John Smith")
	require.NoError(t, f.tc.worker.aliasService.SyncAliases(f.tc.ctx, aliases.PersonConfig, personID, f.book.LibraryID, []string{"Jon Smith"}))
	f.tc.bookService.MarkSidecarsStaleForPerson(f.tc.ctx, personID)
	require.NotNil(t, f.reload(t).SidecarsStaleAt)

	libraryID := f.book.LibraryID
	require.NoError(t, f.tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))

	book := f.reload(t)
	assert.Nil(t, book.SidecarsStaleAt, "the scan clears the mark once it has rewritten the sidecars")
	require.Len(t, book.Authors, 1)
	assert.Equal(t, personID, book.Authors[0].PersonID)
	assert.NotEqual(t, models.DataSourceSidecar, book.AuthorSource)
	s := f.bookSidecar(t)
	require.Len(t, s.Authors, 1)
	assert.Equal(t, "John Smith", s.Authors[0].Name)
}

// Marking queues one sidecar rewrite job, however many renames mark Books
// before it runs.
func TestWriteSidecars_QueuesOneJob(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	personID := f.book.Authors[0].PersonID

	f.tc.bookService.MarkSidecarsStaleForPerson(f.tc.ctx, personID)
	f.tc.bookService.MarkSidecarsStaleForPerson(f.tc.ctx, personID)

	jobType := models.JobTypeSidecarRewrite
	pending, err := f.tc.jobService.ListJobs(f.tc.ctx, jobs.ListJobsOptions{Type: &jobType})
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}

// A Book whose sidecars can't be written stays marked, the job still
// finishes the rest, and a scan skips that Book's stale sidecars.
func TestWriteSidecars_UnwritableBookStaysMarked(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	personID := f.book.Authors[0].PersonID
	f.rename(t, "persons", personID, "John Smith")
	f.tc.bookService.MarkSidecarsStaleForPerson(f.tc.ctx, personID)
	// The file's sidecar path is taken by a directory, so it can't be written.
	sidecarPath := sidecar.FileSidecarPath(f.epubPath)
	require.NoError(t, os.Remove(sidecarPath))
	require.NoError(t, os.MkdirAll(sidecarPath, 0o755))

	f.rewriteStale(t)
	assert.NotNil(t, f.reload(t).SidecarsStaleAt)

	// A scan can't rewrite them either, so it doesn't read the stale book
	// sidecar, which still names "Jon Smith", as an edit on disk.
	libraryID := f.book.LibraryID
	require.NoError(t, f.tc.runScanJob(&models.Job{
		Type:       models.JobTypeScan,
		LibraryID:  &libraryID,
		DataParsed: &models.JobScanData{ScanForNewMetadata: true},
	}))
	assert.NotEqual(t, models.DataSourceSidecar, f.reload(t).AuthorSource)
}

// At startup the worker queues the rewrite job when Books are still marked,
// as after a shutdown that cancelled it.
func TestEnsureSidecarRewrite_QueuesTheJobAtStartup(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	jobType := models.JobTypeSidecarRewrite
	queued := func() int {
		pending, err := f.tc.jobService.ListJobs(f.tc.ctx, jobs.ListJobsOptions{Type: &jobType})
		require.NoError(t, err)
		return len(pending)
	}

	f.tc.worker.ensureSidecarRewrite(f.tc.ctx)
	assert.Equal(t, 0, queued(), "nothing marked, nothing queued")

	_, err := f.tc.db.NewUpdate().Model((*models.Book)(nil)).Set("sidecars_stale_at = ?", time.Now()).Where("id = ?", f.book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	f.tc.worker.ensureSidecarRewrite(f.tc.ctx)
	assert.Equal(t, 1, queued())
}

func noWarn(string, logger.Data) {}
