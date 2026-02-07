package kobo

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetScopedFiles_OnlyReturnsPrimaryFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	bookSvc := books.NewService(db)
	koboSvc := NewService(db)

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with library access
	user := &models.User{
		Username:     "testuser",
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Grant library access
	libraryAccess := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: &library.ID,
	}
	_, err = db.NewInsert().Model(libraryAccess).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two EPUB files (both main role)
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = bookSvc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = bookSvc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Verify file1 is primary (first file created should be auto-set as primary)
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID)

	// Get scoped files for sync
	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	// Should only return the primary file (file1), not file2
	require.Len(t, files, 1)
	assert.Equal(t, file1.ID, files[0].FileID)
}

func TestGetScopedFiles_ReturnsNewPrimaryAfterChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	bookSvc := books.NewService(db)
	koboSvc := NewService(db)

	// Create library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with library access
	user := &models.User{
		Username:     "testuser",
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Grant library access
	libraryAccess := &models.UserLibraryAccess{
		UserID:    user.ID,
		LibraryID: &library.ID,
	}
	_, err = db.NewInsert().Model(libraryAccess).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two EPUB files
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = bookSvc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = bookSvc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Manually change primary to file2
	book.PrimaryFileID = &file2.ID
	_, err = db.NewUpdate().Model(book).Column("primary_file_id").Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)

	// Get scoped files for sync
	scope := &SyncScope{Type: "all"}
	files, err := koboSvc.GetScopedFiles(ctx, user.ID, scope)
	require.NoError(t, err)

	// Should only return file2 (the new primary)
	require.Len(t, files, 1)
	assert.Equal(t, file2.ID, files[0].FileID)
}
