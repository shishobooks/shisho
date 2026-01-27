package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveFilesToBook_WithOrganizeFileStructure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directories
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	// Create test file
	sourceFile := filepath.Join(sourceDir, "test.epub")
	require.NoError(t, os.WriteFile(sourceFile, []byte("test content"), 0644))

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  tmpDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create source book
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Source Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Source Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	// Create file record for source book
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile,
		FilesizeBytes: 12, // Length of "test content"
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Target Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Target Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        targetDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(targetBook).Exec(ctx)
	require.NoError(t, err)

	// Move file
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)

	// Verify file was physically moved
	newFilePath := filepath.Join(targetDir, "test.epub")
	_, err = os.Stat(newFilePath)
	require.NoError(t, err, "file should exist at new location")

	_, err = os.Stat(sourceFile)
	assert.True(t, os.IsNotExist(err), "file should not exist at old location")

	// Verify database record updated
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, newFilePath, movedFile.Filepath)
	assert.Equal(t, targetBook.ID, movedFile.BookID)
}

func TestMoveFilesToBook_WithoutOrganizeFileStructure_NoPhysicalMove(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directories
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	// Create test file
	sourceFile := filepath.Join(sourceDir, "test.epub")
	require.NoError(t, os.WriteFile(sourceFile, []byte("test content"), 0644))

	// Create library with OrganizeFileStructure DISABLED
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    false,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create source book
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Source Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Source Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	// Create file record for source book
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile,
		FilesizeBytes: 12,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Target Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Target Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        targetDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(targetBook).Exec(ctx)
	require.NoError(t, err)

	// Move file
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)

	// Verify file was NOT physically moved (OrganizeFileStructure is disabled)
	_, err = os.Stat(sourceFile)
	require.NoError(t, err, "file should still exist at original location")

	// Verify database record updated (book_id changed, but filepath unchanged)
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, sourceFile, movedFile.Filepath, "filepath should be unchanged")
	assert.Equal(t, targetBook.ID, movedFile.BookID, "book_id should be updated")
}

func TestMoveFilesToBook_WithOrganizeFileStructure_HandlesDuplicateFilename(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directories
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	// Create source file
	sourceFile := filepath.Join(sourceDir, "book.epub")
	require.NoError(t, os.WriteFile(sourceFile, []byte("source content"), 0644))

	// Create existing file in target with same name
	existingFile := filepath.Join(targetDir, "book.epub")
	require.NoError(t, os.WriteFile(existingFile, []byte("existing content"), 0644))

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  tmpDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create source book with file
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Source Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Source Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile,
		FilesizeBytes: 14,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create target book with existing file
	targetBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Target Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Target Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        targetDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(targetBook).Exec(ctx)
	require.NoError(t, err)

	existingFileRecord := &models.File{
		LibraryID:     library.ID,
		BookID:        targetBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      existingFile,
		FilesizeBytes: 16,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(existingFileRecord).Exec(ctx)
	require.NoError(t, err)

	// Move file
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)

	// Verify the file was moved with a unique name (not overwriting existing)
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)

	// The moved file should have a different path than the existing file
	assert.NotEqual(t, existingFile, movedFile.Filepath, "moved file should have unique path")
	assert.Contains(t, movedFile.Filepath, targetDir, "moved file should be in target directory")

	// Both files should exist
	_, err = os.Stat(existingFile)
	require.NoError(t, err, "existing file should still exist")

	_, err = os.Stat(movedFile.Filepath)
	require.NoError(t, err, "moved file should exist at new location")

	// Original should not exist
	_, err = os.Stat(sourceFile)
	assert.True(t, os.IsNotExist(err), "source file should not exist at old location")
}

func TestMoveFilesToBook_MovesAssociatedFilesAndCleansUpDirectory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directories
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	// Create test file and associated files
	sourceFile := filepath.Join(sourceDir, "book.m4b")
	sourceCover := filepath.Join(sourceDir, "book.m4b.cover.jpg")
	sourceFileSidecar := filepath.Join(sourceDir, "book.m4b.metadata.json")
	sourceBookSidecar := filepath.Join(sourceDir, "source.metadata.json") // book sidecar uses directory name
	require.NoError(t, os.WriteFile(sourceFile, []byte("audiobook content"), 0644))
	require.NoError(t, os.WriteFile(sourceCover, []byte("cover image"), 0644))
	require.NoError(t, os.WriteFile(sourceFileSidecar, []byte(`{"version":1}`), 0644))
	require.NoError(t, os.WriteFile(sourceBookSidecar, []byte(`{"version":1,"title":"Source Book"}`), 0644))

	coverPath := "book.m4b.cover.jpg"

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  tmpDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create source book
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Source Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Source Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	// Create file record with cover path
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             sourceBook.ID,
		FileType:           models.FileTypeM4B,
		FileRole:           models.FileRoleMain,
		Filepath:           sourceFile,
		FilesizeBytes:      17,
		CoverImageFilename: &coverPath,
		CoverMimeType:      ptrString("image/jpeg"),
		CoverSource:        ptrString(models.DataSourceM4BMetadata),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Target Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Target Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        targetDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(targetBook).Exec(ctx)
	require.NoError(t, err)

	// Move file
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)

	// Verify main file was moved
	newFilePath := filepath.Join(targetDir, "book.m4b")
	_, err = os.Stat(newFilePath)
	require.NoError(t, err, "main file should exist at new location")

	// Verify cover was moved
	newCoverPath := filepath.Join(targetDir, "book.m4b.cover.jpg")
	_, err = os.Stat(newCoverPath)
	require.NoError(t, err, "cover should exist at new location")

	// Verify file sidecar was moved
	newFileSidecarPath := filepath.Join(targetDir, "book.m4b.metadata.json")
	_, err = os.Stat(newFileSidecarPath)
	require.NoError(t, err, "file sidecar should exist at new location")

	// Verify original files are gone
	_, err = os.Stat(sourceFile)
	assert.True(t, os.IsNotExist(err), "main file should not exist at old location")
	_, err = os.Stat(sourceCover)
	assert.True(t, os.IsNotExist(err), "cover should not exist at old location")
	_, err = os.Stat(sourceFileSidecar)
	assert.True(t, os.IsNotExist(err), "file sidecar should not exist at old location")

	// Verify book sidecar was deleted (not moved - it belongs to deleted source book)
	_, err = os.Stat(sourceBookSidecar)
	assert.True(t, os.IsNotExist(err), "book sidecar should be deleted with source book")

	// Verify book sidecar was NOT moved to target (it's book-level, not file-level)
	targetBookSidecar := filepath.Join(targetDir, "source.metadata.json")
	_, err = os.Stat(targetBookSidecar)
	assert.True(t, os.IsNotExist(err), "book sidecar should NOT be moved to target")

	// Verify source directory was cleaned up (should be deleted since it's now empty)
	_, err = os.Stat(sourceDir)
	assert.True(t, os.IsNotExist(err), "empty source directory should be cleaned up")

	// Verify database cover path was updated
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file.ID})
	require.NoError(t, err)
	assert.Equal(t, newFilePath, movedFile.Filepath)
	require.NotNil(t, movedFile.CoverImageFilename)
	// The cover path should be the relative filename (not full path)
	// This matches how cover paths are stored in the database and used by handlers
	assert.Equal(t, "book.m4b.cover.jpg", *movedFile.CoverImageFilename)
}

func TestMoveFilesToBook_MovesSupplementFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directory structure
	tempDir, err := os.MkdirTemp("", "merge-supplement-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	libraryPath := filepath.Join(tempDir, "library")
	sourceDir := filepath.Join(libraryPath, "source")
	targetDir := filepath.Join(libraryPath, "target")
	err = os.MkdirAll(sourceDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	// Create main file
	mainFile := filepath.Join(sourceDir, "book.m4b")
	err = os.WriteFile(mainFile, []byte("main audio content"), 0600)
	require.NoError(t, err)

	// Create supplement file (e.g., a PDF companion)
	supplementFile := filepath.Join(sourceDir, "companion.pdf")
	err = os.WriteFile(supplementFile, []byte("PDF content"), 0600)
	require.NoError(t, err)

	now := time.Now()

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	_, err = db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPathRecord := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  libraryPath,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.NewInsert().Model(libraryPathRecord).Exec(ctx)
	require.NoError(t, err)

	// Create source book
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Source Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Source Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	// Create main file record
	mainFileRecord := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		Filepath:      mainFile,
		FilesizeBytes: 18,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(mainFileRecord).Exec(ctx)
	require.NoError(t, err)

	// Create supplement file record
	supplementRecord := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      "pdf", // or whatever type supplements are
		FileRole:      models.FileRoleSupplement,
		Filepath:      supplementFile,
		FilesizeBytes: 11,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(supplementRecord).Exec(ctx)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "Target Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Target Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        targetDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(targetBook).Exec(ctx)
	require.NoError(t, err)

	// Move both files (main + supplement)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{mainFileRecord.ID, supplementRecord.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 2, result.FilesMoved, "both main and supplement files should be moved")

	// Verify main file was moved
	newMainPath := filepath.Join(targetDir, "book.m4b")
	_, err = os.Stat(newMainPath)
	require.NoError(t, err, "main file should exist at new location")

	// Verify supplement file was moved
	newSupplementPath := filepath.Join(targetDir, "companion.pdf")
	_, err = os.Stat(newSupplementPath)
	require.NoError(t, err, "supplement file should exist at new location")

	// Verify original files are gone
	_, err = os.Stat(mainFile)
	assert.True(t, os.IsNotExist(err), "main file should not exist at old location")
	_, err = os.Stat(supplementFile)
	assert.True(t, os.IsNotExist(err), "supplement file should not exist at old location")

	// Verify database records were updated
	movedMain, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &mainFileRecord.ID})
	require.NoError(t, err)
	assert.Equal(t, newMainPath, movedMain.Filepath)
	assert.Equal(t, targetBook.ID, movedMain.BookID)

	movedSupplement, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &supplementRecord.ID})
	require.NoError(t, err)
	assert.Equal(t, newSupplementPath, movedSupplement.Filepath)
	assert.Equal(t, targetBook.ID, movedSupplement.BookID)

	// Verify source directory was cleaned up
	_, err = os.Stat(sourceDir)
	assert.True(t, os.IsNotExist(err), "empty source directory should be cleaned up")
}

func ptrString(s string) *string {
	return &s
}

func TestMoveFilesToBook_CreateNewBook_WithOrganizeFileStructure_GeneratesUniqueDirectory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directory structure
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "BookDir")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))

	// Create test files in the same directory
	file1Path := filepath.Join(sourceDir, "book1.epub")
	file2Path := filepath.Join(sourceDir, "book2.epub")
	require.NoError(t, os.WriteFile(file1Path, []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(file2Path, []byte("content2"), 0644))

	// Create library with OrganizeFileStructure enabled
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  tmpDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create source book with two files
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "BookDir",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "BookDir",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	// Create an author for the source book (needed for folder naming)
	person := &models.Person{
		LibraryID: library.ID,
		Name:      "Test Author",
		SortName:  "Author, Test",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	author := &models.Author{
		BookID:    sourceBook.ID,
		PersonID:  person.ID,
		SortOrder: 1,
		Role:      nil, // nil for generic author
	}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file1Path,
		FilesizeBytes: 8,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file2Path,
		FilesizeBytes: 8,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Move file2 to a NEW book (TargetBookID = nil)
	// This should create a new book in a unique directory using [Author] Title format
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file2.ID},
		TargetBookID: nil, // Create new book
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.True(t, result.NewBookCreated, "a new book should be created")
	assert.Equal(t, 1, result.FilesMoved)

	// The new book should have a directory using [Author] Title format
	// Title is derived from filename "book2.epub" -> "book2"
	expectedNewBookDir := filepath.Join(tmpDir, "[Test Author] book2")
	assert.Equal(t, expectedNewBookDir, result.TargetBook.Filepath, "new book should have [Author] Title format directory")

	// The file should be moved to the new directory
	expectedNewFilePath := filepath.Join(expectedNewBookDir, "book2.epub")
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &file2.ID})
	require.NoError(t, err)
	assert.Equal(t, expectedNewFilePath, movedFile.Filepath)
	assert.Equal(t, result.TargetBook.ID, movedFile.BookID)

	// Verify the physical file was moved
	_, err = os.Stat(expectedNewFilePath)
	require.NoError(t, err, "file should exist at new location")

	// Original file should not exist
	_, err = os.Stat(file2Path)
	assert.True(t, os.IsNotExist(err), "file should not exist at old location")
}

func TestMoveFilesToBook_CreateNewBook_WithoutOrganizeFileStructure_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create temp directory
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "BookDir")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))

	// Create test files in the same directory
	file1Path := filepath.Join(sourceDir, "book1.epub")
	file2Path := filepath.Join(sourceDir, "book2.epub")
	require.NoError(t, os.WriteFile(file1Path, []byte("content1"), 0644))
	require.NoError(t, os.WriteFile(file2Path, []byte("content2"), 0644))

	// Create library with OrganizeFileStructure DISABLED
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    false,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create source book with two files
	now := time.Now()
	sourceBook := &models.Book{
		LibraryID:       library.ID,
		Title:           "BookDir",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "BookDir",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        sourceDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err = db.NewInsert().Model(sourceBook).Exec(ctx)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file1Path,
		FilesizeBytes: 8,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file2Path,
		FilesizeBytes: 8,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Try to move file2 to a NEW book (TargetBookID = nil)
	// This should fail because OrganizeFileStructure is disabled and a book
	// already exists at the file's directory
	_, err = svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file2.ID},
		TargetBookID: nil, // Create new book
		LibraryID:    library.ID,
	})
	require.Error(t, err, "should fail when creating new book without OrganizeFileStructure")
	assert.Contains(t, err.Error(), "cannot create a new book", "error should explain the reason")

	// Verify file was not moved
	_, err = os.Stat(file2Path)
	require.NoError(t, err, "file should still exist at original location")
}
