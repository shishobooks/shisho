package worker

import (
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A book's oldest main file owns its book-level fields as far as embedded
// metadata goes. Without that rule every embedded format shares one
// priority, so a book with an EPUB and a PDF took each field from whichever
// file a scan reached last.

type bookOwnerFixture struct {
	tc      *testContext
	bookDir string
	book    *models.Book
	epub    *models.File // the owner: created first, so its ID is lowest
	pdf     *models.File
}

func newBookOwnerFixture(t *testing.T) *bookOwnerFixture {
	t.Helper()
	tc := newTestContext(t)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Open Advice")

	source := models.DataSourceEPUBMetadata
	subtitle := "From the EPUB"
	description := "From the EPUB."
	book := &models.Book{
		LibraryID:         1,
		Filepath:          bookDir,
		Title:             "Open Advice",
		TitleSource:       source,
		SortTitle:         "Open Advice",
		SortTitleSource:   source,
		Subtitle:          &subtitle,
		SubtitleSource:    &source,
		Description:       &description,
		DescriptionSource: &source,
		AuthorSource:      source,
		SeriesSource:      &source,
		GenreSource:       &source,
		TagSource:         &source,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, book))

	f := &bookOwnerFixture{tc: tc, bookDir: bookDir, book: book}
	f.epub = f.addFile(t, "Open Advice.epub", models.FileTypeEPUB)
	f.pdf = f.addFile(t, "Open Advice.pdf", models.FileTypePDF)
	f.setAuthors(t, "Lydia Pintscher")

	series, err := tc.worker.seriesService.FindOrCreateSeries(tc.ctx, "FOSS", 1, source)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1}).Exec(tc.ctx)
	require.NoError(t, err)
	genre, err := tc.worker.genreService.FindOrCreateGenre(tc.ctx, "Nonfiction", 1)
	require.NoError(t, err)
	require.NoError(t, tc.bookService.CreateBookGenre(tc.ctx, &models.BookGenre{BookID: book.ID, GenreID: genre.ID}))
	tag, err := tc.worker.tagService.FindOrCreateTag(tc.ctx, "essays", 1)
	require.NoError(t, err)
	require.NoError(t, tc.bookService.CreateBookTag(tc.ctx, &models.BookTag{BookID: book.ID, TagID: tag.ID}))
	return f
}

func (f *bookOwnerFixture) addFile(t *testing.T, name, fileType string) *models.File {
	t.Helper()
	file := &models.File{
		LibraryID: 1, BookID: f.book.ID, Filepath: filepath.Join(f.bookDir, name),
		FileType: fileType, FileRole: models.FileRoleMain, FilesizeBytes: 1000,
	}
	require.NoError(t, f.tc.bookService.CreateFile(f.tc.ctx, file))
	return file
}

// setAuthors replaces the book's authors directly, keeping AuthorSource.
func (f *bookOwnerFixture) setAuthors(t *testing.T, names ...string) {
	t.Helper()
	require.NoError(t, f.tc.bookService.DeleteAuthors(f.tc.ctx, f.book.ID))
	for i, name := range names {
		person, err := f.tc.personService.FindOrCreatePerson(f.tc.ctx, name, 1)
		require.NoError(t, err)
		require.NoError(t, f.tc.bookService.CreateAuthor(f.tc.ctx, &models.Author{BookID: f.book.ID, PersonID: person.ID, SortOrder: i + 1}))
	}
}

// scan runs scanFileCore for file the way a scan does, with the book and
// file reloaded so their relations are populated.
func (f *bookOwnerFixture) scan(t *testing.T, file *models.File, metadata *mediafile.ParsedMetadata, forceRefresh bool) *models.Book {
	t.Helper()
	book, err := f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &f.book.ID})
	require.NoError(t, err)
	loaded, err := f.tc.bookService.RetrieveFileWithRelations(f.tc.ctx, file.ID)
	require.NoError(t, err)
	_, err = f.tc.worker.scanFileCore(f.tc.ctx, loaded, book, metadata, forceRefresh, true, nil, nil)
	require.NoError(t, err)
	book, err = f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &f.book.ID})
	require.NoError(t, err)
	return book
}

func pdfMetadata() *mediafile.ParsedMetadata {
	return &mediafile.ParsedMetadata{
		Title:       "Open Advice: FOSS",
		Subtitle:    "From the PDF",
		Description: "From the PDF.",
		Authors:     []mediafile.ParsedAuthor{{Name: "Georg Greve"}, {Name: "Lydia Pintscher"}},
		Series:      "Open Source",
		Genres:      []string{"Technology"},
		Tags:        []string{"software"},
		DataSource:  models.DataSourcePDFMetadata,
	}
}

func authorNames(book *models.Book) []string {
	names := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		names = append(names, a.Person.Name)
	}
	return names
}

func TestScanFileCore_NonOwnerKeepsBookMetadata(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)

	book := f.scan(t, f.pdf, pdfMetadata(), false)

	assert.Equal(t, "Open Advice", book.Title)
	assert.Equal(t, "Open Advice", book.SortTitle)
	require.NotNil(t, book.Subtitle)
	assert.Equal(t, "From the EPUB", *book.Subtitle)
	require.NotNil(t, book.Description)
	assert.Equal(t, "From the EPUB.", *book.Description)
	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book))
	require.Len(t, book.BookSeries, 1)
	assert.Equal(t, "FOSS", book.BookSeries[0].Series.Name)
	require.Len(t, book.BookGenres, 1)
	assert.Equal(t, "Nonfiction", book.BookGenres[0].Genre.Name)
	require.Len(t, book.BookTags, 1)
	assert.Equal(t, "essays", book.BookTags[0].Tag.Name)
}

// Fields the owner left empty are still filled from another file.
func TestScanFileCore_NonOwnerFillsEmptyBookFields(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)
	_, err := f.tc.db.NewDelete().Model((*models.BookGenre)(nil)).Where("book_id = ?", f.book.ID).Exec(f.tc.ctx)
	require.NoError(t, err)
	_, err = f.tc.db.NewUpdate().Model(f.book).Set("genre_source = NULL").WherePK().Exec(f.tc.ctx)
	require.NoError(t, err)

	book := f.scan(t, f.pdf, pdfMetadata(), false)

	require.Len(t, book.BookGenres, 1)
	assert.Equal(t, "Technology", book.BookGenres[0].Genre.Name)
}

// A second file of the owner's format does not replace its values either.
func TestScanFileCore_NonOwnerOfSameFormatKeepsBookMetadata(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)
	second := f.addFile(t, "Open Advice (2nd edition).epub", models.FileTypeEPUB)

	metadata := pdfMetadata()
	metadata.DataSource = models.DataSourceEPUBMetadata
	book := f.scan(t, second, metadata, false)

	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book))
}

// The owner still updates the book, including over values another file set
// before this rule existed.
func TestScanFileCore_OwnerReplacesAnotherFilesBookMetadata(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)
	f.setAuthors(t, "Georg Greve", "Lydia Pintscher")
	_, err := f.tc.db.NewUpdate().Model(f.book).Set("author_source = ?", models.DataSourcePDFMetadata).WherePK().Exec(f.tc.ctx)
	require.NoError(t, err)

	book := f.scan(t, f.epub, &mediafile.ParsedMetadata{
		Authors:    []mediafile.ParsedAuthor{{Name: "Lydia Pintscher"}},
		DataSource: models.DataSourceEPUBMetadata,
	}, false)

	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book))
	assert.Equal(t, models.DataSourceEPUBMetadata, book.AuthorSource)
}

// Once the owner's file is gone, the next oldest main file owns the book.
func TestScanFileCore_NextFileOwnsBookAfterOwnerIsRemoved(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)
	_, err := f.tc.db.NewDelete().Model((*models.File)(nil)).Where("id = ?", f.epub.ID).Exec(f.tc.ctx)
	require.NoError(t, err)

	book := f.scan(t, f.pdf, pdfMetadata(), false)

	assert.Equal(t, []string{"Georg Greve", "Lydia Pintscher"}, authorNames(book))
}

// Refreshing one file takes that file's values, so a user can pick them.
func TestScanFileCore_NonOwnerReplacesOnFileRefresh(t *testing.T) {
	t.Parallel()
	f := newBookOwnerFixture(t)

	book := f.scan(t, f.pdf, pdfMetadata(), true)

	assert.Equal(t, []string{"Georg Greve", "Lydia Pintscher"}, authorNames(book))
	assert.Equal(t, models.DataSourcePDFMetadata, book.AuthorSource)
}

// Refreshing the whole book applies the owner last, so its values win, even
// though files are otherwise scanned in file-type order (mobi after epub).
func TestScanBook_RefreshAppliesOwnerLast(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Lydia Pintscher] Open Advice")
	testgen.GenerateEPUB(t, bookDir, "Open Advice.epub", testgen.EPUBOptions{
		Title: "Open Advice", Authors: []string{"Lydia Pintscher"},
	})
	require.NoError(t, tc.runScan())
	testgen.GenerateMOBI(t, bookDir, "Open Advice.mobi", testgen.MOBIOptions{
		Kind: testgen.MOBIKindMOBI6, Title: "Open Advice", Authors: []string{"Georg Greve", "Lydia Pintscher"},
	})
	require.NoError(t, tc.runScan())

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	bookID := allBooks[0].ID
	book, err := tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	require.Len(t, book.Files, 2)
	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book), "a normal scan keeps the EPUB's author")

	_, err = tc.worker.scanInternal(tc.ctx, ScanOptions{BookID: bookID, ForceRefresh: true, SkipPlugins: true}, nil)
	require.NoError(t, err)

	book, err = tc.bookService.RetrieveBook(tc.ctx, books.RetrieveBookOptions{ID: &bookID})
	require.NoError(t, err)
	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book))
	assert.Equal(t, models.DataSourceEPUBMetadata, book.AuthorSource)
}
