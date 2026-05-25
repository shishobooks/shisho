package kobo

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupScopedFilesTest creates a library, user with access, and returns them
// along with a book service and kobo service. Reduces boilerplate across tests.
func setupScopedFilesTest(t *testing.T) (context.Context, *books.Service, *Service, *models.Library, *models.User) {
	t.Helper()
	ctx := context.Background()
	db := setupTestDB(t)
	bookSvc := books.NewService(db)
	koboSvc := NewService(db)

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	user := &models.User{
		Username:     "testuser",
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	libraryAccess := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: &library.ID,
	}
	_, err = db.NewInsert().Model(libraryAccess).Exec(ctx)
	require.NoError(t, err)

	return ctx, bookSvc, koboSvc, library, user
}

func createBook(ctx context.Context, t *testing.T, bookSvc *books.Service, libraryID int, title string) *models.Book {
	t.Helper()
	db := bookSvc.DB()
	book := &models.Book{
		LibraryID:       libraryID,
		Filepath:        "/tmp/test/" + title,
		Title:           title,
		SortTitle:       title,
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	return book
}

func createFile(ctx context.Context, t *testing.T, bookSvc *books.Service, libraryID, bookID int, filepath, fileType, fileRole string, size int64) *models.File {
	t.Helper()
	file := &models.File{
		LibraryID:     libraryID,
		BookID:        bookID,
		Filepath:      filepath,
		FileType:      fileType,
		FileRole:      fileRole,
		FilesizeBytes: size,
	}
	err := bookSvc.CreateFile(ctx, file)
	require.NoError(t, err)
	return file
}

func scopedFileIDs(files []ScopedFile) []int {
	ids := make([]int, len(files))
	for i, f := range files {
		ids[i] = f.FileID
	}
	sort.Ints(ids)
	return ids
}

func TestGetScopedFiles_TwoEPUBsSyncsBoth(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "Two EPUBs")
	file1 := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/two-epubs/edition1.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	file2 := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/two-epubs/edition2.epub", models.FileTypeEPUB, models.FileRoleMain, 2000)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Equal(t, []int{file1.ID, file2.ID}, scopedFileIDs(files))
}

func TestGetScopedFiles_EPUBPlusM4BSyncsOnlyEPUB(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "EPUB and M4B")
	epub := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/epub-m4b/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/epub-m4b/book.m4b", models.FileTypeM4B, models.FileRoleMain, 5000)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	require.Len(t, files, 1)
	assert.Equal(t, epub.ID, files[0].FileID)
}

func TestGetScopedFiles_OnlyM4BSyncsNothing(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "M4B Only")
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/m4b-only/audiobook.m4b", models.FileTypeM4B, models.FileRoleMain, 5000)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Empty(t, files)
}

func TestGetScopedFiles_SingleEPUBSyncsNormally(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "Single EPUB")
	epub := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/single-epub/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	require.Len(t, files, 1)
	assert.Equal(t, epub.ID, files[0].FileID)
}

func TestGetScopedFiles_SupplementFilesExcluded(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "With Supplement")
	mainFile := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/supplement/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/supplement/guide.epub", models.FileTypeEPUB, models.FileRoleSupplement, 500)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	require.Len(t, files, 1)
	assert.Equal(t, mainFile.ID, files[0].FileID)
}

func TestGetScopedFiles_CBZFilesSyncToo(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "EPUB and CBZ")
	epub := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/epub-cbz/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	cbz := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/epub-cbz/book.cbz", models.FileTypeCBZ, models.FileRoleMain, 2000)

	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Equal(t, []int{epub.ID, cbz.ID}, scopedFileIDs(files))
}
