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

// insertList creates a list owned by the user holding the given books.
func insertList(ctx context.Context, t *testing.T, koboSvc *Service, userID int, bookIDs ...int) *models.List {
	t.Helper()
	now := time.Now()
	list := &models.List{
		UserID:      userID,
		Name:        "Sync List",
		DefaultSort: "added_at",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := koboSvc.db.NewInsert().Model(list).Exec(ctx)
	require.NoError(t, err)
	for _, bookID := range bookIDs {
		_, err = koboSvc.db.NewInsert().Model(&models.ListBook{
			ListID:  list.ID,
			BookID:  bookID,
			AddedAt: now,
		}).Exec(ctx)
		require.NoError(t, err)
	}
	return list
}

// insertOtherUser creates a second user who can own lists the test user does
// not own.
func insertOtherUser(ctx context.Context, t *testing.T, koboSvc *Service) *models.User {
	t.Helper()
	other := &models.User{
		Username:     "otheruser",
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := koboSvc.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)
	return other
}

func TestGetScopedFiles_ListScopeFiltersByLibraryAccess(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	// A second library the user cannot access, holding a book on the same list.
	other := &models.Library{
		Name:                     "Other Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err := koboSvc.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)

	mine := createBook(ctx, t, bookSvc, library.ID, "Mine")
	mineFile := createFile(ctx, t, bookSvc, library.ID, mine.ID, "/tmp/test/list/mine.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	theirs := createBook(ctx, t, bookSvc, other.ID, "Theirs")
	_ = createFile(ctx, t, bookSvc, other.ID, theirs.ID, "/tmp/test/list/theirs.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	list := insertList(ctx, t, koboSvc, user.ID, mine.ID, theirs.ID)

	scope := &SyncScope{Type: "list", ListID: &list.ID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Equal(t, []int{mineFile.ID}, scopedFileIDs(files), "a list scope only syncs books in libraries the user can access")
}

func TestGetScopedFiles_ListScopeWithAllLibraryAccess(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	// Replace the user's single-library grant with the all-libraries grant.
	_, err := koboSvc.db.NewDelete().Model((*models.UserLibraryAccess)(nil)).Where("user_id = ?", user.ID).Exec(ctx)
	require.NoError(t, err)
	_, err = koboSvc.db.NewInsert().Model(&models.UserLibraryAccess{UserID: user.ID}).Exec(ctx)
	require.NoError(t, err)

	other := &models.Library{
		Name:                     "Other Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err = koboSvc.db.NewInsert().Model(other).Exec(ctx)
	require.NoError(t, err)

	mine := createBook(ctx, t, bookSvc, library.ID, "Mine")
	mineFile := createFile(ctx, t, bookSvc, library.ID, mine.ID, "/tmp/test/list-all/mine.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	theirs := createBook(ctx, t, bookSvc, other.ID, "Theirs")
	theirsFile := createFile(ctx, t, bookSvc, other.ID, theirs.ID, "/tmp/test/list-all/theirs.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	list := insertList(ctx, t, koboSvc, user.ID, mine.ID, theirs.ID)

	scope := &SyncScope{Type: "list", ListID: &list.ID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Equal(t, []int{mineFile.ID, theirsFile.ID}, scopedFileIDs(files))
}

func TestGetScopedFiles_ListScopeWithoutLibraryAccessSyncsNothing(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	// Drop the user's only library grant so they can access no library at all.
	_, err := koboSvc.db.NewDelete().Model((*models.UserLibraryAccess)(nil)).Where("user_id = ?", user.ID).Exec(ctx)
	require.NoError(t, err)

	book := createBook(ctx, t, bookSvc, library.ID, "Unreachable")
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/list-none/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	list := insertList(ctx, t, koboSvc, user.ID, book.ID)

	scope := &SyncScope{Type: "list", ListID: &list.ID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Empty(t, files)
}

func TestGetScopedFiles_ListScopeUnsharedListSyncsNothing(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	// The user can access the library, but the list belongs to someone else
	// and is not shared with them.
	owner := insertOtherUser(ctx, t, koboSvc)
	book := createBook(ctx, t, bookSvc, library.ID, "Private")
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/list-unshared/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	list := insertList(ctx, t, koboSvc, owner.ID, book.ID)

	scope := &SyncScope{Type: "list", ListID: &list.ID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.NotNil(t, files)
	assert.Empty(t, files, "a list the user neither owns nor has a share on must not sync")
}

func TestGetScopedFiles_ListScopeSharedListSyncs(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	owner := insertOtherUser(ctx, t, koboSvc)
	book := createBook(ctx, t, bookSvc, library.ID, "Shared")
	epub := createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/list-shared/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/list-shared/book.m4b", models.FileTypeM4B, models.FileRoleMain, 5000)
	list := insertList(ctx, t, koboSvc, owner.ID, book.ID)

	// A viewer share is enough to sync the list.
	_, err := koboSvc.db.NewInsert().Model(&models.ListShare{
		ListID:         list.ID,
		UserID:         user.ID,
		Permission:     models.ListPermissionViewer,
		CreatedAt:      time.Now(),
		SharedByUserID: &owner.ID,
	}).Exec(ctx)
	require.NoError(t, err)

	scope := &SyncScope{Type: "list", ListID: &list.ID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.Equal(t, []int{epub.ID}, scopedFileIDs(files))
}

func TestGetScopedFiles_ListScopeMissingListSyncsNothing(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "Orphan")
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/list-missing/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)

	missingID := 999999
	scope := &SyncScope{Type: "list", ListID: &missingID}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	assert.NotNil(t, files)
	assert.Empty(t, files)
}

func TestGetScopedFiles_ScopeWithoutIDSyncsNothing(t *testing.T) {
	t.Parallel()
	ctx, bookSvc, koboSvc, library, user := setupScopedFilesTest(t)

	book := createBook(ctx, t, bookSvc, library.ID, "Unscoped")
	_ = createFile(ctx, t, bookSvc, library.ID, book.ID, "/tmp/test/no-id/book.epub", models.FileTypeEPUB, models.FileRoleMain, 1000)

	// A library or list scope missing its id must fail closed instead of
	// falling through to every file.
	for _, scopeType := range []string{"library", "list"} {
		files, err := koboSvc.GetScopedFiles(ctx, user.ID, &SyncScope{Type: scopeType})
		require.NoError(t, err)
		assert.NotNil(t, files, scopeType)
		assert.Empty(t, files, scopeType)
	}
}
