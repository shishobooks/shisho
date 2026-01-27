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
	"github.com/uptrace/bun"
)

// setupTestLibrary creates a library for testing.
func setupTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	ctx := context.Background()

	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
		OrganizeFileStructure:    false,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	return library
}

// setupTestBookWithFile creates a book with a file in a temp directory.
func setupTestBookWithFile(t *testing.T, db *bun.DB, library *models.Library, title string) (*models.Book, *models.File) {
	t.Helper()
	ctx := context.Background()

	// Create a temporary directory structure for the book
	bookDir := t.TempDir()

	// Create the book record
	now := time.Now()
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           title,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        bookDir,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_, err := db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create a test file on disk
	filePath := filepath.Join(bookDir, title+".epub")
	err = os.WriteFile(filePath, []byte("test epub content"), 0644)
	require.NoError(t, err)

	// Create the file record
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: 17, // Length of "test epub content"
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	return book, file
}

func TestMoveFilesToBook_Basic(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Setup: library without organize file structure (simpler test)
	library := setupTestLibrary(t, db)

	// Create source book with 2 files
	sourceBook, sourceFile1 := setupTestBookWithFile(t, db, library, "Source Book")

	// Create a second file for the source book
	sourceFile2Path := filepath.Join(filepath.Dir(sourceFile1.Filepath), "Source Book 2.epub")
	err := os.WriteFile(sourceFile2Path, []byte("test epub content 2"), 0644)
	require.NoError(t, err)

	sourceFile2 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile2Path,
		FilesizeBytes: 19,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	_, err = db.NewInsert().Model(sourceFile2).Exec(ctx)
	require.NoError(t, err)

	// Create target book
	targetBook, _ := setupTestBookWithFile(t, db, library, "Target Book")

	// Act: Move one file from source to target
	svc := NewService(db)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceFile1.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, targetBook.ID, result.TargetBook.ID)
	assert.Equal(t, 1, result.FilesMoved)
	assert.False(t, result.SourceBookDeleted)
	assert.Empty(t, result.DeletedBookIDs)
	assert.False(t, result.NewBookCreated)

	// Verify the file is now on the target book
	var movedFile models.File
	err = db.NewSelect().Model(&movedFile).Where("id = ?", sourceFile1.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, targetBook.ID, movedFile.BookID)

	// Verify source book still exists with one file
	var remainingFiles []models.File
	err = db.NewSelect().Model(&remainingFiles).Where("book_id = ?", sourceBook.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, remainingFiles, 1)
	assert.Equal(t, sourceFile2.ID, remainingFiles[0].ID)
}

func TestMoveFilesToBook_SourceBookDeleted(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Setup: library without organize file structure
	library := setupTestLibrary(t, db)

	// Create source book with 1 file
	sourceBook, sourceFile := setupTestBookWithFile(t, db, library, "Source Book")

	// Create target book
	targetBook, _ := setupTestBookWithFile(t, db, library, "Target Book")

	// Act: Move the only file from source to target
	svc := NewService(db)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceFile.ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, targetBook.ID, result.TargetBook.ID)
	assert.Equal(t, 1, result.FilesMoved)
	assert.True(t, result.SourceBookDeleted)
	assert.Contains(t, result.DeletedBookIDs, sourceBook.ID)
	assert.False(t, result.NewBookCreated)

	// Verify source book is deleted
	var deletedBook models.Book
	err = db.NewSelect().Model(&deletedBook).Where("id = ?", sourceBook.ID).Scan(ctx)
	assert.Error(t, err) // Should not find the book
}

func TestMoveFilesToBook_CreateNewBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Setup: library without organize file structure
	library := setupTestLibrary(t, db)

	// Create a library path for the library
	libraryPath := t.TempDir()
	lp := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  libraryPath,
	}
	_, err := db.NewInsert().Model(lp).Exec(ctx)
	require.NoError(t, err)

	// Create source book with a file
	sourceBook, sourceFile1 := setupTestBookWithFile(t, db, library, "Source Book")

	// Create a second file in a DIFFERENT subdirectory
	// This simulates a scenario where we're splitting a file that's in its own subdirectory
	newFileDir := filepath.Join(libraryPath, "New Book Dir")
	err = os.MkdirAll(newFileDir, 0755)
	require.NoError(t, err)

	sourceFile2Path := filepath.Join(newFileDir, "New Book.epub")
	err = os.WriteFile(sourceFile2Path, []byte("test epub content 2"), 0644)
	require.NoError(t, err)

	sourceFile2 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile2Path,
		FilesizeBytes: 19,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	_, err = db.NewInsert().Model(sourceFile2).Exec(ctx)
	require.NoError(t, err)

	// Act: Move the second file (which is in a different directory) to a new book
	svc := NewService(db)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceFile2.ID},
		TargetBookID: nil, // Create new book
		LibraryID:    library.ID,
	})

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result.TargetBook)
	assert.NotEqual(t, sourceBook.ID, result.TargetBook.ID)
	assert.Equal(t, 1, result.FilesMoved)
	assert.False(t, result.SourceBookDeleted)
	assert.Empty(t, result.DeletedBookIDs)
	assert.True(t, result.NewBookCreated)

	// Verify the file is on the new book
	var movedFile models.File
	err = db.NewSelect().Model(&movedFile).Where("id = ?", sourceFile2.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Equal(t, result.TargetBook.ID, movedFile.BookID)

	// Verify the new book was created with appropriate defaults
	// Title should be derived from filename (without extension), not directory name
	assert.Equal(t, library.ID, result.TargetBook.LibraryID)
	assert.Equal(t, newFileDir, result.TargetBook.Filepath)
	assert.Equal(t, "New Book", result.TargetBook.Title)

	// Verify source book still exists with the remaining file
	var remainingFiles []models.File
	err = db.NewSelect().Model(&remainingFiles).Where("book_id = ?", sourceBook.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Len(t, remainingFiles, 1)
	assert.Equal(t, sourceFile1.ID, remainingFiles[0].ID)
}

func TestMoveFilesToBook_CreateNewBook_UsesFileMetadataName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Setup: library without organize file structure
	library := setupTestLibrary(t, db)

	// Create a library path for the library
	libraryPath := t.TempDir()
	lp := &models.LibraryPath{
		LibraryID: library.ID,
		Filepath:  libraryPath,
	}
	_, err := db.NewInsert().Model(lp).Exec(ctx)
	require.NoError(t, err)

	// Create source book with a file
	sourceBook, _ := setupTestBookWithFile(t, db, library, "Source Book")

	// Create a second file in a DIFFERENT subdirectory with a metadata name
	newFileDir := filepath.Join(libraryPath, "New Book Dir")
	err = os.MkdirAll(newFileDir, 0755)
	require.NoError(t, err)

	sourceFile2Path := filepath.Join(newFileDir, "some_filename_on_disk.epub")
	err = os.WriteFile(sourceFile2Path, []byte("test epub content 2"), 0644)
	require.NoError(t, err)

	// Set the file's metadata name, which should be used as the book title
	metadataName := "The Actual Book Title From Metadata"
	nameSource := models.DataSourceFileMetadata
	sourceFile2 := &models.File{
		LibraryID:     library.ID,
		BookID:        sourceBook.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      sourceFile2Path,
		FilesizeBytes: 19,
		Name:          &metadataName,
		NameSource:    &nameSource,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	_, err = db.NewInsert().Model(sourceFile2).Exec(ctx)
	require.NoError(t, err)

	// Act: Move the file to a new book
	svc := NewService(db)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceFile2.ID},
		TargetBookID: nil, // Create new book
		LibraryID:    library.ID,
	})

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, result.TargetBook)
	assert.True(t, result.NewBookCreated)

	// Verify the new book title comes from file.Name, NOT the filename on disk
	assert.Equal(t, metadataName, result.TargetBook.Title)
	assert.Equal(t, models.DataSourceFileMetadata, result.TargetBook.TitleSource)
}

func TestMoveFilesToBook_DifferentLibraries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create two libraries
	lib1 := &models.Library{
		Name:                     "Library 1",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib1).Exec(ctx)
	require.NoError(t, err)

	lib2 := &models.Library{
		Name:                     "Library 2",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err = db.NewInsert().Model(lib2).Exec(ctx)
	require.NoError(t, err)

	// Create books in different libraries
	book1, file1 := setupTestBookWithFile(t, db, lib1, "Book 1")
	_ = book1 // book1 is only used to create file1

	book2, _ := setupTestBookWithFile(t, db, lib2, "Book 2")

	// Try to move file to book in different library - should fail
	// The file is in lib1, but we're passing lib2's ID as the LibraryID
	_, err = svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file1.ID},
		TargetBookID: &book2.ID,
		LibraryID:    lib2.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMoveFilesToBook_MoveToSameBook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	library := setupTestLibrary(t, db)
	book, file := setupTestBookWithFile(t, db, library, "Test Book")

	// Try to move file to same book - should succeed but move 0 files
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{file.ID},
		TargetBookID: &book.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.FilesMoved, "should move 0 files when file is already on target book")
	assert.False(t, result.SourceBookDeleted)
	assert.False(t, result.NewBookCreated)
}

func TestMoveFilesToBook_FileNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	library := setupTestLibrary(t, db)
	targetBook, _ := setupTestBookWithFile(t, db, library, "Target Book")

	// Try to move non-existent file
	_, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{99999},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestMoveFilesToBook_NoFilesSelected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	_, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:   []int{},
		LibraryID: 1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no files")
}
