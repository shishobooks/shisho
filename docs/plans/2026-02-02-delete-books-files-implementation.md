# Delete Books and Files Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the ability to delete books and files from Shisho, including permanent deletion of files from disk.

**Architecture:** Three API endpoints (DELETE single book, DELETE single file, POST bulk delete). Service layer handles file system operations before DB commit. Frontend uses shared DeleteConfirmationDialog with collapsible details. Query invalidation on success.

**Tech Stack:** Go/Echo/Bun (backend), React/TypeScript/TanStack Query (frontend), SQLite with CASCADE deletes

---

## Task 1: Add DeleteBookAndFiles Service Method

**Files:**
- Modify: `pkg/books/service.go` (after line 1370)
- Test: `pkg/books/service_test.go`

This new method extends the existing `DeleteBook` to also delete files from disk.

**Step 1: Write the failing test**

Add to `pkg/books/service_test.go`:

```go
func TestDeleteBookAndFiles_DeletesBookFilesAndDiskFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library with OrganizeFileStructure=false (files at root level)
	library := &models.Library{
		Name:                  "Test Library",
		Path:                  t.TempDir(),
		OrganizeFileStructure: false,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID: library.ID,
		Title:     "Test Book",
		Filepath:  filepath.Join(library.Path, "test-book"),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create test files on disk
	mainFilePath := filepath.Join(library.Path, "test.epub")
	err = os.WriteFile(mainFilePath, []byte("test content"), 0644)
	require.NoError(t, err)

	coverPath := filepath.Join(library.Path, "test.cover.jpg")
	err = os.WriteFile(coverPath, []byte("cover content"), 0644)
	require.NoError(t, err)

	sidecarPath := filepath.Join(library.Path, "test.epub.metadata.json")
	err = os.WriteFile(sidecarPath, []byte("{}"), 0644)
	require.NoError(t, err)

	// Create file record
	file := &models.File{
		LibraryID:          library.ID,
		BookID:             book.ID,
		FileType:           models.FileTypeEPUB,
		FileRole:           models.FileRoleMain,
		Filepath:           mainFilePath,
		FilesizeBytes:      12,
		CoverImageFilename: &coverPath,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Call DeleteBookAndFiles
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteBookAndFiles(ctx, book.ID, library)
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesDeleted)

	// Verify book is deleted from DB
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file is deleted from DB
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify files are deleted from disk
	_, err = os.Stat(mainFilePath)
	assert.True(t, os.IsNotExist(err), "main file should be deleted")
	_, err = os.Stat(coverPath)
	assert.True(t, os.IsNotExist(err), "cover should be deleted")
	_, err = os.Stat(sidecarPath)
	assert.True(t, os.IsNotExist(err), "sidecar should be deleted")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestDeleteBookAndFiles_DeletesBookFilesAndDiskFiles ./pkg/books/...`
Expected: FAIL with "DeleteBookAndFiles not defined"

**Step 3: Write minimal implementation**

Add to `pkg/books/service.go` after line 1370:

```go
// DeleteBookAndFilesResult contains the results of deleting a book and its files.
type DeleteBookAndFilesResult struct {
	FilesDeleted int
}

// DeleteBookAndFiles deletes a book and all its files from both disk and database.
// If library.OrganizeFileStructure is true, the entire book directory is deleted.
// Otherwise, each file is deleted individually with its cover and sidecar.
func (svc *Service) DeleteBookAndFiles(ctx context.Context, bookID int, library *models.Library) (*DeleteBookAndFilesResult, error) {
	result := &DeleteBookAndFilesResult{}

	// Load book with files
	var book models.Book
	err := svc.db.NewSelect().
		Model(&book).
		Relation("Files").
		Where("book.id = ?", bookID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result.FilesDeleted = len(book.Files)

	// Delete files from disk first (before DB transaction)
	if library.OrganizeFileStructure && book.Filepath != "" {
		// Organized structure: delete entire book directory
		if err := os.RemoveAll(book.Filepath); err != nil && !os.IsNotExist(err) {
			return nil, errors.Wrap(err, "failed to delete book directory")
		}
	} else {
		// Root-level files: delete each file individually
		for _, file := range book.Files {
			if err := deleteFileFromDisk(file); err != nil {
				return nil, errors.Wrap(err, "failed to delete file from disk")
			}
		}
	}

	// Delete book from database (cascades to files and associations)
	if err := svc.DeleteBook(ctx, bookID); err != nil {
		return nil, err
	}

	return result, nil
}

// deleteFileFromDisk deletes a file and its associated cover and sidecar from disk.
func deleteFileFromDisk(file *models.File) error {
	// Delete the main file
	if err := os.Remove(file.Filepath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to delete main file")
	}

	// Delete cover image if exists
	if file.CoverImageFilename != nil {
		if err := os.Remove(*file.CoverImageFilename); err != nil && !os.IsNotExist(err) {
			// Log but don't fail - cover cleanup is best effort
		}
	}

	// Delete sidecar file if exists
	sidecarPath := file.Filepath + ".metadata.json"
	if err := os.Remove(sidecarPath); err != nil && !os.IsNotExist(err) {
		// Log but don't fail - sidecar cleanup is best effort
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeleteBookAndFiles_DeletesBookFilesAndDiskFiles ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add DeleteBookAndFiles service method

Deletes book and all its files from both disk and database.
Handles organized vs root-level file structures.
EOF
)"
```

---

## Task 2: Add DeleteFileAndCleanup Service Method

**Files:**
- Modify: `pkg/books/service.go`
- Test: `pkg/books/service_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/service_test.go`:

```go
func TestDeleteFileAndCleanup_DeletesFileAndKeepsBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name: "Test Library",
		Path: t.TempDir(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID: library.ID,
		Title:     "Test Book",
		Filepath:  library.Path,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files - one to delete, one to keep
	file1Path := filepath.Join(library.Path, "test1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)

	file1 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file1Path,
		FilesizeBytes: 8,
	}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(library.Path, "test2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      file2Path,
		FilesizeBytes: 8,
	}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Delete first file
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file1.ID, library)
	require.NoError(t, err)

	assert.False(t, result.BookDeleted, "book should not be deleted")

	// Verify file1 is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file1.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file1 is deleted from disk
	_, err = os.Stat(file1Path)
	assert.True(t, os.IsNotExist(err))

	// Verify book still exists
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify file2 still exists
	count, err = db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file2.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestDeleteFileAndCleanup_DeletesBookWhenLastFile(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name: "Test Library",
		Path: t.TempDir(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID: library.ID,
		Title:     "Test Book",
		Filepath:  library.Path,
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create single file
	filePath := filepath.Join(library.Path, "test.epub")
	err = os.WriteFile(filePath, []byte("content"), 0644)
	require.NoError(t, err)

	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filePath,
		FilesizeBytes: 7,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Delete the only file
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteFileAndCleanup(ctx, file.ID, library)
	require.NoError(t, err)

	assert.True(t, result.BookDeleted, "book should be deleted when last file removed")

	// Verify file is deleted from DB
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify book is deleted from DB
	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -run TestDeleteFileAndCleanup ./pkg/books/...`
Expected: FAIL with "DeleteFileAndCleanup not defined"

**Step 3: Write minimal implementation**

Add to `pkg/books/service.go`:

```go
// DeleteFileAndCleanupResult contains the results of deleting a file.
type DeleteFileAndCleanupResult struct {
	BookDeleted bool
	BookID      int
}

// DeleteFileAndCleanup deletes a file from disk and database.
// If this was the last file in the book, also deletes the book.
func (svc *Service) DeleteFileAndCleanup(ctx context.Context, fileID int, library *models.Library) (*DeleteFileAndCleanupResult, error) {
	result := &DeleteFileAndCleanupResult{}

	// Load file with book
	var file models.File
	err := svc.db.NewSelect().
		Model(&file).
		Relation("Book").
		Where("file.id = ?", fileID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	result.BookID = file.BookID

	// Delete file from disk
	if err := deleteFileFromDisk(&file); err != nil {
		return nil, err
	}

	// Delete file from database
	if err := svc.DeleteFile(ctx, fileID); err != nil {
		return nil, err
	}

	// Check if book has any remaining files
	count, err := svc.db.NewSelect().
		Model((*models.File)(nil)).
		Where("book_id = ?", file.BookID).
		Count(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if count == 0 {
		// No files remain, delete the book
		if err := svc.DeleteBook(ctx, file.BookID); err != nil {
			return nil, err
		}
		result.BookDeleted = true

		// Clean up empty book directory if organized structure
		if library.OrganizeFileStructure && file.Book != nil && file.Book.Filepath != "" {
			fileutils.CleanupEmptyDirectory(file.Book.Filepath)
		}
	}

	return result, nil
}
```

Add import at top of file:
```go
"github.com/shishobooks/shisho/pkg/fileutils"
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -run TestDeleteFileAndCleanup ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add DeleteFileAndCleanup service method

Deletes file from disk and database, and removes book when last file.
EOF
)"
```

---

## Task 3: Add DeleteBooksAndFiles Service Method (Bulk)

**Files:**
- Modify: `pkg/books/service.go`
- Test: `pkg/books/service_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/service_test.go`:

```go
func TestDeleteBooksAndFiles_DeletesMultipleBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	library := &models.Library{
		Name: "Test Library",
		Path: t.TempDir(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create two books with files
	book1 := &models.Book{LibraryID: library.ID, Title: "Book 1", Filepath: library.Path}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	book2 := &models.Book{LibraryID: library.ID, Title: "Book 2", Filepath: library.Path}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	// Create files
	file1Path := filepath.Join(library.Path, "book1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book1.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(library.Path, "book2a.epub")
	err = os.WriteFile(file2Path, []byte("content2a"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 9}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	file3Path := filepath.Join(library.Path, "book2b.epub")
	err = os.WriteFile(file3Path, []byte("content2b"), 0644)
	require.NoError(t, err)
	file3 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file3Path, FilesizeBytes: 9}
	_, err = db.NewInsert().Model(file3).Exec(ctx)
	require.NoError(t, err)

	// Delete both books
	bookSvc := NewService(db)
	result, err := bookSvc.DeleteBooksAndFiles(ctx, []int{book1.ID, book2.ID}, library)
	require.NoError(t, err)

	assert.Equal(t, 2, result.BooksDeleted)
	assert.Equal(t, 3, result.FilesDeleted)

	// Verify all books deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify all files deleted from disk
	_, err = os.Stat(file1Path)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(file2Path)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(file3Path)
	assert.True(t, os.IsNotExist(err))
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestDeleteBooksAndFiles_DeletesMultipleBooks ./pkg/books/...`
Expected: FAIL with "DeleteBooksAndFiles not defined"

**Step 3: Write minimal implementation**

Add to `pkg/books/service.go`:

```go
// DeleteBooksAndFilesResult contains the results of bulk book deletion.
type DeleteBooksAndFilesResult struct {
	BooksDeleted int
	FilesDeleted int
}

// DeleteBooksAndFiles deletes multiple books and all their files from disk and database.
func (svc *Service) DeleteBooksAndFiles(ctx context.Context, bookIDs []int, library *models.Library) (*DeleteBooksAndFilesResult, error) {
	result := &DeleteBooksAndFilesResult{}

	for _, bookID := range bookIDs {
		bookResult, err := svc.DeleteBookAndFiles(ctx, bookID, library)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to delete book %d", bookID)
		}
		result.BooksDeleted++
		result.FilesDeleted += bookResult.FilesDeleted
	}

	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeleteBooksAndFiles_DeletesMultipleBooks ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add DeleteBooksAndFiles bulk service method
EOF
)"
```

---

## Task 4: Add DELETE /books/:id Handler

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Test: `pkg/books/handlers_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/handlers_test.go`:

```go
func TestDeleteBook_DeletesBookAndFiles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	tmpDir := t.TempDir()
	library := &models.Library{
		Name: "Test Library",
		Path: tmpDir,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with admin permissions
	user := &models.User{Username: "admin", PasswordHash: "hash", Role: models.RoleAdmin}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Create book with file
	book := &models.Book{LibraryID: library.ID, Title: "Test Book", Filepath: tmpDir}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	filePath := filepath.Join(tmpDir, "test.epub")
	err = os.WriteFile(filePath, []byte("content"), 0644)
	require.NoError(t, err)

	file := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: filePath, FilesizeBytes: 7}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo and handler
	e := echo.New()
	e.Binder = binder.NewBinder()
	e.HTTPErrorHandler = errcodes.ErrorHandler
	cfg := &config.Config{}
	authMiddleware := auth.NewMiddleware(db)
	RegisterRoutesWithGroup(e.Group("/books"), db, cfg, authMiddleware, nil)

	// Make DELETE request
	req := httptest.NewRequest(http.MethodDelete, "/books/"+strconv.Itoa(book.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify book deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Verify file deleted from disk
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestDeleteBook_DeletesBookAndFiles ./pkg/books/...`
Expected: FAIL with 404 or method not allowed

**Step 3: Write minimal implementation**

Add to `pkg/books/handlers.go`:

```go
// DeleteBookResponse is the response for deleting a book.
type DeleteBookResponse struct {
	FilesDeleted int `json:"files_deleted"`
}

func (h *handler) deleteBook(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse book ID
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.ValidationError("Invalid book ID")
	}

	// Load book to get library ID
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}
	if book == nil {
		return errcodes.NotFound("Book")
	}

	// Load library for deletion config
	library, err := h.libraryService.GetLibrary(ctx, book.LibraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Delete book and files
	result, err := h.bookService.DeleteBookAndFiles(ctx, id, library)
	if err != nil {
		return errors.WithStack(err)
	}

	// Clean up search indexes
	if err := h.searchService.DeleteFromBookIndex(ctx, id); err != nil {
		// Log but don't fail
	}

	// Clean up orphaned entities
	h.personService.CleanupOrphanedPeople(ctx)
	h.bookService.CleanupOrphanedSeries(ctx)
	h.genreService.CleanupOrphanedGenres(ctx)
	h.tagService.CleanupOrphanedTags(ctx)

	return c.JSON(http.StatusOK, DeleteBookResponse{
		FilesDeleted: result.FilesDeleted,
	})
}
```

Add route in `pkg/books/routes.go` after line 58 (after the GET /:id route):

```go
g.DELETE("/:id", h.deleteBook, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationDelete))
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeleteBook_DeletesBookAndFiles ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/books/handlers_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add DELETE /books/:id endpoint

Deletes book and all its files from disk and database.
EOF
)"
```

---

## Task 5: Add DELETE /books/files/:id Handler

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Test: `pkg/books/handlers_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/handlers_test.go`:

```go
func TestDeleteFile_DeletesFileAndKeepsBook(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	tmpDir := t.TempDir()
	library := &models.Library{Name: "Test Library", Path: tmpDir}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user
	user := &models.User{Username: "admin", PasswordHash: "hash", Role: models.RoleAdmin}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Create book with two files
	book := &models.Book{LibraryID: library.ID, Title: "Test Book", Filepath: tmpDir}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file1Path := filepath.Join(tmpDir, "test1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(tmpDir, "test2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo
	e := echo.New()
	e.Binder = binder.NewBinder()
	e.HTTPErrorHandler = errcodes.ErrorHandler
	cfg := &config.Config{}
	authMiddleware := auth.NewMiddleware(db)
	RegisterRoutesWithGroup(e.Group("/books"), db, cfg, authMiddleware, nil)

	// Delete first file
	req := httptest.NewRequest(http.MethodDelete, "/books/files/"+strconv.Itoa(file1.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response
	var resp struct {
		BookDeleted bool `json:"book_deleted"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.False(t, resp.BookDeleted)

	// Verify file1 deleted, book and file2 remain
	count, err := db.NewSelect().Model((*models.File)(nil)).Where("id = ?", file1.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	count, err = db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", book.ID).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
```

Add import to test file:
```go
"encoding/json"
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestDeleteFile_DeletesFileAndKeepsBook ./pkg/books/...`
Expected: FAIL with 404 or method not allowed

**Step 3: Write minimal implementation**

Add to `pkg/books/handlers.go`:

```go
// DeleteFileResponse is the response for deleting a file.
type DeleteFileResponse struct {
	BookDeleted bool `json:"book_deleted"`
}

func (h *handler) deleteFile(c echo.Context) error {
	ctx := c.Request().Context()

	// Parse file ID
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.ValidationError("Invalid file ID")
	}

	// Load file to get library ID
	var file models.File
	err = h.bookService.db.NewSelect().
		Model(&file).
		Where("id = ?", id).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// Load library
	library, err := h.libraryService.GetLibrary(ctx, file.LibraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Delete file
	result, err := h.bookService.DeleteFileAndCleanup(ctx, id, library)
	if err != nil {
		return errors.WithStack(err)
	}

	// Clean up search indexes if book was deleted
	if result.BookDeleted {
		if err := h.searchService.DeleteFromBookIndex(ctx, result.BookID); err != nil {
			// Log but don't fail
		}
		h.personService.CleanupOrphanedPeople(ctx)
		h.bookService.CleanupOrphanedSeries(ctx)
		h.genreService.CleanupOrphanedGenres(ctx)
		h.tagService.CleanupOrphanedTags(ctx)
	}

	return c.JSON(http.StatusOK, DeleteFileResponse{
		BookDeleted: result.BookDeleted,
	})
}
```

Add route in `pkg/books/routes.go` after the file routes (around line 76):

```go
g.DELETE("/files/:id", h.deleteFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationDelete))
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeleteFile_DeletesFileAndKeepsBook ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/books/handlers_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add DELETE /books/files/:id endpoint

Deletes file from disk and database, removes book when last file.
EOF
)"
```

---

## Task 6: Add POST /books/delete Handler (Bulk)

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Test: `pkg/books/handlers_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/handlers_test.go`:

```go
func TestDeleteBooks_BulkDeletesBooks(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	tmpDir := t.TempDir()
	library := &models.Library{Name: "Test Library", Path: tmpDir}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user
	user := &models.User{Username: "admin", PasswordHash: "hash", Role: models.RoleAdmin}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Create two books with files
	book1 := &models.Book{LibraryID: library.ID, Title: "Book 1", Filepath: tmpDir}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	file1Path := filepath.Join(tmpDir, "book1.epub")
	err = os.WriteFile(file1Path, []byte("content1"), 0644)
	require.NoError(t, err)
	file1 := &models.File{LibraryID: library.ID, BookID: book1.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file1Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file1).Exec(ctx)
	require.NoError(t, err)

	book2 := &models.Book{LibraryID: library.ID, Title: "Book 2", Filepath: tmpDir}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	file2Path := filepath.Join(tmpDir, "book2.epub")
	err = os.WriteFile(file2Path, []byte("content2"), 0644)
	require.NoError(t, err)
	file2 := &models.File{LibraryID: library.ID, BookID: book2.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: file2Path, FilesizeBytes: 8}
	_, err = db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo
	e := echo.New()
	e.Binder = binder.NewBinder()
	e.HTTPErrorHandler = errcodes.ErrorHandler
	cfg := &config.Config{}
	authMiddleware := auth.NewMiddleware(db)
	RegisterRoutesWithGroup(e.Group("/books"), db, cfg, authMiddleware, nil)

	// Bulk delete
	body := `{"book_ids": [` + strconv.Itoa(book1.ID) + `, ` + strconv.Itoa(book2.ID) + `]}`
	req := httptest.NewRequest(http.MethodPost, "/books/delete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Parse response
	var resp struct {
		BooksDeleted int `json:"books_deleted"`
		FilesDeleted int `json:"files_deleted"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.BooksDeleted)
	assert.Equal(t, 2, resp.FilesDeleted)

	// Verify books deleted
	count, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
```

Add import:
```go
"strings"
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestDeleteBooks_BulkDeletesBooks ./pkg/books/...`
Expected: FAIL with 404

**Step 3: Write minimal implementation**

Add to `pkg/books/handlers.go`:

```go
// DeleteBooksRequest is the request body for bulk book deletion.
type DeleteBooksRequest struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1"`
}

// DeleteBooksResponse is the response for bulk book deletion.
type DeleteBooksResponse struct {
	BooksDeleted int `json:"books_deleted"`
	FilesDeleted int `json:"files_deleted"`
}

func (h *handler) deleteBooks(c echo.Context) error {
	ctx := c.Request().Context()

	var req DeleteBooksRequest
	if err := c.Bind(&req); err != nil {
		return errcodes.ValidationError("Invalid request body")
	}

	if len(req.BookIDs) == 0 {
		return errcodes.ValidationError("book_ids is required")
	}

	// Load first book to get library (all books must be in same library)
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &req.BookIDs[0]})
	if err != nil {
		return errors.WithStack(err)
	}
	if book == nil {
		return errcodes.NotFound("Book")
	}

	library, err := h.libraryService.GetLibrary(ctx, book.LibraryID)
	if err != nil {
		return errors.WithStack(err)
	}

	// Verify all books belong to same library
	for _, bookID := range req.BookIDs[1:] {
		b, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
		if err != nil {
			return errors.WithStack(err)
		}
		if b == nil {
			return errcodes.NotFound("Book")
		}
		if b.LibraryID != library.ID {
			return errcodes.ValidationError("All books must belong to the same library")
		}
	}

	// Delete books
	result, err := h.bookService.DeleteBooksAndFiles(ctx, req.BookIDs, library)
	if err != nil {
		return errors.WithStack(err)
	}

	// Clean up search indexes
	for _, bookID := range req.BookIDs {
		h.searchService.DeleteFromBookIndex(ctx, bookID)
	}
	h.personService.CleanupOrphanedPeople(ctx)
	h.bookService.CleanupOrphanedSeries(ctx)
	h.genreService.CleanupOrphanedGenres(ctx)
	h.tagService.CleanupOrphanedTags(ctx)

	return c.JSON(http.StatusOK, DeleteBooksResponse{
		BooksDeleted: result.BooksDeleted,
		FilesDeleted: result.FilesDeleted,
	})
}
```

Add route in `pkg/books/routes.go` after line 54 (before /:id routes):

```go
g.POST("/delete", h.deleteBooks, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationDelete))
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestDeleteBooks_BulkDeletesBooks ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/books/handlers_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add POST /books/delete bulk endpoint

Deletes multiple books and all their files atomically.
EOF
)"
```

---

## Task 7: Add TypeScript Types for Delete Operations

**Files:**
- Modify: `app/types/index.ts`

**Step 1: Add types**

Add to `app/types/index.ts` in the appropriate sections:

```typescript
// Delete book response
export interface DeleteBookResponse {
  files_deleted: number;
}

// Delete file response
export interface DeleteFileResponse {
  book_deleted: boolean;
}

// Delete books request/response (bulk)
export interface DeleteBooksPayload {
  book_ids: number[];
}

export interface DeleteBooksResponse {
  books_deleted: number;
  files_deleted: number;
}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 3: Commit**

```bash
git add app/types/index.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add TypeScript types for delete operations
EOF
)"
```

---

## Task 8: Add Delete Mutation Hooks

**Files:**
- Modify: `app/hooks/queries/books.ts`

**Step 1: Add mutations**

Add to `app/hooks/queries/books.ts`:

```typescript
import { QueryKey as SearchQueryKey } from "./search";

// ... existing imports and code ...

// Delete book mutation
export const useDeleteBook = () => {
  const queryClient = useQueryClient();

  return useMutation<DeleteBookResponse, ShishoAPIError, number>({
    mutationFn: (bookId) => {
      return API.request("DELETE", `/books/${bookId}`, null, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({ queryKey: [SearchQueryKey.GlobalSearch] });
    },
  });
};

// Delete file mutation
export const useDeleteFile = () => {
  const queryClient = useQueryClient();

  return useMutation<DeleteFileResponse, ShishoAPIError, number>({
    mutationFn: (fileId) => {
      return API.request("DELETE", `/books/files/${fileId}`, null, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({ queryKey: [SearchQueryKey.GlobalSearch] });
    },
  });
};

// Bulk delete books mutation
export const useDeleteBooks = () => {
  const queryClient = useQueryClient();

  return useMutation<
    DeleteBooksResponse,
    ShishoAPIError,
    DeleteBooksPayload
  >({
    mutationFn: (payload) => {
      return API.request("POST", "/books/delete", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({ queryKey: [SearchQueryKey.GlobalSearch] });
    },
  });
};
```

Add imports at top:
```typescript
import type {
  // ... existing imports ...
  DeleteBookResponse,
  DeleteBooksPayload,
  DeleteBooksResponse,
  DeleteFileResponse,
} from "@/types";
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 3: Commit**

```bash
git add app/hooks/queries/books.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add delete mutation hooks for books and files
EOF
)"
```

---

## Task 9: Create DeleteConfirmationDialog Component

**Files:**
- Create: `app/components/library/DeleteConfirmationDialog.tsx`

**Step 1: Create the component**

Create `app/components/library/DeleteConfirmationDialog.tsx`:

```typescript
import { AlertTriangle, ChevronDown, ChevronRight, Loader2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { Book, File } from "@/types";

type DeleteVariant = "book" | "books" | "file";

interface DeleteConfirmationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  variant: DeleteVariant;
  // For single book/file delete
  title?: string;
  files?: File[];
  // For bulk delete
  books?: Pick<Book, "id" | "title" | "files">[];
  onConfirm: () => void;
  isPending: boolean;
}

export function DeleteConfirmationDialog({
  open,
  onOpenChange,
  variant,
  title,
  files,
  books,
  onConfirm,
  isPending,
}: DeleteConfirmationDialogProps) {
  const [showDetails, setShowDetails] = useState(false);

  const getDialogTitle = () => {
    switch (variant) {
      case "book":
        return "Delete Book";
      case "books":
        return "Delete Books";
      case "file":
        return "Delete File";
    }
  };

  const getSummary = () => {
    switch (variant) {
      case "book":
        return (
          <>
            <span className="font-medium break-all">"{title}"</span>
            {files && files.length > 0 && (
              <span className="text-muted-foreground">
                {" "}
                ({files.length} file{files.length !== 1 ? "s" : ""})
              </span>
            )}
          </>
        );
      case "books": {
        const totalFiles =
          books?.reduce((sum, b) => sum + (b.files?.length ?? 0), 0) ?? 0;
        return (
          <>
            {books?.length} book{books?.length !== 1 ? "s" : ""} ({totalFiles}{" "}
            file{totalFiles !== 1 ? "s" : ""} total)
          </>
        );
      }
      case "file":
        return <span className="font-medium break-all">"{title}"</span>;
    }
  };

  const renderDetails = () => {
    if (variant === "book" && files) {
      return (
        <ul className="text-sm space-y-1">
          {files.map((file) => (
            <li className="truncate" key={file.id}>
              {file.filepath.split("/").pop()}
            </li>
          ))}
        </ul>
      );
    }

    if (variant === "books" && books) {
      return (
        <ul className="text-sm space-y-2">
          {books.map((book) => (
            <li key={book.id}>
              <div className="font-medium truncate">{book.title}</div>
              {book.files && book.files.length > 0 && (
                <ul className="ml-4 mt-1 text-muted-foreground space-y-0.5">
                  {book.files.map((file) => (
                    <li className="truncate" key={file.id}>
                      {file.filepath.split("/").pop()}
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>
      );
    }

    return null;
  };

  const hasDetails =
    (variant === "book" && files && files.length > 0) ||
    (variant === "books" && books && books.length > 0);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-destructive shrink-0" />
            {getDialogTitle()}
          </DialogTitle>
          <DialogDescription className="sr-only">
            Confirm deletion of {variant === "books" ? "books" : variant}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Warning banner */}
          <div className="bg-destructive/10 border border-destructive/20 rounded-md p-3 text-sm text-destructive">
            This action cannot be undone. Files will be permanently deleted from
            disk.
          </div>

          {/* Summary */}
          <p className="text-sm">
            Are you sure you want to delete {getSummary()}?
          </p>

          {/* Expandable details */}
          {hasDetails && (
            <div>
              <button
                className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
                onClick={() => setShowDetails(!showDetails)}
                type="button"
              >
                {showDetails ? (
                  <ChevronDown className="h-4 w-4" />
                ) : (
                  <ChevronRight className="h-4 w-4" />
                )}
                {showDetails ? "Hide details" : "Show details"}
              </button>

              {showDetails && (
                <ScrollArea className="mt-2 max-h-48 rounded-md border p-3">
                  {renderDetails()}
                </ScrollArea>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm} variant="destructive">
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 3: Commit**

```bash
git add app/components/library/DeleteConfirmationDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add DeleteConfirmationDialog component

Shared dialog for book, file, and bulk delete confirmations.
EOF
)"
```

---

## Task 10: Add Delete Book to BookDetail Page

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add delete functionality**

Add imports at top of file:
```typescript
import { Trash2 } from "lucide-react";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import { useDeleteBook } from "@/hooks/queries/books";
```

Add state after existing state declarations (around line 625):
```typescript
const [showDeleteDialog, setShowDeleteDialog] = useState(false);
const deleteBookMutation = useDeleteBook();
```

Add delete handler after other handlers (around line 930):
```typescript
const handleDeleteBook = async () => {
  if (!id) return;
  try {
    await deleteBookMutation.mutateAsync(parseInt(id));
    toast.success("Book deleted");
    navigate("/");
  } catch (error) {
    toast.error(
      error instanceof Error ? error.message : "Failed to delete book",
    );
  }
};
```

Add delete menu item in the DropdownMenu after the "Merge into another book" item (around line 1070):
```typescript
<DropdownMenuSeparator />
<DropdownMenuItem
  className="text-destructive focus:text-destructive"
  onClick={() => setShowDeleteDialog(true)}
>
  <Trash2 className="h-4 w-4 mr-2" />
  Delete book
</DropdownMenuItem>
```

Add dialog before closing `</LibraryLayout>` at end of component:
```typescript
<DeleteConfirmationDialog
  files={book.files}
  isPending={deleteBookMutation.isPending}
  onConfirm={handleDeleteBook}
  onOpenChange={setShowDeleteDialog}
  open={showDeleteDialog}
  title={book.title}
  variant="book"
/>
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add delete book option to BookDetail page

Adds delete to the More menu with confirmation dialog.
EOF
)"
```

---

## Task 11: Add Delete File to BookDetail Page (File Row)

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add delete to FileRow**

Add to FileRowProps interface (around line 130):
```typescript
onDelete: () => void;
isDeleting: boolean;
```

In the FileRow component's DropdownMenu (inside the `<DropdownMenuContent>`), add after the last menu item:
```typescript
<DropdownMenuSeparator />
<DropdownMenuItem
  className="text-destructive focus:text-destructive"
  disabled={isDeleting}
  onClick={onDelete}
>
  <Trash2 className="h-4 w-4 mr-2" />
  Delete file
</DropdownMenuItem>
```

In the BookDetail component, add state:
```typescript
const [deletingFileId, setDeletingFileId] = useState<number | null>(null);
const [fileToDelete, setFileToDelete] = useState<File | null>(null);
const deleteFileMutation = useDeleteFile();
```

Add handler:
```typescript
const handleDeleteFile = async () => {
  if (!fileToDelete) return;
  setDeletingFileId(fileToDelete.id);
  try {
    const result = await deleteFileMutation.mutateAsync(fileToDelete.id);
    toast.success("File deleted");
    setFileToDelete(null);
    if (result.book_deleted) {
      navigate("/");
    }
  } catch (error) {
    toast.error(
      error instanceof Error ? error.message : "Failed to delete file",
    );
  } finally {
    setDeletingFileId(null);
  }
};
```

Add import for useDeleteFile:
```typescript
import { useDeleteBook, useDeleteFile } from "@/hooks/queries/books";
```

Update FileRow usage to pass the new props:
```typescript
onDelete={() => setFileToDelete(file)}
isDeleting={deletingFileId === file.id}
```

Add dialog for file delete:
```typescript
{fileToDelete && (
  <DeleteConfirmationDialog
    isPending={deleteFileMutation.isPending}
    onConfirm={handleDeleteFile}
    onOpenChange={(open) => !open && setFileToDelete(null)}
    open={!!fileToDelete}
    title={fileToDelete.filepath.split("/").pop() ?? "File"}
    variant="file"
  />
)}
```

**Step 2: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add delete file option to file rows

Adds delete to file action menu with confirmation dialog.
EOF
)"
```

---

## Task 12: Add Delete to SelectionToolbar (Bulk Delete)

**Files:**
- Modify: `app/components/library/SelectionToolbar.tsx`

**Step 1: Add delete functionality**

Add imports:
```typescript
import { Trash2 } from "lucide-react";
import { DeleteConfirmationDialog } from "@/components/library/DeleteConfirmationDialog";
import { useDeleteBooks } from "@/hooks/queries/books";
import { useBooks } from "@/hooks/queries/books";
```

Add state:
```typescript
const [showDeleteDialog, setShowDeleteDialog] = useState(false);
const deleteBooksMutation = useDeleteBooks();

// Fetch book details for selected books (needed for dialog)
const booksQuery = useBooks(
  { ids: selectedBookIds },
  { enabled: showDeleteDialog && selectedBookIds.length > 0 },
);
```

Add handler:
```typescript
const handleDeleteBooks = async () => {
  try {
    const result = await deleteBooksMutation.mutateAsync({
      book_ids: selectedBookIds,
    });
    toast.success(
      `Deleted ${result.books_deleted} book${result.books_deleted !== 1 ? "s" : ""}`,
    );
    setShowDeleteDialog(false);
    exitSelectionMode();
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Failed to delete books";
    toast.error(message);
  }
};
```

Add delete button after the Merge button:
```typescript
<Button
  onClick={() => setShowDeleteDialog(true)}
  size="sm"
  variant="destructive"
>
  <Trash2 className="h-4 w-4" />
  Delete
</Button>
```

Add dialog before closing `</div>`:
```typescript
<DeleteConfirmationDialog
  books={booksQuery.data?.books?.map((b) => ({
    id: b.id,
    title: b.title,
    files: b.files,
  }))}
  isPending={deleteBooksMutation.isPending}
  onConfirm={handleDeleteBooks}
  onOpenChange={setShowDeleteDialog}
  open={showDeleteDialog}
  variant="books"
/>
```

**Step 2: Add `ids` filter support to books query**

Update `app/hooks/queries/books.ts` to support filtering by IDs in ListBooksQuery.

Update `app/types/index.ts` - add to ListBooksQuery interface:
```typescript
ids?: number[];
```

**Step 3: Run TypeScript check**

Run: `yarn lint:types`
Expected: PASS

**Step 4: Commit**

```bash
git add app/components/library/SelectionToolbar.tsx app/hooks/queries/books.ts app/types/index.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add bulk delete to SelectionToolbar

Adds delete button for bulk book deletion with confirmation.
EOF
)"
```

---

## Task 13: Add Backend Support for IDs Filter in List Books

**Files:**
- Modify: `pkg/books/handlers.go`
- Test: `pkg/books/handlers_test.go`

**Step 1: Write the failing test**

Add to `pkg/books/handlers_test.go`:

```go
func TestListBooks_FiltersByIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create library
	library := &models.Library{Name: "Test Library", Path: t.TempDir()}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user
	user := &models.User{Username: "admin", PasswordHash: "hash", Role: models.RoleAdmin}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	// Create three books
	book1 := &models.Book{LibraryID: library.ID, Title: "Book 1", Filepath: library.Path}
	_, err = db.NewInsert().Model(book1).Exec(ctx)
	require.NoError(t, err)

	book2 := &models.Book{LibraryID: library.ID, Title: "Book 2", Filepath: library.Path}
	_, err = db.NewInsert().Model(book2).Exec(ctx)
	require.NoError(t, err)

	book3 := &models.Book{LibraryID: library.ID, Title: "Book 3", Filepath: library.Path}
	_, err = db.NewInsert().Model(book3).Exec(ctx)
	require.NoError(t, err)

	// Setup Echo
	e := echo.New()
	e.Binder = binder.NewBinder()
	e.HTTPErrorHandler = errcodes.ErrorHandler
	cfg := &config.Config{}
	authMiddleware := auth.NewMiddleware(db)
	RegisterRoutesWithGroup(e.Group("/books"), db, cfg, authMiddleware, nil)

	// Filter by IDs
	req := httptest.NewRequest(http.MethodGet, "/books?ids="+strconv.Itoa(book1.ID)+","+strconv.Itoa(book3.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Len(t, resp.Books, 2)
	assert.Equal(t, 2, resp.Total)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run TestListBooks_FiltersByIDs ./pkg/books/...`
Expected: FAIL - returns all 3 books

**Step 3: Update list handler to support IDs filter**

In `pkg/books/handlers.go`, find the `ListBooksQuery` struct and add:
```go
IDs []int `query:"ids"`
```

In the `list` handler, add the filter (after other filters):
```go
if len(q.IDs) > 0 {
    opts.IDs = q.IDs
}
```

In `pkg/books/service.go`, update `ListBooksOptions`:
```go
IDs []int
```

In `ListBooks` method, add the filter:
```go
if len(opts.IDs) > 0 {
    query = query.Where("book.id IN (?)", bun.In(opts.IDs))
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run TestListBooks_FiltersByIDs ./pkg/books/...`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/service.go pkg/books/handlers_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add IDs filter to list books endpoint

Allows filtering books by a list of IDs for bulk operations.
EOF
)"
```

---

## Task 14: Run Full Test Suite and Linting

**Step 1: Run backend tests**

Run: `make test`
Expected: All tests pass

**Step 2: Run backend linting**

Run: `make lint`
Expected: No lint errors

**Step 3: Run frontend linting**

Run: `yarn lint`
Expected: No lint errors

**Step 4: Fix any issues**

If any tests or linting fail, fix the issues and commit.

**Step 5: Final commit if fixes needed**

```bash
git add .
git commit -m "$(cat <<'EOF'
[Fix] Address test and lint issues in delete feature
EOF
)"
```

---

## Task 15: Manual Testing Checklist

Verify the feature works end-to-end:

1. **Delete single book from BookDetail page:**
   - [ ] Open a book detail page
   - [ ] Click More menu → Delete book
   - [ ] Dialog shows book title and file count
   - [ ] Expand "Show details" shows file list
   - [ ] Confirm deletion
   - [ ] Book and files are deleted from disk
   - [ ] Redirects to home page
   - [ ] Book no longer appears in library

2. **Delete file from BookDetail page:**
   - [ ] Open a book with multiple files
   - [ ] Click file action menu → Delete file
   - [ ] Confirm deletion
   - [ ] File is deleted, book remains
   - [ ] Delete last file of a book
   - [ ] Book is also deleted
   - [ ] Redirects to home page

3. **Bulk delete from SelectionToolbar:**
   - [ ] Select multiple books on home page
   - [ ] Click Delete button in toolbar
   - [ ] Dialog shows book count and total file count
   - [ ] Expand "Show details" shows all books and files
   - [ ] Confirm deletion
   - [ ] All selected books and files are deleted
   - [ ] Selection mode exits
   - [ ] Home page refreshes

4. **Error handling:**
   - [ ] Try deleting a book that doesn't exist (404)
   - [ ] Verify transaction rollback if file delete fails

---

## Summary

This plan implements the delete books and files feature with:

**Backend (Tasks 1-6, 13):**
- Service methods for deleting books, files, and bulk delete
- Handles both organized and root-level file structures
- Cleans up search indexes and orphaned entities
- Three API endpoints: DELETE /books/:id, DELETE /books/files/:id, POST /books/delete

**Frontend (Tasks 7-12):**
- TypeScript types and mutation hooks
- Shared DeleteConfirmationDialog component
- Delete option in BookDetail page menu
- Delete option in file row menus
- Bulk delete in SelectionToolbar

**Testing (Tasks 14-15):**
- Comprehensive unit tests for all service methods
- Handler tests for all endpoints
- Manual testing checklist
