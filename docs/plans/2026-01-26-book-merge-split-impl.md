# Book Merge and Split Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the ability to move files between books, enabling merge and split workflows.

**Architecture:** The core operation is `MoveFilesToBook` which handles moving files, updating paths when `OrganizeFileStructure` is enabled, cleaning up empty source books, and updating search indexes. Two API endpoints expose this: one for moving files from a specific book, another for bulk merging books.

**Tech Stack:** Go (Echo, Bun ORM), React 19, TypeScript, TailwindCSS, Tanstack Query

---

## Task 1: Backend Core - MoveFilesToBook Service Method

**Files:**
- Create: `pkg/books/merge.go`
- Modify: `pkg/books/service.go` (add method signature to interface if needed)

### Step 1: Write the failing test

Create `pkg/books/merge_test.go`:

```go
package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveFilesToBook_Basic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create source and target libraries
	library := &models.Library{Name: "Test Library", OrganizeFileStructure: false}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create source book with files
	sourceBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Source Book",
		Filepath:  "/tmp/source",
		Files: []*models.File{
			{LibraryID: library.ID, Filepath: "/tmp/source/file1.epub", FileType: "epub"},
			{LibraryID: library.ID, Filepath: "/tmp/source/file2.epub", FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, sourceBook)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Target Book",
		Filepath:  "/tmp/target",
	}
	err = svc.CreateBook(ctx, targetBook)
	require.NoError(t, err)

	// Move one file to target book
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceBook.Files[0].ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)
	assert.False(t, result.SourceBookDeleted)
	assert.Equal(t, targetBook.ID, result.TargetBook.ID)

	// Verify file was moved
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &sourceBook.Files[0].ID})
	require.NoError(t, err)
	assert.Equal(t, targetBook.ID, movedFile.BookID)

	// Verify source book still has one file
	updatedSource, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &sourceBook.ID})
	require.NoError(t, err)
	assert.Len(t, updatedSource.Files, 1)
}

func TestMoveFilesToBook_SourceBookDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create library
	library := &models.Library{Name: "Test Library", OrganizeFileStructure: false}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create source book with one file
	sourceBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Source Book",
		Filepath:  "/tmp/source",
		Files: []*models.File{
			{LibraryID: library.ID, Filepath: "/tmp/source/file1.epub", FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, sourceBook)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Target Book",
		Filepath:  "/tmp/target",
	}
	err = svc.CreateBook(ctx, targetBook)
	require.NoError(t, err)

	// Move all files - source should be deleted
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceBook.Files[0].ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)
	assert.True(t, result.SourceBookDeleted)
	assert.Contains(t, result.DeletedBookIDs, sourceBook.ID)

	// Verify source book is deleted
	_, err = svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &sourceBook.ID})
	assert.Error(t, err)
}

func TestMoveFilesToBook_CreateNewBook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create library
	library := &models.Library{Name: "Test Library", OrganizeFileStructure: false}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create source book with multiple files
	sourceBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Source Book",
		Filepath:  "/tmp/source",
		Files: []*models.File{
			{LibraryID: library.ID, Filepath: "/tmp/source/file1.epub", FileType: "epub"},
			{LibraryID: library.ID, Filepath: "/tmp/source/file2.epub", FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, sourceBook)
	require.NoError(t, err)

	// Move one file to new book (targetBookID = nil)
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceBook.Files[0].ID},
		TargetBookID: nil, // Create new book
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)
	assert.NotNil(t, result.TargetBook)
	assert.NotEqual(t, sourceBook.ID, result.TargetBook.ID)
	assert.True(t, result.NewBookCreated)

	// Verify new book was created with file's metadata
	newBook, err := svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &result.TargetBook.ID})
	require.NoError(t, err)
	assert.Len(t, newBook.Files, 1)
}
```

### Step 2: Run test to verify it fails

Run: `go test -v -race ./pkg/books/... -run TestMoveFilesToBook -count=1`
Expected: FAIL - `MoveFilesToBook` method not defined

### Step 3: Write the implementation

Create `pkg/books/merge.go`:

```go
package books

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortname"
	"github.com/uptrace/bun"
)

// MoveFilesOptions contains the parameters for moving files between books.
type MoveFilesOptions struct {
	FileIDs      []int
	TargetBookID *int // nil = create new book from first file's metadata
	LibraryID    int
}

// MoveFilesResult contains the result of a move files operation.
type MoveFilesResult struct {
	TargetBook        *models.Book
	FilesMoved        int
	SourceBookDeleted bool
	DeletedBookIDs    []int
	NewBookCreated    bool
}

// fileMove tracks a single file move for rollback purposes.
type fileMove struct {
	FileID       int
	OriginalPath string
	NewPath      string
}

// MoveFilesToBook moves files to a target book, optionally creating a new book.
// If targetBookID is nil, creates a new book from the first file's directory.
func (svc *Service) MoveFilesToBook(ctx context.Context, opts MoveFilesOptions) (*MoveFilesResult, error) {
	if len(opts.FileIDs) == 0 {
		return nil, errors.New("no files selected")
	}

	result := &MoveFilesResult{
		DeletedBookIDs: []int{},
	}

	// Fetch all files to move
	var files []*models.File
	err := svc.db.NewSelect().
		Model(&files).
		Where("id IN (?)", bun.In(opts.FileIDs)).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(files) != len(opts.FileIDs) {
		return nil, errors.New("one or more files not found")
	}

	// Validate all files are in the specified library
	sourceBookIDs := make(map[int]bool)
	for _, f := range files {
		if f.LibraryID != opts.LibraryID {
			return nil, errors.New("all files must be in the same library")
		}
		sourceBookIDs[f.BookID] = true
	}

	// Get or create target book
	var targetBook *models.Book
	if opts.TargetBookID != nil {
		targetBook, err = svc.RetrieveBook(ctx, RetrieveBookOptions{ID: opts.TargetBookID})
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if targetBook.LibraryID != opts.LibraryID {
			return nil, errors.New("target book must be in the same library")
		}
		// Check if files already belong to target book
		for _, f := range files {
			if f.BookID == *opts.TargetBookID {
				return nil, errors.New("files already belong to this book")
			}
		}
	} else {
		// Create new book from first file's directory
		targetBook, err = svc.createBookFromFile(ctx, files[0])
		if err != nil {
			return nil, errors.WithStack(err)
		}
		result.NewBookCreated = true
	}

	// Fetch library to check OrganizeFileStructure
	var library models.Library
	err = svc.db.NewSelect().
		Model(&library).
		Relation("LibraryPaths").
		Where("l.id = ?", opts.LibraryID).
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Track file moves for potential rollback
	var movedFiles []fileMove

	// Run in transaction
	err = svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		for _, file := range files {
			originalPath := file.Filepath
			newPath := originalPath

			// Move physical file if OrganizeFileStructure is enabled
			if library.OrganizeFileStructure {
				targetDir := targetBook.Filepath
				// If target book's filepath is a file (not directory), use its directory
				if filepath.Ext(targetDir) != "" {
					targetDir = filepath.Dir(targetDir)
				}

				newPath = filepath.Join(targetDir, filepath.Base(file.Filepath))

				// Handle filename conflicts
				if newPath != originalPath {
					newPath = fileutils.GenerateUniqueFilepathIfExists(newPath)

					// Move the file
					if err := moveFileWithCovers(originalPath, newPath); err != nil {
						// Rollback already-moved files
						rollbackFileMoves(movedFiles)
						return errors.Wrapf(err, "failed to move file %s", originalPath)
					}

					movedFiles = append(movedFiles, fileMove{
						FileID:       file.ID,
						OriginalPath: originalPath,
						NewPath:      newPath,
					})
				}
			}

			// Update file record
			file.BookID = targetBook.ID
			file.Filepath = newPath
			_, err := tx.NewUpdate().
				Model(file).
				Column("book_id", "filepath", "updated_at").
				WherePK().
				Exec(ctx)
			if err != nil {
				// Rollback physical file moves
				rollbackFileMoves(movedFiles)
				return errors.WithStack(err)
			}

			result.FilesMoved++
		}

		return nil
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Check and delete empty source books
	for sourceBookID := range sourceBookIDs {
		var remainingFiles int
		err := svc.db.NewSelect().
			Model((*models.File)(nil)).
			Where("book_id = ?", sourceBookID).
			Count(ctx)
		if err != nil {
			continue // Non-critical error
		}

		if remainingFiles == 0 {
			if err := svc.DeleteBook(ctx, sourceBookID); err == nil {
				result.DeletedBookIDs = append(result.DeletedBookIDs, sourceBookID)
				result.SourceBookDeleted = true
			}
		}
	}

	// Reload target book with all relations
	result.TargetBook, err = svc.RetrieveBook(ctx, RetrieveBookOptions{ID: &targetBook.ID})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return result, nil
}

// createBookFromFile creates a new book using the file's directory as the book path.
func (svc *Service) createBookFromFile(ctx context.Context, file *models.File) (*models.Book, error) {
	// Use the file's directory as the book path
	bookPath := filepath.Dir(file.Filepath)
	bookTitle := filepath.Base(bookPath)

	// If the file is at the root level, use the filename without extension
	if bookPath == "." || bookPath == "/" {
		bookPath = file.Filepath
		ext := filepath.Ext(file.Filepath)
		bookTitle = filepath.Base(file.Filepath)
		if ext != "" {
			bookTitle = bookTitle[:len(bookTitle)-len(ext)]
		}
	}

	book := &models.Book{
		LibraryID:       file.LibraryID,
		Filepath:        bookPath,
		Title:           bookTitle,
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       sortname.ForTitle(bookTitle),
		SortTitleSource: models.DataSourceFilepath,
	}

	err := svc.CreateBook(ctx, book)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return book, nil
}

// moveFileWithCovers moves a file and its associated cover images.
func moveFileWithCovers(src, dst string) error {
	// The fileutils package already handles cover moves
	_, err := fileutils.MoveFile(src, dst)
	return err
}

// rollbackFileMoves attempts to restore files to their original locations.
func rollbackFileMoves(moves []fileMove) {
	for i := len(moves) - 1; i >= 0; i-- {
		move := moves[i]
		// Best-effort rollback - log errors but continue
		if err := fileutils.MoveFile(move.NewPath, move.OriginalPath); err != nil {
			// TODO: Log warning about failed rollback
		}
	}
}
```

### Step 4: Add missing fileutils function

Add to `pkg/fileutils/operations.go`:

```go
// GenerateUniqueFilepathIfExists returns a unique filepath if the path exists, otherwise returns the original.
func GenerateUniqueFilepathIfExists(path string) string {
	return generateUniqueFilepath(path)
}

// MoveFile safely moves a file from source to destination. Exported for use by merge package.
func MoveFile(src, dst string) (bool, error) {
	return moveFile(src, dst) == nil, moveFile(src, dst)
}
```

Wait, `moveFile` already exists but isn't exported. Let me adjust:

```go
// MoveFileWithRollback moves a file and returns info needed for rollback.
func MoveFileWithRollback(src, dst string) error {
	return moveFile(src, dst)
}
```

### Step 5: Run tests to verify they pass

Run: `go test -v -race ./pkg/books/... -run TestMoveFilesToBook -count=1`
Expected: PASS

### Step 6: Commit

```bash
git add pkg/books/merge.go pkg/books/merge_test.go pkg/fileutils/operations.go
git commit -m "$(cat <<'EOF'
[Backend] Add MoveFilesToBook core operation for book merge/split

Implements the core service method that moves files between books,
handles physical file relocation when OrganizeFileStructure is enabled,
and cleans up empty source books.
EOF
)"
```

---

## Task 2: Backend - Add Move Files Handler

**Files:**
- Modify: `pkg/books/validators.go`
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`

### Step 1: Add request/response types to validators.go

```go
// MoveFilesPayload is the payload for moving files to another book.
type MoveFilesPayload struct {
	FileIDs      []int `json:"file_ids" validate:"required,min=1,dive,min=1"`
	TargetBookID *int  `json:"target_book_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
}

// MoveFilesResponse is the response from a move files operation.
type MoveFilesResponse struct {
	TargetBook        *models.Book `json:"target_book"`
	FilesMoved        int          `json:"files_moved"`
	SourceBookDeleted bool         `json:"source_book_deleted"`
}
```

### Step 2: Add handler to handlers.go

```go
// HandleMoveFiles handles POST /books/:id/move-files
func (h *BookHandler) HandleMoveFiles(c echo.Context) error {
	ctx := c.Request().Context()

	bookID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.ValidationError("invalid book id")
	}

	// Get the source book to determine library
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
	if err != nil {
		return errcodes.NotFound("book not found")
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	var payload MoveFilesPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Validate files belong to this book
	for _, fileID := range payload.FileIDs {
		found := false
		for _, f := range book.Files {
			if f.ID == fileID {
				found = true
				break
			}
		}
		if !found {
			return errcodes.ValidationError("file does not belong to this book")
		}
	}

	// If target book specified, verify it exists and is in same library
	if payload.TargetBookID != nil {
		targetBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: payload.TargetBookID})
		if err != nil {
			return errcodes.NotFound("target book not found")
		}
		if targetBook.LibraryID != book.LibraryID {
			return errcodes.ValidationError("target book must be in the same library")
		}
	}

	result, err := h.bookService.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      payload.FileIDs,
		TargetBookID: payload.TargetBookID,
		LibraryID:    book.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Update search indexes
	if err := h.searchService.IndexBook(ctx, result.TargetBook); err != nil {
		log.Warn("failed to update search index for target book", logger.Data{"book_id": result.TargetBook.ID, "error": err.Error()})
	}

	// Update indexes for source books that still exist
	sourceBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &bookID})
	if err == nil {
		if err := h.searchService.IndexBook(ctx, sourceBook); err != nil {
			log.Warn("failed to update search index for source book", logger.Data{"book_id": sourceBook.ID, "error": err.Error()})
		}
	}

	// Remove deleted books from index
	for _, deletedID := range result.DeletedBookIDs {
		if err := h.searchService.DeleteFromBookIndex(ctx, deletedID); err != nil {
			log.Warn("failed to remove deleted book from search index", logger.Data{"book_id": deletedID, "error": err.Error()})
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, MoveFilesResponse{
		TargetBook:        result.TargetBook,
		FilesMoved:        result.FilesMoved,
		SourceBookDeleted: result.SourceBookDeleted,
	}))
}
```

### Step 3: Register route in routes.go

Add after the existing book routes:

```go
// Move files between books
g.POST("/:id/move-files", h.HandleMoveFiles, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

### Step 4: Run `make tygo` to generate TypeScript types

Run: `make tygo`
Expected: Types generated (or "Nothing to be done" if already up-to-date)

### Step 5: Run tests and lint

Run: `make check`
Expected: PASS

### Step 6: Commit

```bash
git add pkg/books/validators.go pkg/books/handlers.go pkg/books/routes.go
git commit -m "$(cat <<'EOF'
[Backend] Add POST /books/:id/move-files endpoint

Exposes the MoveFilesToBook operation via REST API with proper
validation and library access control.
EOF
)"
```

---

## Task 3: Backend - Add Merge Books Handler

**Files:**
- Modify: `pkg/books/validators.go`
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`

### Step 1: Add request/response types to validators.go

```go
// MergeBooksPayload is the payload for merging multiple books.
type MergeBooksPayload struct {
	SourceBookIDs []int `json:"source_book_ids" validate:"required,min=1,dive,min=1"`
	TargetBookID  int   `json:"target_book_id" validate:"required,min=1"`
}

// MergeBooksResponse is the response from a merge books operation.
type MergeBooksResponse struct {
	TargetBook   *models.Book `json:"target_book"`
	FilesMoved   int          `json:"files_moved"`
	BooksDeleted int          `json:"books_deleted"`
}
```

### Step 2: Add handler to handlers.go

```go
// HandleMergeBooks handles POST /books/merge
func (h *BookHandler) HandleMergeBooks(c echo.Context) error {
	ctx := c.Request().Context()

	var payload MergeBooksPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Get target book
	targetBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &payload.TargetBookID})
	if err != nil {
		return errcodes.NotFound("target book not found")
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(targetBook.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Validate source books exist and are in same library
	var allFileIDs []int
	for _, sourceID := range payload.SourceBookIDs {
		if sourceID == payload.TargetBookID {
			continue // Skip target book if included in sources
		}
		sourceBook, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &sourceID})
		if err != nil {
			return errcodes.NotFound("source book not found")
		}
		if sourceBook.LibraryID != targetBook.LibraryID {
			return errcodes.ValidationError("all books must be in the same library")
		}
		for _, f := range sourceBook.Files {
			allFileIDs = append(allFileIDs, f.ID)
		}
	}

	if len(allFileIDs) == 0 {
		return errcodes.ValidationError("no files to merge")
	}

	result, err := h.bookService.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      allFileIDs,
		TargetBookID: &payload.TargetBookID,
		LibraryID:    targetBook.LibraryID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Update search index for target book
	if err := h.searchService.IndexBook(ctx, result.TargetBook); err != nil {
		log.Warn("failed to update search index for target book", logger.Data{"book_id": result.TargetBook.ID, "error": err.Error()})
	}

	// Remove deleted books from index
	for _, deletedID := range result.DeletedBookIDs {
		if err := h.searchService.DeleteFromBookIndex(ctx, deletedID); err != nil {
			log.Warn("failed to remove deleted book from search index", logger.Data{"book_id": deletedID, "error": err.Error()})
		}
	}

	return errors.WithStack(c.JSON(http.StatusOK, MergeBooksResponse{
		TargetBook:   result.TargetBook,
		FilesMoved:   result.FilesMoved,
		BooksDeleted: len(result.DeletedBookIDs),
	}))
}
```

### Step 3: Register route in routes.go

Add before the `/:id` routes (order matters for Echo):

```go
// Merge books
g.POST("/merge", h.HandleMergeBooks, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

### Step 4: Run `make tygo`

Run: `make tygo`

### Step 5: Run tests and lint

Run: `make check`
Expected: PASS

### Step 6: Commit

```bash
git add pkg/books/validators.go pkg/books/handlers.go pkg/books/routes.go
git commit -m "$(cat <<'EOF'
[Backend] Add POST /books/merge endpoint for bulk merge

Allows merging multiple books into a single target book,
moving all files and cleaning up empty source books.
EOF
)"
```

---

## Task 4: Frontend - Add API Mutation Hooks

**Files:**
- Modify: `app/hooks/queries/books.ts`

### Step 1: Add mutation types and hooks

```typescript
// Add to existing imports
import type {
  // ... existing imports
  MoveFilesPayload,
  MoveFilesResponse,
  MergeBooksPayload,
  MergeBooksResponse,
} from "@/types";

// Add new interfaces
interface MoveFilesMutationVariables {
  bookId: number;
  payload: MoveFilesPayload;
}

interface MergeBooksMutationVariables {
  payload: MergeBooksPayload;
}

// Add new hooks
export const useMoveFiles = () => {
  const queryClient = useQueryClient();

  return useMutation<MoveFilesResponse, ShishoAPIError, MoveFilesMutationVariables>({
    mutationFn: ({ bookId, payload }) => {
      return API.request("POST", `/books/${bookId}/move-files`, payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};

export const useMergeBooks = () => {
  const queryClient = useQueryClient();

  return useMutation<MergeBooksResponse, ShishoAPIError, MergeBooksMutationVariables>({
    mutationFn: ({ payload }) => {
      return API.request("POST", "/books/merge", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};
```

### Step 2: Run lint

Run: `yarn lint`
Expected: PASS

### Step 3: Commit

```bash
git add app/hooks/queries/books.ts
git commit -m "$(cat <<'EOF'
[Frontend] Add useMoveFiles and useMergeBooks mutation hooks

Tanstack Query hooks for the new move-files and merge endpoints.
EOF
)"
```

---

## Task 5: Frontend - Move Files Dialog Component

**Files:**
- Create: `app/components/library/MoveFilesDialog.tsx`

### Step 1: Create the dialog component

```tsx
import { Loader2, Plus, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useMoveFiles } from "@/hooks/queries/books";
import { useBooks } from "@/hooks/queries/books";
import type { Book, File, Library } from "@/types";

interface MoveFilesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sourceBook: Book;
  selectedFiles: File[];
  library: Library;
  onSuccess?: (targetBook: Book) => void;
}

export function MoveFilesDialog({
  open,
  onOpenChange,
  sourceBook,
  selectedFiles,
  library,
  onSuccess,
}: MoveFilesDialogProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedBookId, setSelectedBookId] = useState<string>("new");
  const moveFilesMutation = useMoveFiles();

  // Fetch books for selection
  const booksQuery = useBooks(
    { library_id: library.id, limit: 50 },
    { enabled: open },
  );

  // Filter out source book and apply search
  const availableBooks = useMemo(() => {
    if (!booksQuery.data?.books) return [];
    return booksQuery.data.books.filter((book) => {
      if (book.id === sourceBook.id) return false;
      if (!searchQuery) return true;
      return book.title.toLowerCase().includes(searchQuery.toLowerCase());
    });
  }, [booksQuery.data?.books, sourceBook.id, searchQuery]);

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setSearchQuery("");
      setSelectedBookId("new");
    }
  }, [open]);

  const handleMove = async () => {
    const targetBookId = selectedBookId === "new" ? undefined : parseInt(selectedBookId, 10);

    try {
      const result = await moveFilesMutation.mutateAsync({
        bookId: sourceBook.id,
        payload: {
          file_ids: selectedFiles.map((f) => f.id),
          target_book_id: targetBookId,
        },
      });

      if (result.target_book) {
        toast.success(
          `Moved ${result.files_moved} file${result.files_moved !== 1 ? "s" : ""} to "${result.target_book.title}"`,
        );
        onSuccess?.(result.target_book);
      }
      onOpenChange(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to move files";
      toast.error(message);
    }
  };

  const warningMessage = library.organize_file_structure
    ? "The selected files will be moved to the target book's folder. Metadata from the source book will not be transferred."
    : "The selected files will be reassigned to the target book. Metadata from the source book will not be transferred.";

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Move Files</DialogTitle>
          <DialogDescription>
            Move {selectedFiles.length} file{selectedFiles.length !== 1 ? "s" : ""} to another book
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <Alert>
            <AlertDescription>{warningMessage}</AlertDescription>
          </Alert>

          <div className="space-y-2">
            <Label>Destination</Label>
            <RadioGroup
              onValueChange={setSelectedBookId}
              value={selectedBookId}
            >
              <div className="flex items-center space-x-2 p-2 rounded-md hover:bg-muted/50">
                <RadioGroupItem id="new" value="new" />
                <Label className="flex items-center gap-2 cursor-pointer" htmlFor="new">
                  <Plus className="h-4 w-4" />
                  Create new book
                </Label>
              </div>

              {availableBooks.length > 0 && (
                <>
                  <div className="relative">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                    <Input
                      className="pl-9"
                      onChange={(e) => setSearchQuery(e.target.value)}
                      placeholder="Search books..."
                      value={searchQuery}
                    />
                  </div>

                  <ScrollArea className="h-48 border rounded-md p-2">
                    {availableBooks.map((book) => (
                      <div
                        className="flex items-center space-x-2 p-2 rounded-md hover:bg-muted/50"
                        key={book.id}
                      >
                        <RadioGroupItem id={`book-${book.id}`} value={String(book.id)} />
                        <Label className="cursor-pointer flex-1 truncate" htmlFor={`book-${book.id}`}>
                          {book.title}
                        </Label>
                        <span className="text-xs text-muted-foreground">
                          {book.files?.length || 0} files
                        </span>
                      </div>
                    ))}
                  </ScrollArea>
                </>
              )}
            </RadioGroup>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={moveFilesMutation.isPending}
            onClick={handleMove}
          >
            {moveFilesMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Move Files
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

### Step 2: Run lint

Run: `yarn lint`
Expected: PASS

### Step 3: Commit

```bash
git add app/components/library/MoveFilesDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add MoveFilesDialog component

Dialog for selecting destination book when moving files,
with search, create new book option, and library warning.
EOF
)"
```

---

## Task 6: Frontend - Add File Selection to BookDetail

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

### Step 1: Add selection state and UI

Add imports:

```tsx
import { Check, Square } from "lucide-react";
import { MoveFilesDialog } from "@/components/library/MoveFilesDialog";
```

Add state inside `BookDetail` component:

```tsx
const [isSelectMode, setIsSelectMode] = useState(false);
const [selectedFileIds, setSelectedFileIds] = useState<Set<number>>(new Set());
const [showMoveDialog, setShowMoveDialog] = useState(false);

const toggleFileSelection = (fileId: number) => {
  setSelectedFileIds((prev) => {
    const next = new Set(prev);
    if (next.has(fileId)) {
      next.delete(fileId);
    } else {
      next.add(fileId);
    }
    return next;
  });
};

const handleSelectAll = () => {
  if (selectedFileIds.size === mainFiles.length) {
    setSelectedFileIds(new Set());
  } else {
    setSelectedFileIds(new Set(mainFiles.map((f) => f.id)));
  }
};

const exitSelectMode = () => {
  setIsSelectMode(false);
  setSelectedFileIds(new Set());
};

const selectedFiles = mainFiles.filter((f) => selectedFileIds.has(f.id));
```

Modify `FileRow` props interface and component to accept selection props:

```tsx
interface FileRowProps {
  // ... existing props
  isSelectMode?: boolean;
  isFileSelected?: boolean;
  onToggleSelect?: () => void;
}
```

Add checkbox UI to FileRow (at the beginning of the primary row):

```tsx
{isSelectMode && (
  <button
    className={cn(
      "shrink-0 h-5 w-5 rounded border flex items-center justify-center",
      isFileSelected
        ? "bg-primary border-primary"
        : "border-muted-foreground/50 hover:border-primary/50",
    )}
    onClick={(e) => {
      e.stopPropagation();
      onToggleSelect?.();
    }}
    type="button"
  >
    {isFileSelected && <Check className="h-3 w-3 text-white" />}
  </button>
)}
```

Add "Select" button in Files header:

```tsx
<div className="flex items-center justify-between mb-3">
  <h3 className="font-semibold">Files ({mainFiles.length})</h3>
  {mainFiles.length > 1 && (
    <Button
      onClick={() => setIsSelectMode(!isSelectMode)}
      size="sm"
      variant="ghost"
    >
      {isSelectMode ? "Cancel" : "Select"}
    </Button>
  )}
</div>
```

Add floating action bar when files selected:

```tsx
{selectedFileIds.size > 0 && (
  <div className="fixed bottom-4 left-1/2 -translate-x-1/2 bg-background border rounded-lg shadow-lg p-3 flex items-center gap-3 z-50">
    <span className="text-sm text-muted-foreground">
      {selectedFileIds.size} file{selectedFileIds.size !== 1 ? "s" : ""} selected
    </span>
    <Button onClick={handleSelectAll} size="sm" variant="outline">
      {selectedFileIds.size === mainFiles.length ? "Deselect All" : "Select All"}
    </Button>
    <Button onClick={() => setShowMoveDialog(true)} size="sm">
      Move to...
    </Button>
  </div>
)}
```

Add MoveFilesDialog at the end:

```tsx
{showMoveDialog && libraryQuery.data && (
  <MoveFilesDialog
    library={libraryQuery.data}
    onOpenChange={setShowMoveDialog}
    onSuccess={() => exitSelectMode()}
    open={showMoveDialog}
    selectedFiles={selectedFiles}
    sourceBook={book}
  />
)}
```

### Step 2: Run lint

Run: `yarn lint`
Expected: PASS

### Step 3: Commit

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add file selection mode to BookDetail page

Users can select files and move them to another book via
floating action bar and MoveFilesDialog.
EOF
)"
```

---

## Task 7: Frontend - Merge Books Dialog Component

**Files:**
- Create: `app/components/library/MergeBooksDialog.tsx`

### Step 1: Create the dialog component

```tsx
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useMergeBooks } from "@/hooks/queries/books";
import type { Book, Library } from "@/types";

interface MergeBooksDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  books: Book[];
  library: Library;
  onSuccess?: (targetBook: Book) => void;
}

export function MergeBooksDialog({
  open,
  onOpenChange,
  books,
  library,
  onSuccess,
}: MergeBooksDialogProps) {
  const [selectedTargetId, setSelectedTargetId] = useState<string>("");
  const mergeBooksMutation = useMergeBooks();

  // Reset state when dialog opens, default to first book
  useEffect(() => {
    if (open && books.length > 0) {
      setSelectedTargetId(String(books[0].id));
    }
  }, [open, books]);

  const handleMerge = async () => {
    const targetBookId = parseInt(selectedTargetId, 10);
    const sourceBookIds = books.map((b) => b.id).filter((id) => id !== targetBookId);

    try {
      const result = await mergeBooksMutation.mutateAsync({
        payload: {
          source_book_ids: sourceBookIds,
          target_book_id: targetBookId,
        },
      });

      if (result.target_book) {
        toast.success(
          `Merged ${result.books_deleted} book${result.books_deleted !== 1 ? "s" : ""} into "${result.target_book.title}"`,
        );
        onSuccess?.(result.target_book);
      }
      onOpenChange(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to merge books";
      toast.error(message);
    }
  };

  const warningMessage = library.organize_file_structure
    ? "Other books will be deleted. Their files will move to the selected target's folder. Metadata from deleted books will not be transferred."
    : "Other books will be deleted. Their files will be reassigned to the selected target. Metadata from deleted books will not be transferred.";

  const totalFiles = books.reduce((sum, book) => sum + (book.files?.length || 0), 0);

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Merge Books</DialogTitle>
          <DialogDescription>
            Merge {books.length} books ({totalFiles} files total)
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <Alert>
            <AlertDescription>{warningMessage}</AlertDescription>
          </Alert>

          <div className="space-y-2">
            <Label>Select target book</Label>
            <p className="text-sm text-muted-foreground">
              All files will be moved to this book. Other books will be deleted.
            </p>

            <ScrollArea className="h-64 border rounded-md p-2">
              <RadioGroup
                onValueChange={setSelectedTargetId}
                value={selectedTargetId}
              >
                {books.map((book) => (
                  <div
                    className="flex items-start space-x-2 p-2 rounded-md hover:bg-muted/50"
                    key={book.id}
                  >
                    <RadioGroupItem
                      className="mt-1"
                      id={`book-${book.id}`}
                      value={String(book.id)}
                    />
                    <Label className="cursor-pointer flex-1" htmlFor={`book-${book.id}`}>
                      <div className="font-medium truncate">{book.title}</div>
                      <div className="text-xs text-muted-foreground">
                        {book.files?.length || 0} files
                        {book.authors && book.authors.length > 0 && (
                          <> · {book.authors.map((a) => a.person?.name).join(", ")}</>
                        )}
                      </div>
                    </Label>
                  </div>
                ))}
              </RadioGroup>
            </ScrollArea>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={mergeBooksMutation.isPending || !selectedTargetId}
            onClick={handleMerge}
            variant="destructive"
          >
            {mergeBooksMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Merge Books
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

### Step 2: Run lint

Run: `yarn lint`
Expected: PASS

### Step 3: Commit

```bash
git add app/components/library/MergeBooksDialog.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add MergeBooksDialog component

Dialog for selecting target book when merging multiple books,
with destructive action warning and book info display.
EOF
)"
```

---

## Task 8: Frontend - Add Book Selection to Library View

**Files:**
- Modify: `app/contexts/BulkSelection/context.ts` (if needed)
- Modify: `app/components/library/Gallery.tsx`
- Modify: `app/components/library/BookItem.tsx`
- Modify: `app/components/pages/LibraryHome.tsx`

### Step 1: Check existing BulkSelection context

The context already exists with `selectedBookIds`, `toggleBook`, `isSelected`, etc. We just need to add the merge action.

### Step 2: Add Merge button to Gallery

In `Gallery.tsx`, add merge button to the bulk action bar:

```tsx
import { MergeBooksDialog } from "@/components/library/MergeBooksDialog";

// Add state
const [showMergeDialog, setShowMergeDialog] = useState(false);

// In the selection action bar
{selectedBookIds.length >= 2 && (
  <Button onClick={() => setShowMergeDialog(true)} size="sm">
    Merge
  </Button>
)}

// Add dialog
{showMergeDialog && (
  <MergeBooksDialog
    books={selectedBooks}
    library={library}
    onOpenChange={setShowMergeDialog}
    onSuccess={() => {
      exitSelectionMode();
      setShowMergeDialog(false);
    }}
    open={showMergeDialog}
  />
)}
```

Note: Need to get `selectedBooks` from the book data. Add logic to filter books by selectedBookIds.

### Step 3: Run lint

Run: `yarn lint`
Expected: PASS

### Step 4: Commit

```bash
git add app/components/library/Gallery.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add Merge button to library bulk selection

When 2+ books are selected, shows Merge button that opens
MergeBooksDialog.
EOF
)"
```

---

## Task 9: Backend Tests for Edge Cases

**Files:**
- Modify: `pkg/books/merge_test.go`

### Step 1: Add edge case tests

```go
func TestMoveFilesToBook_DifferentLibraries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	// Create two libraries
	lib1 := &models.Library{Name: "Library 1"}
	_, err := db.NewInsert().Model(lib1).Exec(ctx)
	require.NoError(t, err)

	lib2 := &models.Library{Name: "Library 2"}
	_, err = db.NewInsert().Model(lib2).Exec(ctx)
	require.NoError(t, err)

	// Create books in different libraries
	book1 := &models.Book{
		LibraryID: lib1.ID,
		Title:     "Book 1",
		Filepath:  "/lib1/book1",
		Files: []*models.File{
			{LibraryID: lib1.ID, Filepath: "/lib1/book1/file.epub", FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, book1)
	require.NoError(t, err)

	book2 := &models.Book{
		LibraryID: lib2.ID,
		Title:     "Book 2",
		Filepath:  "/lib2/book2",
	}
	err = svc.CreateBook(ctx, book2)
	require.NoError(t, err)

	// Try to move file to book in different library - should fail
	_, err = svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{book1.Files[0].ID},
		TargetBookID: &book2.ID,
		LibraryID:    lib1.ID,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "same library")
}

func TestMoveFilesToBook_MoveToSameBook(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	library := &models.Library{Name: "Test Library"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	book := &models.Book{
		LibraryID: library.ID,
		Title:     "Test Book",
		Filepath:  "/test/book",
		Files: []*models.File{
			{LibraryID: library.ID, Filepath: "/test/book/file.epub", FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, book)
	require.NoError(t, err)

	// Try to move file to same book - should fail
	_, err = svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{book.Files[0].ID},
		TargetBookID: &book.ID,
		LibraryID:    library.ID,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already belong")
}

func TestMoveFilesToBook_FileNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := NewService(db)

	library := &models.Library{Name: "Test Library"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	targetBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Target Book",
		Filepath:  "/test/target",
	}
	err = svc.CreateBook(ctx, targetBook)
	require.NoError(t, err)

	// Try to move non-existent file
	_, err = svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{99999},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	assert.Error(t, err)
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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no files")
}
```

### Step 2: Run tests

Run: `go test -v -race ./pkg/books/... -run TestMoveFilesToBook -count=1`
Expected: PASS

### Step 3: Commit

```bash
git add pkg/books/merge_test.go
git commit -m "$(cat <<'EOF'
[Test] Add edge case tests for MoveFilesToBook

Tests for different libraries, same book, file not found,
and no files selected scenarios.
EOF
)"
```

---

## Task 10: Integration Test - Full Flow

**Files:**
- Create: `pkg/books/merge_integration_test.go`

### Step 1: Write integration test with file system

```go
package books

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
		Name:                  "Test Library",
		OrganizeFileStructure: true,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	libraryPath := &models.LibraryPath{
		LibraryID: library.ID,
		Path:      tmpDir,
	}
	_, err = db.NewInsert().Model(libraryPath).Exec(ctx)
	require.NoError(t, err)

	// Create source book
	sourceBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Source Book",
		Filepath:  sourceDir,
		Files: []*models.File{
			{LibraryID: library.ID, Filepath: sourceFile, FileType: "epub"},
		},
	}
	err = svc.CreateBook(ctx, sourceBook)
	require.NoError(t, err)

	// Create target book
	targetBook := &models.Book{
		LibraryID: library.ID,
		Title:     "Target Book",
		Filepath:  targetDir,
	}
	err = svc.CreateBook(ctx, targetBook)
	require.NoError(t, err)

	// Move file
	result, err := svc.MoveFilesToBook(ctx, MoveFilesOptions{
		FileIDs:      []int{sourceBook.Files[0].ID},
		TargetBookID: &targetBook.ID,
		LibraryID:    library.ID,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.FilesMoved)

	// Verify file was physically moved
	newFilePath := filepath.Join(targetDir, "test.epub")
	_, err = os.Stat(newFilePath)
	assert.NoError(t, err, "file should exist at new location")

	_, err = os.Stat(sourceFile)
	assert.True(t, os.IsNotExist(err), "file should not exist at old location")

	// Verify database record updated
	movedFile, err := svc.RetrieveFile(ctx, RetrieveFileOptions{ID: &sourceBook.Files[0].ID})
	require.NoError(t, err)
	assert.Equal(t, newFilePath, movedFile.Filepath)
}
```

### Step 2: Run test

Run: `go test -v -race ./pkg/books/... -run TestMoveFilesToBook_WithOrganizeFileStructure -count=1`
Expected: PASS

### Step 3: Commit

```bash
git add pkg/books/merge_integration_test.go
git commit -m "$(cat <<'EOF'
[Test] Add integration test for file moves with OrganizeFileStructure

Tests that physical files are moved correctly when
OrganizeFileStructure is enabled.
EOF
)"
```

---

## Task 11: Final - Run Full Test Suite and Lint

### Step 1: Run all checks

Run: `make check`
Expected: PASS

### Step 2: Manual testing checklist

- [ ] Start dev server: `make start`
- [ ] Navigate to a book with multiple files
- [ ] Enter select mode, select files, click "Move to..."
- [ ] Create new book option works
- [ ] Move to existing book works
- [ ] Source book deleted when all files moved
- [ ] Navigate to library view
- [ ] Select 2+ books
- [ ] Click "Merge", select target, confirm
- [ ] Files consolidated, source books deleted
- [ ] Search indexes updated (search for moved content)

### Step 3: Final commit

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Feature] Book merge and split functionality

Complete implementation of file moving between books:
- Backend: MoveFilesToBook service method with rollback support
- Backend: POST /books/:id/move-files endpoint
- Backend: POST /books/merge endpoint for bulk merge
- Frontend: File selection mode on book detail page
- Frontend: MoveFilesDialog for destination selection
- Frontend: MergeBooksDialog for bulk merge
- Frontend: Merge button in library bulk selection
EOF
)"
```

---

## Summary

This plan implements book merge/split in 11 tasks:

1. **Backend Core** - `MoveFilesToBook` service method with tests
2. **Move Files Handler** - `POST /books/:id/move-files` endpoint
3. **Merge Books Handler** - `POST /books/merge` endpoint
4. **API Mutation Hooks** - `useMoveFiles` and `useMergeBooks`
5. **MoveFilesDialog** - Destination selection dialog
6. **BookDetail Selection** - File selection mode and action bar
7. **MergeBooksDialog** - Target selection for bulk merge
8. **Library Selection** - Merge button in bulk actions
9. **Edge Case Tests** - Validation error scenarios
10. **Integration Test** - File system with `OrganizeFileStructure`
11. **Final Verification** - Full test suite and manual testing
