package books

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDBForPrimaryFile(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestCreateFile_SetsPrimaryForFirstFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book with no primary file
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	assert.Nil(t, book.PrimaryFileID)

	// Create the first file
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file)
	require.NoError(t, err)

	// Reload the book and verify primary_file_id is set
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file.ID, *book.PrimaryFileID)
}

func TestCreateFile_DoesNotChangePrimaryForSubsequentFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book with no primary file
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create the first file
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	// Create a second file
	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.pdf",
		FileType:      "pdf",
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Reload the book - primary should still be first file
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID, "Primary should remain the first file")
}

func TestDeleteFile_PromotesPrimaryWhenPrimaryDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files - first becomes primary
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Verify file1 is primary
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID)

	// Delete the primary file
	err = svc.DeleteFile(ctx, file1.ID)
	require.NoError(t, err)

	// Reload the book - file2 should now be primary
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file2.ID, *book.PrimaryFileID)
}

func TestDeleteFile_PromotesMainFileOverSupplement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create primary file (main)
	filePrimary := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/primary.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, filePrimary)
	require.NoError(t, err)

	// Create supplement (older timestamp to test priority logic)
	fileSupplement := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/supplement.pdf",
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 500,
	}
	err = svc.CreateFile(ctx, fileSupplement)
	require.NoError(t, err)

	// Create another main file (newest)
	fileMain2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/main2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, fileMain2)
	require.NoError(t, err)

	// Delete the primary file
	err = svc.DeleteFile(ctx, filePrimary.ID)
	require.NoError(t, err)

	// Reload the book - should promote fileMain2 (main) over fileSupplement (supplement)
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, fileMain2.ID, *book.PrimaryFileID, "Should promote main file over supplement")
}

func TestDeleteFile_DoesNotChangePrimaryWhenNonPrimaryDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file1.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/tmp/test/book/file2.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Verify file1 is primary
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID)

	// Delete the non-primary file (file2)
	err = svc.DeleteFile(ctx, file2.ID)
	require.NoError(t, err)

	// Reload the book - file1 should still be primary
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID, "Primary should not change when non-primary is deleted")
}

func TestCreateBook_SetsPrimaryWhenFilesInsertedInline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book with inline files
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Files: []*models.File{
			{
				LibraryID:     library.ID,
				Filepath:      "/tmp/test/book/file1.epub",
				FileType:      models.FileTypeEPUB,
				FileRole:      models.FileRoleMain,
				FilesizeBytes: 1000,
			},
			{
				LibraryID:     library.ID,
				Filepath:      "/tmp/test/book/file2.epub",
				FileType:      models.FileTypeEPUB,
				FileRole:      models.FileRoleMain,
				FilesizeBytes: 2000,
			},
		},
	}
	err = svc.CreateBook(ctx, book)
	require.NoError(t, err)

	// Reload the book and verify primary_file_id is set to first file
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID, "PrimaryFileID should be set when files are inserted inline")
	assert.Equal(t, book.Files[0].ID, *book.PrimaryFileID, "Primary should be the first inline file")
}

func TestDeleteFileAndCleanup_PromotesPrimaryFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDBForPrimaryFile(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        t.TempDir(),
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files via CreateFile (so primary_file_id is set)
	file1Path := filepath.Join(book.Filepath, "file1.epub")
	require.NoError(t, os.WriteFile(file1Path, []byte("content1"), 0644))
	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      file1Path,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2Path := filepath.Join(book.Filepath, "file2.epub")
	require.NoError(t, os.WriteFile(file2Path, []byte("content2"), 0644))
	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      file2Path,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Verify file1 is primary
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID)

	// Delete the primary file through DeleteFileAndCleanup (production code path)
	supportedTypes := map[string]struct{}{models.FileTypeEPUB: {}}
	result, err := svc.DeleteFileAndCleanup(ctx, file1.ID, library, supportedTypes, nil)
	require.NoError(t, err)
	assert.False(t, result.BookDeleted)

	// Refetch the book via RetrieveBook (exactly like the handler does after delete)
	updatedBook, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &book.ID})
	require.NoError(t, err)
	require.NotNil(t, updatedBook.PrimaryFileID, "PrimaryFileID should not be nil after primary deletion")
	assert.Equal(t, file2.ID, *updatedBook.PrimaryFileID, "file2 should be promoted to primary")
}
