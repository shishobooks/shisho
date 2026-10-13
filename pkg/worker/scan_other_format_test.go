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

// A book with an EPUB and a PDF keeps the book-level metadata its first file
// set: scanning the other file does not replace it with that file's
// embedded values. Without this rule both formats share one priority, so the
// book takes each field from whichever file was scanned last.

type otherFormatFixture struct {
	tc   *testContext
	book *models.Book
	pdf  *models.File
}

func newOtherFormatFixture(t *testing.T) *otherFormatFixture {
	t.Helper()
	tc := newTestContext(t)
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "Open Advice")

	epubSource := models.DataSourceEPUBMetadata
	description := "From the EPUB."
	book := &models.Book{
		LibraryID:         1,
		Filepath:          bookDir,
		Title:             "Open Advice",
		TitleSource:       epubSource,
		SortTitle:         "Open Advice",
		SortTitleSource:   epubSource,
		Description:       &description,
		DescriptionSource: &epubSource,
		AuthorSource:      epubSource,
	}
	require.NoError(t, tc.bookService.CreateBook(tc.ctx, book))
	person, err := tc.personService.FindOrCreatePerson(tc.ctx, "Lydia Pintscher", 1)
	require.NoError(t, err)
	require.NoError(t, tc.bookService.CreateAuthor(tc.ctx, &models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}))

	require.NoError(t, tc.bookService.CreateFile(tc.ctx, &models.File{
		LibraryID: 1, BookID: book.ID, Filepath: filepath.Join(bookDir, "Open Advice.epub"),
		FileType: models.FileTypeEPUB, FilesizeBytes: 1000,
	}))
	pdf := &models.File{
		LibraryID: 1, BookID: book.ID, Filepath: filepath.Join(bookDir, "Open Advice.pdf"),
		FileType: models.FileTypePDF, FilesizeBytes: 1000,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, pdf))
	return &otherFormatFixture{tc: tc, book: book, pdf: pdf}
}

func (f *otherFormatFixture) scanPDF(t *testing.T, metadata *mediafile.ParsedMetadata, forceRefresh bool) *models.Book {
	t.Helper()
	book, err := f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &f.book.ID})
	require.NoError(t, err)
	file, err := f.tc.bookService.RetrieveFileWithRelations(f.tc.ctx, f.pdf.ID)
	require.NoError(t, err)
	_, err = f.tc.worker.scanFileCore(f.tc.ctx, file, book, metadata, forceRefresh, true, nil, nil)
	require.NoError(t, err)
	book, err = f.tc.bookService.RetrieveBook(f.tc.ctx, books.RetrieveBookOptions{ID: &f.book.ID})
	require.NoError(t, err)
	return book
}

func authorNames(book *models.Book) []string {
	names := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		names = append(names, a.Person.Name)
	}
	return names
}

func pdfMetadata() *mediafile.ParsedMetadata {
	return &mediafile.ParsedMetadata{
		Title:       "Open Advice: FOSS",
		Description: "From the PDF.",
		Authors:     []mediafile.ParsedAuthor{{Name: "Georg Greve"}, {Name: "Lydia Pintscher"}},
		Genres:      []string{"Technology"},
		Series:      "FOSS Books",
		DataSource:  models.DataSourcePDFMetadata,
	}
}

func TestScanFileCore_OtherFormatKeepsBookMetadata(t *testing.T) {
	t.Parallel()
	f := newOtherFormatFixture(t)

	book := f.scanPDF(t, pdfMetadata(), false)

	assert.Equal(t, "Open Advice", book.Title)
	assert.Equal(t, models.DataSourceEPUBMetadata, book.TitleSource)
	require.NotNil(t, book.Description)
	assert.Equal(t, "From the EPUB.", *book.Description)
	assert.Equal(t, []string{"Lydia Pintscher"}, authorNames(book))
	assert.Equal(t, models.DataSourceEPUBMetadata, book.AuthorSource)
}

// Fields the first file left empty are still filled from the other file.
func TestScanFileCore_OtherFormatFillsEmptyBookFields(t *testing.T) {
	t.Parallel()
	f := newOtherFormatFixture(t)

	book := f.scanPDF(t, pdfMetadata(), false)

	genres := make([]string, 0, len(book.BookGenres))
	for _, bg := range book.BookGenres {
		genres = append(genres, bg.Genre.Name)
	}
	assert.Equal(t, []string{"Technology"}, genres)
	require.Len(t, book.BookSeries, 1)
	assert.Equal(t, "FOSS Books", book.BookSeries[0].Series.Name)
}

// Once the PDF has set the genres, rescanning with different PDF genres
// still updates them: the rule only stops one format replacing another's.
func TestScanFileCore_SameFormatStillUpdatesBookMetadata(t *testing.T) {
	t.Parallel()
	f := newOtherFormatFixture(t)
	f.scanPDF(t, pdfMetadata(), false)

	metadata := pdfMetadata()
	metadata.Genres = []string{"Programming"}
	book := f.scanPDF(t, metadata, false)

	genres := make([]string, 0, len(book.BookGenres))
	for _, bg := range book.BookGenres {
		genres = append(genres, bg.Genre.Name)
	}
	assert.Equal(t, []string{"Programming"}, genres)
}

// Refresh still takes the scanned file's values.
func TestScanFileCore_OtherFormatReplacesOnRefresh(t *testing.T) {
	t.Parallel()
	f := newOtherFormatFixture(t)

	book := f.scanPDF(t, pdfMetadata(), true)

	assert.Equal(t, []string{"Georg Greve", "Lydia Pintscher"}, authorNames(book))
	assert.Equal(t, models.DataSourcePDFMetadata, book.AuthorSource)
}

func TestFromAnotherFileFormat(t *testing.T) {
	t.Parallel()
	assert.True(t, fromAnotherFileFormat(models.DataSourcePDFMetadata, models.DataSourceEPUBMetadata))
	assert.False(t, fromAnotherFileFormat(models.DataSourceEPUBMetadata, models.DataSourceEPUBMetadata))
	// The generic source a book merge writes is not one file's format.
	assert.False(t, fromAnotherFileFormat(models.DataSourceEPUBMetadata, models.DataSourceFileMetadata))
	assert.False(t, fromAnotherFileFormat(models.DataSourceEPUBMetadata, models.DataSourceFilepath))
	assert.False(t, fromAnotherFileFormat(models.DataSourcePlugin, models.DataSourceEPUBMetadata))
}
