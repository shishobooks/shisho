package opds

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// listsFixture is a list owned by "owner" holding books from two libraries,
// shared with "viewer", who can access only the first library, and with
// "noaccess", who can access no library. "stranger" can access every library
// but has no lists. Apricot is in library A but not in the list. Apple's file
// has a publisher. Every book is in the search index.
type listsFixture struct {
	db    *bun.DB
	list  *models.List
	books map[string]*models.Book
	files map[string]*models.File
}

func newListsFixture(t *testing.T) *listsFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.New(t)
	booksRead := models.Permission{Resource: models.ResourceBooks, Operation: models.OperationRead}
	owner := newOPDSUser(t, db, "owner", booksRead)
	viewer := newOPDSUser(t, db, "viewer", booksRead)
	newOPDSUser(t, db, "stranger", booksRead)
	noAccess := newOPDSUser(t, db, "noaccess", booksRead)

	libA := &models.Library{Name: "Lib A", CoverAspectRatio: "book"}
	libB := &models.Library{Name: "Lib B", CoverAspectRatio: "book"}
	for _, lib := range []*models.Library{libA, libB} {
		_, err := db.NewInsert().Model(lib).Exec(ctx)
		require.NoError(t, err)
	}

	// Narrow the viewer to library A.
	_, err := db.NewDelete().Model((*models.UserLibraryAccess)(nil)).Where("user_id = ?", viewer.ID).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.UserLibraryAccess{UserID: viewer.ID, LibraryID: &libA.ID}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewDelete().Model((*models.UserLibraryAccess)(nil)).Where("user_id = ?", noAccess.ID).Exec(ctx)
	require.NoError(t, err)

	publisher := &models.Publisher{LibraryID: libB.ID, Name: "Orchard Press"}
	_, err = db.NewInsert().Model(publisher).Exec(ctx)
	require.NoError(t, err)

	f := &listsFixture{db: db, books: map[string]*models.Book{}, files: map[string]*models.File{}}
	for _, b := range []struct {
		title    string
		lib      *models.Library
		fileType string
	}{
		{"Cherry", libA, models.FileTypeEPUB},
		{"Apple", libB, models.FileTypeEPUB},
		{"Banana", libA, models.FileTypeCBZ},
		{"Apricot", libA, models.FileTypeEPUB},
	} {
		book := &models.Book{
			LibraryID:       b.lib.ID,
			Title:           b.title,
			TitleSource:     models.DataSourceFilepath,
			SortTitle:       b.title,
			SortTitleSource: models.DataSourceFilepath,
			AuthorSource:    models.DataSourceFilepath,
			Filepath:        "/books/" + b.title,
		}
		_, err := db.NewInsert().Model(book).Exec(ctx)
		require.NoError(t, err)
		file := &models.File{
			LibraryID:     b.lib.ID,
			BookID:        book.ID,
			FileType:      b.fileType,
			FileRole:      models.FileRoleMain,
			Filepath:      "/books/" + b.title + "/book." + b.fileType,
			FilesizeBytes: 1,
		}
		if b.title == "Apple" {
			file.PublisherID = &publisher.ID
		}
		_, err = db.NewInsert().Model(file).Exec(ctx)
		require.NoError(t, err)
		f.books[b.title] = book
		f.files[b.title] = file
		require.NoError(t, search.NewService(db).ReindexBookByID(ctx, book.ID))
	}

	now := time.Now()
	f.list = &models.List{
		CreatedAt:   now,
		UpdatedAt:   now,
		UserID:      owner.ID,
		Name:        "Favorites",
		DefaultSort: models.ListSortTitleAsc,
	}
	_, err = db.NewInsert().Model(f.list).Exec(ctx)
	require.NoError(t, err)
	for _, title := range []string{"Cherry", "Apple", "Banana"} {
		_, err = db.NewInsert().Model(&models.ListBook{ListID: f.list.ID, BookID: f.books[title].ID, AddedAt: now}).Exec(ctx)
		require.NoError(t, err)
	}
	for _, user := range []*models.User{viewer, noAccess} {
		_, err = db.NewInsert().Model(&models.ListShare{
			ListID:     f.list.ID,
			UserID:     user.ID,
			Permission: models.ListPermissionViewer,
			CreatedAt:  now,
		}).Exec(ctx)
		require.NoError(t, err)
	}

	return f
}

func (f *listsFixture) get(t *testing.T, username, path string) (int, string) {
	t.Helper()
	rec := serveOPDS(t, f.db, username, http.MethodGet, path)
	return rec.Code, rec.Body.String()
}

// entryTitles returns the entry titles of a feed body in order.
func entryTitles(body string) []string {
	var titles []string
	for _, part := range strings.Split(body, "<entry>")[1:] {
		start := strings.Index(part, "<title>") + len("<title>")
		end := strings.Index(part, "</title>")
		titles = append(titles, part[start:end])
	}
	return titles
}

func TestOPDS_RootCatalogLinksListsOnlyWhenUserHasLists(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	for _, username := range []string{"owner", "viewer"} {
		code, body := f.get(t, username, "/opds/v1/catalog")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, `href="http://example.com/opds/v1/lists"`, username)
	}

	code, body := f.get(t, "stranger", "/opds/v1/catalog")
	require.Equal(t, http.StatusOK, code, body)
	assert.NotContains(t, body, "/lists", "stranger")
}

func TestOPDS_ListsFeedShowsOwnedAndSharedLists(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)
	listURL := fmt.Sprintf(`href="http://example.com/opds/v1/lists/%d"`, f.list.ID)

	for _, username := range []string{"owner", "viewer"} {
		code, body := f.get(t, username, "/opds/v1/lists")
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, []string{"Favorites"}, entryTitles(body), username)
		assert.Contains(t, body, listURL, username)
	}

	code, body := f.get(t, "stranger", "/opds/v1/lists")
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, entryTitles(body))
}

func TestOPDS_ListBooksFeed(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)
	path := fmt.Sprintf("/opds/v1/lists/%d", f.list.ID)

	// The list's title_asc sort applies, and the CBZ-only book, which the
	// rules don't deliver, is filtered out.
	for _, username := range []string{"owner", "viewer", "noaccess"} {
		setRules(t, f.db, username, epubOnly)
	}
	code, body := f.get(t, "owner", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple", "Cherry"}, entryTitles(body))
	assert.Contains(t, body, fmt.Sprintf(`href="http://example.com/opds/download/%d.epub"`, f.files["Apple"].ID))
	assert.Contains(t, body, "<dc:publisher>Orchard Press</dc:publisher>")

	// The viewer cannot access library B, so its book is hidden.
	code, body = f.get(t, "viewer", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Cherry"}, entryTitles(body))

	// A user with no library access sees none of the list's books.
	code, body = f.get(t, "noaccess", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, entryTitles(body))

	// The built-in rules deliver the CBZ too.
	resetRules(t, f.db, "owner")
	code, body = f.get(t, "owner", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple", "Banana", "Cherry"}, entryTitles(body))
}

func TestOPDS_ListBooksFeed_Pagination(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	code, body := f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d?limit=2&offset=0", f.list.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple", "Banana"}, entryTitles(body))
	assert.Contains(t, body, fmt.Sprintf(`href="http://example.com/opds/v1/lists/%d?limit=2&amp;offset=2"`, f.list.ID))
}

func TestOPDS_ListBooksFeed_Kepub(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	setRules(t, f.db, "owner", epubAsKepub)
	code, body := f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d", f.list.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple", "Cherry"}, entryTitles(body))
	assert.Contains(t, body, fmt.Sprintf(`href="http://example.com/opds/download/%d.kepub.epub"`, f.files["Apple"].ID))
}

// A list the user neither owns nor has been shared is a 404, so its id does
// not confirm it exists.
func TestOPDS_ListBooksFeed_UnsharedListReturns404(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	for _, path := range []string{
		fmt.Sprintf("/opds/v1/lists/%d", f.list.ID),
		"/opds/v1/lists/999999",
	} {
		code, body := f.get(t, "stranger", path)
		assert.Equal(t, http.StatusNotFound, code, path+": "+body)
	}
}

func TestOPDS_ListBooksFeed_LinksListSearch(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)
	listURL := fmt.Sprintf("http://example.com/opds/v1/lists/%d", f.list.ID)

	code, body := f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d", f.list.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, `rel="search" href="`+listURL+`/opensearch.xml"`)

	code, body = f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d/opensearch.xml", f.list.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, listURL+"/search?q={searchTerms}")
}

// Search matches only the list's books, never other books in its libraries.
func TestOPDS_ListSearchFeed(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)
	path := fmt.Sprintf("/opds/v1/lists/%d/search?q=ap", f.list.ID)

	code, body := f.get(t, "owner", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple"}, entryTitles(body))

	// Neither the viewer nor noaccess can access library B, where Apple lives.
	for _, username := range []string{"viewer", "noaccess"} {
		code, body = f.get(t, username, path)
		require.Equal(t, http.StatusOK, code, body)
		assert.Empty(t, entryTitles(body), username)
	}

	// The search feed's up link points at the list, an acquisition feed.
	code, body = f.get(t, "owner", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, fmt.Sprintf(`<link rel="up" href="http://example.com/opds/v1/lists/%d" type="%s">`, f.list.ID, MimeTypeAcquisition))

	setRules(t, f.db, "owner", epubAsKepub)
	code, body = f.get(t, "owner", path)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple"}, entryTitles(body))
	assert.Contains(t, body, fmt.Sprintf(`href="http://example.com/opds/download/%d.kepub.epub"`, f.files["Apple"].ID))
}

func TestOPDS_ListSearchFeed_Pagination(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	// Every fixture book's path starts with /books, so "books" matches them all.
	code, body := f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d/search?q=books&limit=2&offset=0", f.list.ID))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []string{"Apple", "Banana"}, entryTitles(body))
	assert.Contains(t, body, fmt.Sprintf(`href="http://example.com/opds/v1/lists/%d/search?q=books&amp;limit=2&amp;offset=2"`, f.list.ID))
}

func TestOPDS_ListSearchFeed_Errors(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)

	code, body := f.get(t, "owner", fmt.Sprintf("/opds/v1/lists/%d/search", f.list.ID))
	assert.Equal(t, http.StatusUnprocessableEntity, code, body)

	for _, path := range []string{
		fmt.Sprintf("/opds/v1/lists/%d/search?q=ap", f.list.ID),
		fmt.Sprintf("/opds/v1/lists/%d/opensearch.xml", f.list.ID),
	} {
		code, body := f.get(t, "stranger", path)
		assert.Equal(t, http.StatusNotFound, code, path+": "+body)
	}
}

// A database failure while checking list access is a 500, not the 404 a
// list the user cannot view gets.
func TestOPDS_ListBooksFeed_AccessCheckFaultReturns500(t *testing.T) {
	t.Parallel()
	f := newListsFixture(t)
	_, err := f.db.ExecContext(context.Background(), "DROP TABLE list_shares")
	require.NoError(t, err)

	code, body := f.get(t, "viewer", fmt.Sprintf("/opds/v1/lists/%d", f.list.ID))
	assert.Equal(t, http.StatusInternalServerError, code, body)
}
