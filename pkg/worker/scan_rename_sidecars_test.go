package worker

import (
	"os"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/aliases"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Renaming a resource rewrites the sidecars of the Books that hold it, so the
// next scan doesn't read the old name back. Without that, a scan of a library
// with organizing off re-created the old name for every Book and undid the
// rename.

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
	f.tc.bookService.WriteSidecarsForPerson(f.tc.ctx, personID)

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
	f.tc.bookService.WriteSidecarsForSeries(f.tc.ctx, seriesID)

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
	f.tc.bookService.WriteSidecarsForPublisher(f.tc.ctx, publisherID)

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
	f.tc.bookService.WriteSidecarsForGenre(ctx, genre.ID)
	f.rename(t, "tags", tag.ID, "New Tag")
	f.tc.bookService.WriteSidecarsForTag(ctx, tag.ID)

	s := f.bookSidecar(t)
	assert.Equal(t, []string{"New Genre"}, s.Genres)
	assert.Equal(t, []string{"New Tag"}, s.Tags)
}

// A Book whose sidecar can't be written is skipped; the rest are still
// written.
func TestWriteSidecars_UnwritableBookIsSkipped(t *testing.T) {
	t.Parallel()
	f := newRenameFixture(t)
	personID := f.book.Authors[0].PersonID
	f.rename(t, "persons", personID, "John Smith")
	require.NoError(t, os.RemoveAll(f.book.Filepath))

	assert.NotPanics(t, func() {
		f.tc.bookService.WriteSidecarsForPerson(f.tc.ctx, personID)
	})
}
