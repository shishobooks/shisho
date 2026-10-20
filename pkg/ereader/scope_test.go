package ereader

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scopeBook struct {
	book *models.Book
	file *models.File
}

// insertScopeBook adds a book with one EPUB, written by author, to library.
func (f *eReaderAccessFixture) insertScopeBook(t *testing.T, libraryID int, title string, author *models.Person, series *models.Series) scopeBook {
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
	file := &models.File{
		LibraryID:     libraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(dir, title+".epub"),
		FilesizeBytes: 1,
	}
	_, err = f.db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)
	if author != nil {
		_, err = f.db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
	}
	if series != nil {
		_, err = f.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
	}
	return scopeBook{book: book, file: file}
}

func (f *eReaderAccessFixture) insertPerson(t *testing.T, libraryID int, name string) *models.Person {
	t.Helper()
	person := &models.Person{LibraryID: libraryID, Name: name, SortName: name, SortNameSource: models.DataSourceManual}
	_, err := f.db.NewInsert().Model(person).Exec(context.Background())
	require.NoError(t, err)
	return person
}

func (f *eReaderAccessFixture) insertSeries(t *testing.T, libraryID int, name string) *models.Series {
	t.Helper()
	series := &models.Series{
		LibraryID: libraryID, Name: name, NameSource: models.DataSourceManual,
		SortName: name, SortNameSource: models.DataSourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	_, err := f.db.NewInsert().Model(series).Exec(context.Background())
	require.NoError(t, err)
	return series
}

func (f *eReaderAccessFixture) insertList(t *testing.T, bookIDs ...int) *models.List {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	list := &models.List{UserID: f.user.ID, Name: "On the Kobo", DefaultSort: "added_at", CreatedAt: now, UpdatedAt: now}
	_, err := f.db.NewInsert().Model(list).Exec(ctx)
	require.NoError(t, err)
	for _, bookID := range bookIDs {
		_, err = f.db.NewInsert().Model(&models.ListBook{ListID: list.ID, BookID: bookID, AddedAt: now}).Exec(ctx)
		require.NoError(t, err)
	}
	return list
}

func (f *eReaderAccessFixture) setScope(t *testing.T, scope devices.Scope) {
	t.Helper()
	_, err := devices.NewService(f.db).Update(context.Background(), f.user.ID, f.device.ID, devices.UpdateDeviceOptions{Scope: &scope})
	require.NoError(t, err)
}

func byIDPaths(b scopeBook) []string {
	return []string{
		fmt.Sprintf("/download/%d", b.book.ID),
		fmt.Sprintf("/cover/%d", b.book.ID),
		fmt.Sprintf("/file/%d", b.file.ID),
		fmt.Sprintf("/file/%d/kepub", b.file.ID),
		fmt.Sprintf("/file/%d/kindle/book.epub", b.file.ID),
	}
}

// A Device scoped to one library shows only that library. Other libraries'
// pages and books are not found, though its owner can access them.
func TestEReaderScope_Library(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	inA := f.insertScopeBook(t, f.libA.ID, "Alpha Book", nil, nil)
	inB := f.insertScopeBook(t, f.libB.ID, "Bravo Book", nil, nil)
	f.libB.Name = "Bravo Library"
	_, err := f.db.NewUpdate().Model(f.libB).Column("name").WherePK().Exec(context.Background())
	require.NoError(t, err)

	f.setScope(t, devices.Scope{Type: devices.ScopeLibrary, LibraryID: &f.libA.ID})

	root := f.serve("/")
	require.Equal(t, http.StatusOK, root.Code)
	assert.Contains(t, root.Body.String(), fmt.Sprintf("/libraries/%d", f.libA.ID))
	assert.NotContains(t, root.Body.String(), "Bravo Library")

	for _, page := range []string{"", "/all", "/series", "/authors", "/search?q=Bravo"} {
		assert.Equal(t, http.StatusNotFound, f.serve(fmt.Sprintf("/libraries/%d%s", f.libB.ID, page)).Code, page)
	}
	all := f.serve(fmt.Sprintf("/libraries/%d/all", f.libA.ID))
	require.Equal(t, http.StatusOK, all.Code)
	assert.Contains(t, all.Body.String(), "Alpha Book")

	for _, path := range byIDPaths(inB) {
		rec := f.serve(path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "Bravo Book", path)
	}
	assert.Equal(t, http.StatusOK, f.serve(fmt.Sprintf("/download/%d", inA.book.ID)).Code)
}

// A Device scoped to a list shows only the list's books, and only the
// libraries, series, and authors that have one.
func TestEReaderScope_List(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	listedAuthor := f.insertPerson(t, f.libA.ID, "Listed Author")
	otherAuthor := f.insertPerson(t, f.libA.ID, "Other Author")
	listedSeries := f.insertSeries(t, f.libA.ID, "Listed Series")
	otherSeries := f.insertSeries(t, f.libA.ID, "Other Series")
	listed := f.insertScopeBook(t, f.libA.ID, "Listed Book", listedAuthor, listedSeries)
	unlisted := f.insertScopeBook(t, f.libA.ID, "Unlisted Book", otherAuthor, otherSeries)
	f.insertScopeBook(t, f.libB.ID, "Other Library Book", nil, nil)
	list := f.insertList(t, listed.book.ID)

	f.setScope(t, devices.Scope{Type: devices.ScopeList, ListID: &list.ID})

	root := f.serve("/")
	require.Equal(t, http.StatusOK, root.Code)
	assert.Contains(t, root.Body.String(), fmt.Sprintf("/libraries/%d\"", f.libA.ID))
	assert.NotContains(t, root.Body.String(), fmt.Sprintf("/libraries/%d\"", f.libB.ID), "a library with no listed books is hidden")

	libA := fmt.Sprintf("/libraries/%d", f.libA.ID)
	for page, want := range map[string]string{
		"/all":     "Listed Book",
		"/series":  "Listed Series",
		"/authors": "Listed Author",
		fmt.Sprintf("/series/%d", listedSeries.ID):  "Listed Book",
		fmt.Sprintf("/authors/%d", listedAuthor.ID): "Listed Book",
	} {
		rec := f.serve(libA + page)
		require.Equal(t, http.StatusOK, rec.Code, page)
		assert.Contains(t, rec.Body.String(), want, page)
		for _, hidden := range []string{"Unlisted Book", "Other Series", "Other Author"} {
			assert.NotContains(t, rec.Body.String(), hidden, page)
		}
	}

	for _, path := range byIDPaths(unlisted) {
		assert.Equal(t, http.StatusNotFound, f.serve(path).Code, path)
	}
	for _, page := range []string{
		fmt.Sprintf("/series/%d", otherSeries.ID),
		fmt.Sprintf("/authors/%d", otherAuthor.ID),
	} {
		rec := f.serve(libA + page)
		assert.Equal(t, http.StatusNotFound, rec.Code, page)
		assert.NotContains(t, rec.Body.String(), "Other", page)
	}
	assert.Equal(t, http.StatusOK, f.serve(fmt.Sprintf("/download/%d", listed.book.ID)).Code)
}

// A list the owner can no longer see shows nothing, as Kobo sync does.
func TestEReaderScope_ListNotVisibleShowsNothing(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	listed := f.insertScopeBook(t, f.libA.ID, "Listed Book", nil, nil)
	list := f.insertList(t, listed.book.ID)
	f.setScope(t, devices.Scope{Type: devices.ScopeList, ListID: &list.ID})
	other := &models.User{Username: "other", PasswordHash: "x", RoleID: f.user.RoleID, IsActive: true}
	_, err := f.db.NewInsert().Model(other).Exec(context.Background())
	require.NoError(t, err)
	_, err = f.db.NewUpdate().Model((*models.List)(nil)).Set("user_id = ?", other.ID).Where("id = ?", list.ID).Exec(context.Background())
	require.NoError(t, err)

	root := f.serve("/")
	require.Equal(t, http.StatusOK, root.Code)
	assert.NotContains(t, root.Body.String(), "/libraries/")
	assert.Equal(t, http.StatusNotFound, f.serve(fmt.Sprintf("/libraries/%d/all", f.libA.ID)).Code)
	assert.Equal(t, http.StatusNotFound, f.serve(fmt.Sprintf("/download/%d", listed.book.ID)).Code)
}

// A list scope that cannot be checked is a server error, not the 404 a book
// off the list gets.
func TestEReaderScope_ListCheckFailureReturns500(t *testing.T) {
	t.Parallel()
	f := newEReaderAccessFixture(t)
	listed := f.insertScopeBook(t, f.libA.ID, "Listed Book", nil, nil)
	list := f.insertList(t, listed.book.ID)
	f.setScope(t, devices.Scope{Type: devices.ScopeList, ListID: &list.ID})
	_, err := f.db.ExecContext(context.Background(), `ALTER TABLE list_books RENAME TO list_books_gone`)
	require.NoError(t, err)

	for _, path := range []string{
		fmt.Sprintf("/download/%d", listed.book.ID),
		fmt.Sprintf("/cover/%d", listed.book.ID),
		fmt.Sprintf("/file/%d", listed.file.ID),
	} {
		assert.Equal(t, http.StatusInternalServerError, f.serve(path).Code, path)
	}
}
