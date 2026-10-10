package opds

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// kindleFormatsFixture is a library with one book holding an EPUB, a MOBI,
// and an AZW3, and a user with Books Read.
type kindleFormatsFixture struct {
	db    *bun.DB
	lib   *models.Library
	files map[string]*models.File
}

func newKindleFormatsFixture(t *testing.T) *kindleFormatsFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	newOPDSUser(t, db, "reader", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})

	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Kindle Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Kindle Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/books/kindle",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	f := &kindleFormatsFixture{db: db, lib: lib, files: map[string]*models.File{}}
	for _, fileType := range []string{models.FileTypeEPUB, models.FileTypeMOBI, models.FileTypeAZW3} {
		file := &models.File{
			LibraryID:     lib.ID,
			BookID:        book.ID,
			FileType:      fileType,
			FileRole:      models.FileRoleMain,
			Filepath:      "/books/kindle/book." + fileType,
			FilesizeBytes: 1,
		}
		_, err := db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
		f.files[fileType] = file
	}
	return f
}

func (f *kindleFormatsFixture) get(t *testing.T, path string) (int, string) {
	t.Helper()
	rec := serveOPDS(t, f.db, "reader", http.MethodGet, path)
	return rec.Code, rec.Body.String()
}

// acquisitionLink is the XML an entry carries for a file's plain download.
func (f *kindleFormatsFixture) acquisitionLink(fileType, mimeType string) string {
	return fmt.Sprintf(`<link rel="http://opds-spec.org/acquisition" href="http://example.com/opds/download/%d" type="%s">`, f.files[fileType].ID, mimeType)
}

func TestOPDS_MOBIAndAZW3Feeds(t *testing.T) {
	t.Parallel()
	f := newKindleFormatsFixture(t)

	code, body := f.get(t, "/opds/v1/mobi+azw3/catalog")
	require.Equal(t, http.StatusOK, code, body)

	code, body = f.get(t, fmt.Sprintf("/opds/v1/mobi+azw3/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, f.acquisitionLink(models.FileTypeMOBI, "application/x-mobipocket-ebook"))
	assert.Contains(t, body, f.acquisitionLink(models.FileTypeAZW3, "application/vnd.amazon.mobi8-ebook"))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d\"", f.files[models.FileTypeEPUB].ID))

	code, body = f.get(t, fmt.Sprintf("/opds/v1/azw3/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, f.acquisitionLink(models.FileTypeAZW3, "application/vnd.amazon.mobi8-ebook"))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d\"", f.files[models.FileTypeMOBI].ID))
}

// KePub feeds are for Kobo devices, which cannot read MOBI or AZW3, so they
// skip those types. A KePub feed with no other type is refused.
func TestOPDS_KepubFeedsSkipMOBIAndAZW3(t *testing.T) {
	t.Parallel()
	f := newKindleFormatsFixture(t)

	for _, path := range []string{
		"/opds/v1/kepub/mobi/catalog",
		"/opds/v1/kepub/mobi+azw3/catalog",
		fmt.Sprintf("/opds/v1/kepub/azw3/libraries/%d/all", f.lib.ID),
	} {
		code, body := f.get(t, path)
		assert.Equal(t, http.StatusUnprocessableEntity, code, path+": "+body)
	}

	code, body := f.get(t, "/opds/v1/kepub/epub+mobi+azw3/catalog")
	require.Equal(t, http.StatusOK, code, body)

	code, body = f.get(t, fmt.Sprintf("/opds/v1/kepub/epub+mobi+azw3/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, fmt.Sprintf("/opds/download/%d/kepub\"", f.files[models.FileTypeEPUB].ID))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d\"", f.files[models.FileTypeMOBI].ID))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d\"", f.files[models.FileTypeAZW3].ID))
}

// A KePub feed that names AZW3 next to another type does not list a book
// whose only file is an AZW3.
func TestOPDS_KepubFeedOmitsKindleOnlyBooks(t *testing.T) {
	t.Parallel()
	f := newKindleFormatsFixture(t)
	ctx := context.Background()
	book := &models.Book{
		LibraryID:       f.lib.ID,
		Title:           "Kindle Only",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Kindle Only",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/books/kindle-only",
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	_, err = f.db.NewInsert().Model(&models.File{
		LibraryID: f.lib.ID, BookID: book.ID, FileType: models.FileTypeAZW3, FileRole: models.FileRoleMain,
		Filepath: "/books/kindle-only/book.azw3", FilesizeBytes: 1,
	}).Exec(ctx)
	require.NoError(t, err)

	code, body := f.get(t, fmt.Sprintf("/opds/v1/kepub/cbz+azw3/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "Kindle Only")
}

func TestValidateFileTypes(t *testing.T) {
	t.Parallel()

	for _, types := range []string{"epub", "cbz", "m4b", "pdf", "mobi", "azw3", "epub+cbz+m4b+pdf+mobi+azw3"} {
		require.NoError(t, validateFileTypes(types), types)
	}
	for _, types := range []string{"", "fb2", "epub+azw", "epub+"} {
		assert.Error(t, validateFileTypes(types), types)
	}
}

// Supplements are not acquisitions: a supplement neither gets a download link
// nor puts its book in a feed for its type. An .azw supplement is typed mobi.
func TestOPDS_SupplementsAreNotAcquisitions(t *testing.T) {
	t.Parallel()
	f := newKindleFormatsFixture(t)
	ctx := context.Background()
	book := &models.Book{
		LibraryID:       f.lib.ID,
		Title:           "Has Supplement",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Has Supplement",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/books/supplement",
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	mainFile := &models.File{
		LibraryID: f.lib.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		Filepath: "/books/supplement/book.epub", FilesizeBytes: 1,
	}
	supplement := &models.File{
		LibraryID: f.lib.ID, BookID: book.ID, FileType: models.FileTypeMOBI, FileRole: models.FileRoleSupplement,
		Filepath: "/books/supplement/extras.azw", FilesizeBytes: 1,
	}
	for _, file := range []*models.File{mainFile, supplement} {
		_, err = f.db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
	}

	code, body := f.get(t, fmt.Sprintf("/opds/v1/mobi/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "Has Supplement")

	code, body = f.get(t, fmt.Sprintf("/opds/v1/epub+mobi/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, fmt.Sprintf("/opds/download/%d\"", mainFile.ID))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d\"", supplement.ID))
}
