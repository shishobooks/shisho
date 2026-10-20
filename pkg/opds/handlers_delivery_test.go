package opds

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/delivery"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

var epubOnly = []delivery.Rule{{SourceFileType: models.FileTypeEPUB, DeliveredFileType: models.FileTypeEPUB}}

// setRules saves rules as username's default Delivery Rules.
func setRules(t *testing.T, db *bun.DB, username string, rules []delivery.Rule) {
	t.Helper()
	_, err := delivery.NewService(db).SaveUserDefault(context.Background(), userID(t, db, username), rules)
	require.NoError(t, err)
}

// resetRules makes username's default follow the built-in rules again.
func resetRules(t *testing.T, db *bun.DB, username string) {
	t.Helper()
	_, err := delivery.NewService(db).ResetUserDefault(context.Background(), userID(t, db, username))
	require.NoError(t, err)
}

func userID(t *testing.T, db *bun.DB, username string) int {
	t.Helper()
	var user models.User
	require.NoError(t, db.NewSelect().Model(&user).Where("username = ?", username).Scan(context.Background()))
	return user.ID
}

// deliveryFixture is a library and a user with Books Read, to which tests
// add books.
type deliveryFixture struct {
	t   *testing.T
	db  *bun.DB
	lib *models.Library
	// lastBook is the book addBook inserted last.
	lastBook *models.Book
}

func newDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	db := testdb.New(t)
	newOPDSUser(t, db, "reader", models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead})
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return &deliveryFixture{t: t, db: db, lib: lib}
}

// addBook inserts a book titled title with one main File of each type, by
// the given author when one is named, and returns its Files by type.
func (f *deliveryFixture) addBook(title, author string, fileTypes ...string) map[string]*models.File {
	f.t.Helper()
	ctx := context.Background()
	book := &models.Book{
		LibraryID:       f.lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/books/" + title,
	}
	_, err := f.db.NewInsert().Model(book).Exec(ctx)
	require.NoError(f.t, err)
	files := map[string]*models.File{}
	for _, fileType := range fileTypes {
		file := &models.File{
			LibraryID: f.lib.ID, BookID: book.ID, FileType: fileType, FileRole: models.FileRoleMain,
			Filepath: "/books/" + title + "/book." + fileType, FilesizeBytes: 1,
		}
		_, err := f.db.NewInsert().Model(file).Exec(ctx)
		require.NoError(f.t, err)
		files[fileType] = file
	}
	if author != "" {
		person := &models.Person{}
		err := f.db.NewSelect().Model(person).Where("name = ? AND library_id = ?", author, f.lib.ID).Scan(ctx)
		if err != nil {
			person = &models.Person{LibraryID: f.lib.ID, Name: author, SortName: author, SortNameSource: models.DataSourceFilepath}
			_, err = f.db.NewInsert().Model(person).Exec(ctx)
			require.NoError(f.t, err)
		}
		_, err = f.db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(f.t, err)
	}
	f.lastBook = book
	return files
}

func (f *deliveryFixture) get(path string) (int, string) {
	f.t.Helper()
	rec := serveOPDS(f.t, f.db, "reader", http.MethodGet, path)
	return rec.Code, rec.Body.String()
}

func acquisitionHref(file *models.File, ext string) string {
	return fmt.Sprintf(`href="http://example.com/opds/download/%d.%s"`, file.ID, ext)
}

// With the default untouched, the built-in rules deliver the earliest
// listed File Type each book has, as itself.
func TestOPDS_FeedFollowsTheBuiltInRules(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	kindle := f.addBook("Kindle Book", "", models.FileTypeEPUB, models.FileTypeMOBI, models.FileTypeAZW3)
	comic := f.addBook("Comic", "", models.FileTypeCBZ)

	code, body := f.get(fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(kindle[models.FileTypeEPUB], "epub")+` type="application/epub+zip" title="EPUB · Kindle Book"`)
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d.", kindle[models.FileTypeMOBI].ID))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d.", kindle[models.FileTypeAZW3].ID))
	assert.Contains(t, body, acquisitionHref(comic[models.FileTypeCBZ], "cbz")+` type="application/vnd.comicbook+zip"`)
}

func TestOPDS_FeedFollowsTheUsersDefaultRules(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	kindle := f.addBook("Kindle Book", "", models.FileTypeEPUB, models.FileTypeMOBI, models.FileTypeAZW3)
	comic := f.addBook("Comic", "", models.FileTypeCBZ)
	setRules(t, f.db, "reader", []delivery.Rule{
		{SourceFileType: models.FileTypeAZW3, DeliveredFileType: models.FileTypeAZW3},
		{SourceFileType: models.FileTypeCBZ, DeliveredFileType: models.FileTypeEPUB},
	})

	code, body := f.get(fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(kindle[models.FileTypeAZW3], "azw3")+` type="application/vnd.amazon.mobi8-ebook"`)
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d.", kindle[models.FileTypeEPUB].ID))
	assert.Contains(t, body, acquisitionHref(comic[models.FileTypeCBZ], "epub")+` type="application/epub+zip" title="EPUB · Comic"`)
}

// A KePub goes out typed as an EPUB, with a URL ending in .kepub.epub, since
// KOReader recognizes a download by its extension or content type.
func TestOPDS_KepubContentTypeAndURL(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	book := f.addBook("Novel", "", models.FileTypeEPUB)
	setRules(t, f.db, "reader", epubAsKepub)

	code, body := f.get(fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(book[models.FileTypeEPUB], "kepub.epub")+` type="application/epub+zip" title="KePub · Novel"`)
}

// A book the rules deliver nothing from is left out before paging, so page
// sizes and the next and last links count only books that are shown.
func TestOPDS_BooksWithNothingToDeliverAreHiddenAndPagingStaysCorrect(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	f.addBook("Audiobook A", "", models.FileTypeM4B)
	f.addBook("Novel B", "", models.FileTypeEPUB)
	f.addBook("Audiobook C", "", models.FileTypeM4B)
	f.addBook("Novel D", "", models.FileTypeEPUB, models.FileTypeM4B)
	f.addBook("Audiobook E", "", models.FileTypeM4B)
	setRules(t, f.db, "reader", epubOnly)
	allURL := fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID)

	code, body := f.get(allURL + "?limit=1&offset=0")
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, entryTitles(body), 1)
	assert.Contains(t, body, fmt.Sprintf(`rel="last" href="http://example.com%s?limit=1&amp;offset=1"`, allURL))

	code, body = f.get(allURL)
	require.Equal(t, http.StatusOK, code, body)
	assert.ElementsMatch(t, []string{"Novel B", "Novel D"}, entryTitles(body))

	code, body = f.get(fmt.Sprintf("/opds/v1/libraries/%d/search?q=audiobook", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, entryTitles(body))
}

// Series and author lists leave out entries with nothing to deliver, and
// count only the books that are delivered.
func TestOPDS_SeriesAndAuthorListsHideEmptyEntries(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	ctx := context.Background()
	addToSeries := func(name string, book *models.Book) {
		s := &models.Series{}
		err := f.db.NewSelect().Model(s).Where("name = ? AND library_id = ?", name, f.lib.ID).Scan(ctx)
		if err != nil {
			s = &models.Series{LibraryID: f.lib.ID, Name: name, NameSource: models.DataSourceManual, SortName: name, SortNameSource: models.DataSourceManual, CreatedAt: time.Now(), UpdatedAt: time.Now()}
			_, err = f.db.NewInsert().Model(s).Exec(ctx)
			require.NoError(t, err)
		}
		_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: s.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
	}
	f.addBook("Narrated", "Voice Only", models.FileTypeM4B)
	addToSeries("Audio Saga", f.lastBook)
	f.addBook("Printed", "Ink Writer", models.FileTypeEPUB)
	addToSeries("Paper Saga", f.lastBook)
	f.addBook("Printed Audio", "Ink Writer", models.FileTypeM4B)
	addToSeries("Paper Saga", f.lastBook)
	setRules(t, f.db, "reader", epubOnly)

	code, body := f.get(fmt.Sprintf("/opds/v1/libraries/%d/series", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Paper Saga"}, entryTitles(body))
	assert.Contains(t, body, "1 books in series")

	code, body = f.get(fmt.Sprintf("/opds/v1/libraries/%d/authors", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Ink Writer"}, entryTitles(body))
	assert.Contains(t, body, "1 books")

	code, body = f.get(fmt.Sprintf("/opds/v1/libraries/%d/authors/Voice%%20Only", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, entryTitles(body))

	// The built-in rules deliver the audiobooks too.
	resetRules(t, f.db, "reader")
	code, body = f.get(fmt.Sprintf("/opds/v1/libraries/%d/series", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Audio Saga", "Paper Saga"}, entryTitles(body))
}

// Supplements are not acquisitions: a supplement neither gets a download link
// nor puts its book in a feed. An .azw supplement is typed mobi.
func TestOPDS_SupplementsAreNotAcquisitions(t *testing.T) {
	t.Parallel()
	f := newDeliveryFixture(t)
	ctx := context.Background()
	files := f.addBook("Has Supplement", "", models.FileTypeEPUB)
	supplement := &models.File{
		LibraryID: f.lib.ID, BookID: f.lastBook.ID, FileType: models.FileTypeMOBI, FileRole: models.FileRoleSupplement,
		Filepath: "/books/supplement/extras.azw", FilesizeBytes: 1,
	}
	_, err := f.db.NewInsert().Model(supplement).Exec(ctx)
	require.NoError(t, err)

	setRules(t, f.db, "reader", []delivery.Rule{{SourceFileType: models.FileTypeMOBI, DeliveredFileType: models.FileTypeMOBI}})
	code, body := f.get(fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "Has Supplement")

	resetRules(t, f.db, "reader")
	code, body = f.get(fmt.Sprintf("/opds/v1/libraries/%d/all", f.lib.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, acquisitionHref(files[models.FileTypeEPUB], "epub"))
	assert.NotContains(t, body, fmt.Sprintf("/opds/download/%d.", supplement.ID))
}
