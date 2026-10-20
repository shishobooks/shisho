package people

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MainFileTypes keeps authors of a book with a main file of those types,
// and with ListID too, the same book has to be on the list.
func TestListPeople_MainFileTypes(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	lib := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO users (id, username, password_hash, role_id) VALUES (1, 'reader', 'hash', 1)`)
	require.NoError(t, err)
	list := &models.List{UserID: 1, Name: "Shelf", DefaultSort: models.ListSortAddedAtDesc}
	_, err = db.NewInsert().Model(list).Exec(ctx)
	require.NoError(t, err)

	addPerson := func(name string) *models.Person {
		p := &models.Person{LibraryID: lib.ID, Name: name, SortName: name, SortNameSource: models.DataSourceFilepath}
		_, err := db.NewInsert().Model(p).Exec(ctx)
		require.NoError(t, err)
		return p
	}
	addBook := func(author *models.Person, fileType string, onList bool) {
		b := &models.Book{LibraryID: lib.ID, Title: author.Name + fileType, TitleSource: models.DataSourceFilepath, SortTitle: author.Name + fileType, SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/" + author.Name + fileType}
		_, err := db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewInsert().Model(&models.File{LibraryID: lib.ID, BookID: b.ID, FileType: fileType, FileRole: models.FileRoleMain, Filepath: b.Filepath + "/f." + fileType, FilesizeBytes: 1}).Exec(ctx)
		require.NoError(t, err)
		_, err = db.NewInsert().Model(&models.Author{BookID: b.ID, PersonID: author.ID, SortOrder: 1}).Exec(ctx)
		require.NoError(t, err)
		if onList {
			_, err = db.ExecContext(ctx, `INSERT INTO list_books (list_id, book_id, sort_order, added_at) VALUES (?, ?, 1, CURRENT_TIMESTAMP)`, list.ID, b.ID)
			require.NoError(t, err)
		}
	}

	novelist := addPerson("Novelist")
	addBook(novelist, models.FileTypeEPUB, true)
	split := addPerson("Split")
	addBook(split, models.FileTypeCBZ, true)
	addBook(split, models.FileTypeEPUB, false)
	addPerson("Artist")
	addBook(addPerson("Comics"), models.FileTypeCBZ, false)

	names := func(opts ListPeopleOptions) []string {
		people, _, err := NewService(db).ListPeopleWithTotal(ctx, opts)
		require.NoError(t, err)
		got := []string{}
		for _, p := range people {
			got = append(got, p.Name)
		}
		return got
	}

	epub := []string{models.FileTypeEPUB}
	assert.ElementsMatch(t, []string{"Novelist", "Split"}, names(ListPeopleOptions{LibraryID: &lib.ID, MainFileTypes: epub}))
	assert.Equal(t, []string{"Novelist"}, names(ListPeopleOptions{LibraryID: &lib.ID, ListID: &list.ID, MainFileTypes: epub}),
		"Split's EPUB isn't on the list, and its book on the list isn't an EPUB")
	assert.Empty(t, names(ListPeopleOptions{LibraryID: &lib.ID, MainFileTypes: []string{}}))
}
