package ereader

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertTypedBook adds a book with a main File of each of mainTypes to
// library, by author and in series when they are set, and indexes it for
// search.
func (f *eReaderAccessFixture) insertTypedBook(t *testing.T, libraryID int, title string, author *models.Person, series *models.Series, mainTypes ...string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	book := &models.Book{
		LibraryID:       libraryID,
		Title:           title,
		Filepath:        dir,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	for _, fileType := range mainTypes {
		_, err = f.db.NewInsert().Model(&models.File{
			LibraryID:     libraryID,
			BookID:        book.ID,
			FileType:      fileType,
			FileRole:      models.FileRoleMain,
			Filepath:      filepath.Join(dir, title+"."+fileType),
			FilesizeBytes: 1,
		}).Exec(ctx)
		require.NoError(t, err)
	}
	if author != nil {
		_, err = f.db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
	}
	if series != nil {
		_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
	}
	require.NoError(t, search.NewService(f.db).ReindexBookByID(ctx, book.ID))
}

// kepubAsEPUB saves the built-in rules with KePub sent as EPUB as the
// user's default, which the Browser Device follows, so a KePub's delivered
// File Type differs from its own.
func (f *eReaderAccessFixture) kepubAsEPUB(t *testing.T) {
	t.Helper()
	rules := delivery.BuiltInRules()
	for i := range rules {
		if rules[i].SourceFileType == models.FileTypeKePub {
			rules[i].DeliveredFileType = models.FileTypeEPUB
		}
	}
	_, err := delivery.NewService(f.db).SaveUserDefault(context.Background(), f.user.ID, rules)
	require.NoError(t, err)
}

// The type filter offers and matches the File Type each book is sent as,
// not the types of its Files. Under rules that send KePub as EPUB, which the
// Browser Device follows, a KePub-only book is sent as EPUB, and a book with
// an EPUB and a CBZ sends only its EPUB. The library, series, author, and
// search pages all agree.
func TestEReaderTypeFilter_MatchesDeliveredFileType(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	f.kepubAsEPUB(t)
	lib := f.libA.ID
	author := f.insertPerson(t, lib, "Writer")
	series := f.insertSeries(t, lib, "Saga")
	f.insertTypedBook(t, lib, "Kobo Book", author, series, models.FileTypeKePub)
	f.insertTypedBook(t, lib, "Mixed Book", author, series, models.FileTypeEPUB, models.FileTypeCBZ)
	f.insertTypedBook(t, lib, "Comic Book", author, series, models.FileTypeCBZ)

	pages := map[string]string{
		"all":    fmt.Sprintf("/libraries/%d/all", lib),
		"series": fmt.Sprintf("/libraries/%d/series/%d", lib, series.ID),
		"author": fmt.Sprintf("/libraries/%d/authors/%d", lib, author.ID),
		"search": fmt.Sprintf("/libraries/%d/search?q=Book", lib),
	}
	withTypes := func(path, fileType string) string {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + "types=" + fileType
	}

	for name, path := range pages {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := f.serve(path)
			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, ">EPUB</a>")
			assert.Contains(t, body, ">CBZ</a>")
			assert.NotContains(t, body, ">KePub</a>", "nothing is sent as KePub")

			epub := f.serve(withTypes(path, models.FileTypeEPUB)).Body.String()
			assert.Contains(t, epub, "Kobo Book")
			assert.Contains(t, epub, "Mixed Book")
			assert.NotContains(t, epub, "Comic Book")

			cbz := f.serve(withTypes(path, models.FileTypeCBZ)).Body.String()
			assert.Contains(t, cbz, "Comic Book")
			assert.NotContains(t, cbz, "Mixed Book", "only its EPUB is sent")
			assert.NotContains(t, cbz, "Kobo Book")

			// A File Type nothing is sent as, as in a link saved before the
			// rules changed, shows every book, with All selected.
			kepub := f.serve(withTypes(path, models.FileTypeKePub)).Body.String()
			assert.Contains(t, kepub, "Kobo Book")
			assert.Contains(t, kepub, "Mixed Book")
			assert.Contains(t, kepub, "Comic Book")
			assert.Contains(t, kepub, `font-weight: bold; border-color: #000;">All</span>`)
			assert.NotContains(t, kepub, "types=kepub", "links on the page drop the stale filter")
		})
	}
}

// The type filter narrows in SQL before paging, so the page count follows
// the filtered books.
func TestEReaderTypeFilter_PaginatesFilteredBooks(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	f.kepubAsEPUB(t)
	lib := f.libA.ID
	for i := 0; i < defaultPageSize+2; i++ {
		f.insertTypedBook(t, lib, fmt.Sprintf("Kobo %03d", i), nil, nil, models.FileTypeKePub)
	}
	for i := 0; i < defaultPageSize+2; i++ {
		f.insertTypedBook(t, lib, fmt.Sprintf("Mixed %03d", i), nil, nil, models.FileTypeEPUB, models.FileTypeCBZ)
	}
	f.insertTypedBook(t, lib, "Comic", nil, nil, models.FileTypeCBZ)

	epub := f.serve(fmt.Sprintf("/libraries/%d/all?types=epub", lib)).Body.String()
	assert.Contains(t, epub, "Page 1 of 3", "every KePub-only and mixed book is sent as EPUB")

	cbz := f.serve(fmt.Sprintf("/libraries/%d/all?types=cbz", lib)).Body.String()
	assert.Contains(t, cbz, "Comic")
	assert.NotContains(t, cbz, "Page 1 of", "one CBZ book fits on one page")
	assert.NotContains(t, cbz, "Mixed")
}
