package books

import (
	"context"
	"testing"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HasBooks, LibraryIDsWithBooks, and MainFileTypesWithBooks answer from the
// same filters as ListBooksWithTotal, without loading books.
func TestService_ExistenceQueriesUseTheListFilters(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	ctx := context.Background()
	svc := NewService(db, appsettings.NewService(db))
	addLibrary := func(name string) *models.Library {
		lib := &models.Library{Name: name, CoverAspectRatio: "book"}
		_, err := db.NewInsert().Model(lib).Exec(ctx)
		require.NoError(t, err)
		return lib
	}
	addBook := func(lib *models.Library, title string, files map[string]string) *models.Book {
		b := &models.Book{LibraryID: lib.ID, Title: title, TitleSource: models.DataSourceFilepath, SortTitle: title, SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: "/" + title}
		_, err := db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
		for fileType, role := range files {
			_, err := db.NewInsert().Model(&models.File{LibraryID: lib.ID, BookID: b.ID, FileType: fileType, FileRole: role, Filepath: b.Filepath + "/f." + fileType, FilesizeBytes: 1}).Exec(ctx)
			require.NoError(t, err)
		}
		return b
	}
	novels := addLibrary("Novels")
	comics := addLibrary("Comics")
	empty := addLibrary("Empty")
	addBook(novels, "Novel", map[string]string{models.FileTypeEPUB: models.FileRoleMain, models.FileTypePDF: models.FileRoleSupplement})
	comic := addBook(comics, "Comic", map[string]string{models.FileTypeCBZ: models.FileRoleMain})
	person := seedPerson(t, db, comics, "Artist")
	seedAuthor(t, db, comic, person)

	cbz := []string{models.FileTypeCBZ}
	has, err := svc.HasBooks(ctx, ListBooksOptions{MainFileTypes: cbz})
	require.NoError(t, err)
	assert.True(t, has)
	has, err = svc.HasBooks(ctx, ListBooksOptions{LibraryID: &novels.ID, MainFileTypes: cbz})
	require.NoError(t, err)
	assert.False(t, has)
	has, err = svc.HasBooks(ctx, ListBooksOptions{PersonID: &person.ID})
	require.NoError(t, err)
	assert.True(t, has)
	has, err = svc.HasBooks(ctx, ListBooksOptions{LibraryID: &empty.ID})
	require.NoError(t, err)
	assert.False(t, has)

	ids, err := svc.LibraryIDsWithBooks(ctx, ListBooksOptions{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{novels.ID, comics.ID}, ids)
	ids, err = svc.LibraryIDsWithBooks(ctx, ListBooksOptions{MainFileTypes: cbz})
	require.NoError(t, err)
	assert.Equal(t, []int{comics.ID}, ids)

	// A supplement's type doesn't count.
	types, err := svc.MainFileTypesWithBooks(ctx, ListBooksOptions{LibraryID: &novels.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{models.FileTypeEPUB}, types)
	types, err = svc.MainFileTypesWithBooks(ctx, ListBooksOptions{MainFileTypes: cbz})
	require.NoError(t, err)
	assert.Equal(t, cbz, types)
}
