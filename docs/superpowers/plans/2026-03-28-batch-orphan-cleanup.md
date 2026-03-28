# Batch Orphan File Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sequential orphan-file cleanup loop during library scans with batch database operations to reduce ~N individual DB round-trips to ~5 batch queries.

**Architecture:** New batch delete methods in the books service (`DeleteFilesByIDs`, `DeleteBooksByIDs`, `PromoteNextPrimaryFile`) called from a new `cleanupOrphanedFiles` method in `pkg/worker/scan_orphans.go`. The existing sequential orphan loop in `scan.go` is replaced with a single call to this new method.

**Tech Stack:** Go, Bun ORM, SQLite, testify

**Spec:** `docs/superpowers/specs/2026-03-28-batch-orphan-cleanup-design.md`

---

### Task 1: Add `DeleteFilesByIDs` batch method

**Files:**
- Modify: `pkg/books/service.go` (add after `DeleteFile` method, around line 1349)
- Test: `pkg/books/service_test.go`

- [ ] **Step 1: Write the failing test for `DeleteFilesByIDs`**

Create `pkg/books/service_test.go` (or add to existing). The test creates a book with two files, each having narrators, identifiers, and chapters, then batch-deletes both and verifies all related records are gone.

```go
package books_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/chapters"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(context.Background())
	require.NoError(t, err)
	return library
}

func createTestBook(t *testing.T, db *bun.DB, libraryID int) *models.Book {
	t.Helper()
	book := &models.Book{
		LibraryID:       libraryID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(context.Background())
	require.NoError(t, err)
	return book
}

func createTestFile(t *testing.T, svc *books.Service, bookID, libraryID int, filepath, fileType, fileRole string) *models.File {
	t.Helper()
	file := &models.File{
		BookID:        bookID,
		LibraryID:     libraryID,
		Filepath:      filepath,
		FileType:      fileType,
		FileRole:      fileRole,
		FilesizeBytes: 100,
	}
	err := svc.CreateFile(context.Background(), file)
	require.NoError(t, err)
	return file
}

func TestDeleteFilesByIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)
	chapterSvc := chapters.NewService(db)
	library := createTestLibrary(t, db)
	book := createTestBook(t, db, library.ID)

	file1 := createTestFile(t, svc, book.ID, library.ID, "/tmp/file1.epub", models.FileTypeEPUB, models.FileRoleMain)
	file2 := createTestFile(t, svc, book.ID, library.ID, "/tmp/file2.epub", models.FileTypeEPUB, models.FileRoleMain)

	// Add narrators
	now := time.Now()
	person1 := &models.Person{Name: "Narrator 1", SortName: "Narrator 1", CreatedAt: now, UpdatedAt: now}
	_, err := db.NewInsert().Model(person1).Exec(ctx)
	require.NoError(t, err)
	narrator1 := &models.Narrator{FileID: file1.ID, PersonID: person1.ID, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(narrator1).Exec(ctx)
	require.NoError(t, err)
	narrator2 := &models.Narrator{FileID: file2.ID, PersonID: person1.ID, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(narrator2).Exec(ctx)
	require.NoError(t, err)

	// Add identifiers
	id1 := &models.FileIdentifier{FileID: file1.ID, Type: "isbn", Value: "123", CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(id1).Exec(ctx)
	require.NoError(t, err)
	id2 := &models.FileIdentifier{FileID: file2.ID, Type: "isbn", Value: "456", CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(id2).Exec(ctx)
	require.NoError(t, err)

	// Add chapters
	err = chapterSvc.ReplaceChapters(ctx, file1.ID, []mediafile.ParsedChapter{{Title: "Ch1"}})
	require.NoError(t, err)
	err = chapterSvc.ReplaceChapters(ctx, file2.ID, []mediafile.ParsedChapter{{Title: "Ch2"}})
	require.NoError(t, err)

	// Delete both files
	err = svc.DeleteFilesByIDs(ctx, []int{file1.ID, file2.ID})
	require.NoError(t, err)

	// Verify files are gone
	var fileCount int
	fileCount, err = db.NewSelect().Model((*models.File)(nil)).Where("id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount)

	// Verify narrators are gone
	var narratorCount int
	narratorCount, err = db.NewSelect().Model((*models.Narrator)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, narratorCount)

	// Verify identifiers are gone
	var identifierCount int
	identifierCount, err = db.NewSelect().Model((*models.FileIdentifier)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, identifierCount)

	// Verify chapters are gone
	var chapterCount int
	chapterCount, err = db.NewSelect().Model((*models.Chapter)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount)
}

func TestDeleteFilesByIDs_EmptySlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)

	err := svc.DeleteFilesByIDs(ctx, []int{})
	require.NoError(t, err)

	err = svc.DeleteFilesByIDs(ctx, nil)
	require.NoError(t, err)
}
```

Note: The `chapters.ReplaceChapters` call above uses `mediafile.ParsedChapter`. Check the import: `"github.com/shishobooks/shisho/pkg/mediafile"`. If `ReplaceChapters` doesn't accept that type, create chapters directly:

```go
ch := &models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0}
_, err = db.NewInsert().Model(ch).Exec(ctx)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestDeleteFilesByIDs" -v -count=1`

Expected: Compilation failure — `svc.DeleteFilesByIDs` does not exist yet.

- [ ] **Step 3: Implement `DeleteFilesByIDs`**

Add to `pkg/books/service.go` after the `DeleteFile` method (after line 1349):

```go
// DeleteFilesByIDs batch-deletes multiple files and their related records
// (narrators, identifiers, chapters) in a single transaction.
// Returns nil immediately if fileIDs is empty.
// Does not handle primary file promotion — callers must handle that separately.
func (svc *Service) DeleteFilesByIDs(ctx context.Context, fileIDs []int) error {
	if len(fileIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Delete narrators for all files
		_, err := tx.NewDelete().
			Model((*models.Narrator)(nil)).
			Where("file_id IN (?)", bun.In(fileIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete identifiers for all files
		_, err = tx.NewDelete().
			Model((*models.FileIdentifier)(nil)).
			Where("file_id IN (?)", bun.In(fileIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete chapters for all files
		_, err = tx.NewDelete().
			Model((*models.Chapter)(nil)).
			Where("file_id IN (?)", bun.In(fileIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the file records
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("id IN (?)", bun.In(fileIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestDeleteFilesByIDs" -v -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "[Backend] Add DeleteFilesByIDs batch method for orphan cleanup"
```

---

### Task 2: Add `DeleteBooksByIDs` batch method

**Files:**
- Modify: `pkg/books/service.go` (add after `DeleteFilesByIDs`)
- Test: `pkg/books/service_test.go`

- [ ] **Step 1: Write the failing test for `DeleteBooksByIDs`**

Add to `pkg/books/service_test.go`:

```go
func TestDeleteBooksByIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)
	library := createTestLibrary(t, db)

	book1 := createTestBook(t, db, library.ID)
	book2 := createTestBook(t, db, library.ID)

	file1 := createTestFile(t, svc, book1.ID, library.ID, "/tmp/b1f1.epub", models.FileTypeEPUB, models.FileRoleMain)
	file2 := createTestFile(t, svc, book2.ID, library.ID, "/tmp/b2f1.epub", models.FileTypeEPUB, models.FileRoleMain)

	// Add narrators
	now := time.Now()
	person := &models.Person{Name: "Narrator", SortName: "Narrator", CreatedAt: now, UpdatedAt: now}
	_, err := db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Narrator{FileID: file1.ID, PersonID: person.ID, CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Narrator{FileID: file2.ID, PersonID: person.ID, CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)

	// Add identifiers
	_, err = db.NewInsert().Model(&models.FileIdentifier{FileID: file1.ID, Type: "isbn", Value: "111", CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)

	// Add chapters
	_, err = db.NewInsert().Model(&models.Chapter{FileID: file1.ID, Title: "Ch1", SortOrder: 0}).Exec(ctx)
	require.NoError(t, err)

	// Add authors
	author := &models.Person{Name: "Author", SortName: "Author", CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(author).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book1.ID, PersonID: author.ID, CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book2.ID, PersonID: author.ID, CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)

	// Add genres
	genre := &models.Genre{Name: "Fiction", LibraryID: library.ID, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.BookGenre{BookID: book1.ID, GenreID: genre.ID}).Exec(ctx)
	require.NoError(t, err)

	// Add tags
	tag := &models.Tag{Name: "great", LibraryID: library.ID, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(tag).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.BookTag{BookID: book1.ID, TagID: tag.ID}).Exec(ctx)
	require.NoError(t, err)

	// Add series
	ser := &models.Series{Name: "Series", SortName: "Series", LibraryID: library.ID, CreatedAt: now, UpdatedAt: now}
	_, err = db.NewInsert().Model(ser).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.BookSeries{BookID: book1.ID, SeriesID: ser.ID, CreatedAt: now, UpdatedAt: now}).Exec(ctx)
	require.NoError(t, err)

	// Delete both books
	err = svc.DeleteBooksByIDs(ctx, []int{book1.ID, book2.ID})
	require.NoError(t, err)

	// Verify books are gone
	bookCount, err := db.NewSelect().Model((*models.Book)(nil)).Where("id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, bookCount)

	// Verify files are gone
	fileCount, err := db.NewSelect().Model((*models.File)(nil)).Where("id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, fileCount)

	// Verify all book-level relations are gone
	authorCount, err := db.NewSelect().Model((*models.Author)(nil)).Where("book_id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, authorCount)

	narratorCount, err := db.NewSelect().Model((*models.Narrator)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, narratorCount)

	identifierCount, err := db.NewSelect().Model((*models.FileIdentifier)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, identifierCount)

	chapterCount, err := db.NewSelect().Model((*models.Chapter)(nil)).Where("file_id IN (?)", bun.In([]int{file1.ID, file2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, chapterCount)

	genreCount, err := db.NewSelect().Model((*models.BookGenre)(nil)).Where("book_id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, genreCount)

	tagCount, err := db.NewSelect().Model((*models.BookTag)(nil)).Where("book_id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, tagCount)

	seriesCount, err := db.NewSelect().Model((*models.BookSeries)(nil)).Where("book_id IN (?)", bun.In([]int{book1.ID, book2.ID})).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, seriesCount)

	// Person records should still exist (persons are cleaned up separately by cleanupOrphanedEntities)
	personCount, err := db.NewSelect().Model((*models.Person)(nil)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, personCount, "persons should remain — orphan entity cleanup is separate")
}

func TestDeleteBooksByIDs_EmptySlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)

	err := svc.DeleteBooksByIDs(ctx, []int{})
	require.NoError(t, err)

	err = svc.DeleteBooksByIDs(ctx, nil)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestDeleteBooksByIDs" -v -count=1`

Expected: Compilation failure — `svc.DeleteBooksByIDs` does not exist yet.

- [ ] **Step 3: Implement `DeleteBooksByIDs`**

Add to `pkg/books/service.go` after `DeleteFilesByIDs`:

```go
// DeleteBooksByIDs batch-deletes multiple books and all their associated records
// (files, narrators, identifiers, chapters, authors, series, genres, tags) in a single transaction.
// Returns nil immediately if bookIDs is empty.
func (svc *Service) DeleteBooksByIDs(ctx context.Context, bookIDs []int) error {
	if len(bookIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Get all file IDs for these books
		var fileIDs []int
		err := tx.NewSelect().
			Model((*models.File)(nil)).
			Column("id").
			Where("book_id IN (?)", bun.In(bookIDs)).
			Scan(ctx, &fileIDs)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete file-level relations
		if len(fileIDs) > 0 {
			_, err = tx.NewDelete().
				Model((*models.Narrator)(nil)).
				Where("file_id IN (?)", bun.In(fileIDs)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			_, err = tx.NewDelete().
				Model((*models.FileIdentifier)(nil)).
				Where("file_id IN (?)", bun.In(fileIDs)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}

			_, err = tx.NewDelete().
				Model((*models.Chapter)(nil)).
				Where("file_id IN (?)", bun.In(fileIDs)).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Delete files
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("book_id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete authors
		_, err = tx.NewDelete().
			Model((*models.Author)(nil)).
			Where("book_id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book series associations
		_, err = tx.NewDelete().
			Model((*models.BookSeries)(nil)).
			Where("book_id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book genres
		_, err = tx.NewDelete().
			Model((*models.BookGenre)(nil)).
			Where("book_id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete book tags
		_, err = tx.NewDelete().
			Model((*models.BookTag)(nil)).
			Where("book_id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the book records
		_, err = tx.NewDelete().
			Model((*models.Book)(nil)).
			Where("id IN (?)", bun.In(bookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestDeleteBooksByIDs" -v -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "[Backend] Add DeleteBooksByIDs batch method for orphan cleanup"
```

---

### Task 3: Add `PromoteNextPrimaryFile` method

**Files:**
- Modify: `pkg/books/service.go` (add after `DeleteBooksByIDs`)
- Test: `pkg/books/service_test.go`

- [ ] **Step 1: Write the failing test for `PromoteNextPrimaryFile`**

Add to `pkg/books/service_test.go`:

```go
func TestPromoteNextPrimaryFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)
	library := createTestLibrary(t, db)
	book := createTestBook(t, db, library.ID)

	// Create 3 files: 1 supplement (oldest), 1 main (middle), 1 main (newest)
	supplement := createTestFile(t, svc, book.ID, library.ID, "/tmp/supp.epub", models.FileTypeEPUB, models.FileRoleSupplement)
	mainOlder := createTestFile(t, svc, book.ID, library.ID, "/tmp/main1.epub", models.FileTypeEPUB, models.FileRoleMain)
	_ = createTestFile(t, svc, book.ID, library.ID, "/tmp/main2.epub", models.FileTypeEPUB, models.FileRoleMain)

	// Promote — should pick mainOlder (main files preferred over supplements, oldest first)
	err := svc.PromoteNextPrimaryFile(ctx, book.ID)
	require.NoError(t, err)

	// Verify
	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, updatedBook.PrimaryFileID)
	assert.Equal(t, mainOlder.ID, *updatedBook.PrimaryFileID)

	// Verify supplement is NOT chosen over main files
	assert.NotEqual(t, supplement.ID, *updatedBook.PrimaryFileID)
}

func TestPromoteNextPrimaryFile_NoFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := setupTestDB(t)
	svc := books.NewService(db)
	library := createTestLibrary(t, db)
	book := createTestBook(t, db, library.ID)

	// No files — should set primary_file_id to NULL
	err := svc.PromoteNextPrimaryFile(ctx, book.ID)
	require.NoError(t, err)

	var updatedBook models.Book
	err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	assert.Nil(t, updatedBook.PrimaryFileID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestPromoteNextPrimaryFile" -v -count=1`

Expected: Compilation failure — `svc.PromoteNextPrimaryFile` does not exist yet.

- [ ] **Step 3: Implement `PromoteNextPrimaryFile`**

Add to `pkg/books/service.go` after `DeleteBooksByIDs`:

```go
// PromoteNextPrimaryFile sets the primary file for a book to the next best candidate.
// Prefers main files over supplements, oldest first. If no files remain, sets primary_file_id to NULL.
// Used after batch file deletion to fix up primary file pointers.
func (svc *Service) PromoteNextPrimaryFile(ctx context.Context, bookID int) error {
	// Find the best candidate
	var candidate models.File
	err := svc.db.NewSelect().
		Model(&candidate).
		Where("book_id = ?", bookID).
		OrderExpr("CASE WHEN file_role = ? THEN 0 ELSE 1 END", models.FileRoleMain).
		Order("created_at ASC").
		Limit(1).
		Scan(ctx)

	if err != nil {
		// No files remain — set primary_file_id to NULL
		_, updateErr := svc.db.NewUpdate().
			Model((*models.Book)(nil)).
			Set("primary_file_id = NULL").
			Where("id = ?", bookID).
			Exec(ctx)
		return errors.WithStack(updateErr)
	}

	// Promote the candidate
	_, err = svc.db.NewUpdate().
		Model((*models.Book)(nil)).
		Set("primary_file_id = ?", candidate.ID).
		Where("id = ?", bookID).
		Exec(ctx)
	return errors.WithStack(err)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestPromoteNextPrimaryFile" -v -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/books/service.go pkg/books/service_test.go
git commit -m "[Backend] Add PromoteNextPrimaryFile method for batch orphan cleanup"
```

---

### Task 4: Create `cleanupOrphanedFiles` method

**Files:**
- Create: `pkg/worker/scan_orphans.go`
- Test: `pkg/worker/scan_orphans_test.go`

- [ ] **Step 1: Write the failing test for partial orphan scenario**

Create `pkg/worker/scan_orphans_test.go`:

```go
package worker

import (
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupOrphanedFiles_PartialOrphan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create library with temp path
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book dir with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Two Files")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "remove.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	files := tc.listFiles()
	require.Len(t, files, 2)

	// Identify which file to orphan
	var keepFile, removeFile *models.File
	for _, f := range files {
		if filepath.Base(f.Filepath) == "keep.epub" {
			keepFile = f
		} else {
			removeFile = f
		}
	}
	require.NotNil(t, keepFile)
	require.NotNil(t, removeFile)

	// Build existingFiles (what ListFilesForLibrary returns — main files)
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 2)

	// Build scannedPaths — only include the file we're keeping (simulates remove.epub being deleted from disk)
	scannedPaths := map[string]struct{}{
		keepFile.Filepath: {},
	}

	// Get library for the method call
	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	// Run orphan cleanup
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Verify: removed file is gone, kept file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, keepFile.ID, remainingFiles[0].ID)

	// Verify: book still exists
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)
}

func intPtr(i int) *int {
	return &i
}
```

Note: You'll need to import `"github.com/shishobooks/shisho/pkg/libraries"` and `"github.com/robinjoseph08/golib/logger"` in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/worker/ -run "TestCleanupOrphanedFiles_PartialOrphan" -v -count=1`

Expected: Compilation failure — `cleanupOrphanedFiles` does not exist yet.

- [ ] **Step 3: Implement `cleanupOrphanedFiles`**

Create `pkg/worker/scan_orphans.go`:

```go
package worker

import (
	"path/filepath"
	"strings"

	"context"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/joblogs"
	"github.com/shishobooks/shisho/pkg/models"
)

// cleanupOrphanedFiles batch-cleans files that exist in the database but were not found on disk
// during the scan. This replaces the previous sequential scanInternal loop with batch operations.
//
// The method is non-fatal: all errors are logged as warnings and execution continues.
func (w *Worker) cleanupOrphanedFiles(
	ctx context.Context,
	existingFiles []*models.File,
	scannedPaths map[string]struct{},
	library *models.Library,
	jobLog *joblogs.JobLogger,
) {
	// Step 1: Collect orphans and group by book.
	// existingFiles only contains main files (from ListFilesForLibrary).
	totalFilesByBook := make(map[int]int)       // bookID → total main file count
	orphansByBook := make(map[int][]*models.File) // bookID → orphaned files

	for _, file := range existingFiles {
		totalFilesByBook[file.BookID]++
		if _, seen := scannedPaths[file.Filepath]; !seen {
			orphansByBook[file.BookID] = append(orphansByBook[file.BookID], file)
		}
	}

	if len(orphansByBook) == 0 {
		return
	}

	jobLog.Info("batch orphan cleanup starting", logger.Data{
		"orphaned_books": len(orphansByBook),
	})

	// Collect directories for cleanup at the end
	orphanDirs := make(map[string]struct{})

	// Step 2 & 3: Handle partial orphan books.
	// Collect file IDs from books where only SOME main files are orphaned.
	var partialOrphanFileIDs []int
	partialOrphanBookIDs := make(map[int]struct{}) // books that need primary file check

	// Also collect file IDs from full-orphan books where a supplement was promoted
	var promotedBookOrphanFileIDs []int

	// Collect book IDs for full deletion
	var bookIDsToDelete []int

	for bookID, orphans := range orphansByBook {
		// Track directories for all orphans
		for _, f := range orphans {
			orphanDirs[filepath.Dir(f.Filepath)] = struct{}{}
		}

		if len(orphans) < totalFilesByBook[bookID] {
			// Partial orphan: some main files remain
			for _, f := range orphans {
				partialOrphanFileIDs = append(partialOrphanFileIDs, f.ID)
				jobLog.Info("orphaned file (partial)", logger.Data{"file_id": f.ID, "filepath": f.Filepath})
			}
			partialOrphanBookIDs[bookID] = struct{}{}
		}
	}

	// Batch-delete partial orphan files
	if len(partialOrphanFileIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, partialOrphanFileIDs); err != nil {
			jobLog.Warn("failed to batch-delete partial orphan files", logger.Data{"error": err.Error()})
		} else {
			// Promote primary file for affected books
			for bookID := range partialOrphanBookIDs {
				if err := w.bookService.PromoteNextPrimaryFile(ctx, bookID); err != nil {
					jobLog.Warn("failed to promote primary file", logger.Data{"book_id": bookID, "error": err.Error()})
				}
			}
		}
	}

	// Step 4: Handle full orphan books.
	// Build supported types set for supplement promotion.
	supportedTypes := map[string]struct{}{
		models.FileTypeEPUB: {},
		models.FileTypeCBZ:  {},
		models.FileTypeM4B:  {},
		models.FileTypePDF:  {},
	}
	if w.pluginManager != nil {
		for ext := range w.pluginManager.RegisteredFileExtensions() {
			supportedTypes[ext] = struct{}{}
		}
	}

	for bookID, orphans := range orphansByBook {
		if len(orphans) < totalFilesByBook[bookID] {
			continue // Already handled as partial orphan
		}

		// Full orphan: all main files are gone
		jobLog.Info("all main files orphaned for book", logger.Data{"book_id": bookID})

		// Load book with files to check for supplements
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
		if err != nil {
			jobLog.Warn("failed to retrieve orphaned book", logger.Data{"book_id": bookID, "error": err.Error()})
			continue
		}

		// Collect supplements (files with supplement role)
		var supplements []*models.File
		for i := range book.Files {
			if book.Files[i].FileRole == models.FileRoleSupplement {
				supplements = append(supplements, book.Files[i])
			}
		}

		// Try to promote a supplement
		var promoted bool
		for _, supp := range supplements {
			if _, supported := supportedTypes[supp.FileType]; supported {
				if err := w.bookService.PromoteSupplementToMain(ctx, supp.ID); err != nil {
					jobLog.Warn("failed to promote supplement", logger.Data{"file_id": supp.ID, "error": err.Error()})
				} else {
					jobLog.Info("promoted supplement to main", logger.Data{"file_id": supp.ID, "book_id": bookID})
					promoted = true
				}
				break
			}
		}

		if promoted {
			// Delete only the orphaned main files; book and supplements survive
			for _, f := range orphans {
				promotedBookOrphanFileIDs = append(promotedBookOrphanFileIDs, f.ID)
			}
			// Promote primary file to the newly promoted supplement
			if err := w.bookService.PromoteNextPrimaryFile(ctx, bookID); err != nil {
				jobLog.Warn("failed to promote primary file after supplement promotion", logger.Data{"book_id": bookID, "error": err.Error()})
			}
		} else {
			// No promotable supplement — delete the entire book
			// Remove from search index first
			if w.searchService != nil {
				if err := w.searchService.DeleteFromBookIndex(ctx, bookID); err != nil {
					jobLog.Warn("failed to remove book from search index", logger.Data{"book_id": bookID, "error": err.Error()})
				}
			}
			bookIDsToDelete = append(bookIDsToDelete, bookID)
			// Track book directory for cleanup
			orphanDirs[book.Filepath] = struct{}{}
			jobLog.Info("deleting orphaned book", logger.Data{"book_id": bookID})
		}
	}

	// Batch-delete orphaned files from promoted books
	if len(promotedBookOrphanFileIDs) > 0 {
		if err := w.bookService.DeleteFilesByIDs(ctx, promotedBookOrphanFileIDs); err != nil {
			jobLog.Warn("failed to batch-delete promoted book orphan files", logger.Data{"error": err.Error()})
		}
	}

	// Batch-delete fully orphaned books (cascades to all their files and relations)
	if len(bookIDsToDelete) > 0 {
		if err := w.bookService.DeleteBooksByIDs(ctx, bookIDsToDelete); err != nil {
			jobLog.Warn("failed to batch-delete orphaned books", logger.Data{"error": err.Error()})
		}
	}

	// Step 5: Directory cleanup.
	cleanupIgnoredPatterns := make([]string, 0, len(fileutils.ShishoSpecialFilePatterns)+len(w.config.SupplementExcludePatterns))
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, fileutils.ShishoSpecialFilePatterns...)
	cleanupIgnoredPatterns = append(cleanupIgnoredPatterns, w.config.SupplementExcludePatterns...)

	for dir := range orphanDirs {
		for _, libPath := range library.LibraryPaths {
			if strings.HasPrefix(dir, libPath.Filepath) {
				if err := fileutils.CleanupEmptyParentDirectories(dir, libPath.Filepath, cleanupIgnoredPatterns...); err != nil {
					jobLog.Warn("failed to cleanup empty directories", logger.Data{"path": dir, "error": err.Error()})
				}
				break
			}
		}
	}

	jobLog.Info("batch orphan cleanup complete", logger.Data{
		"partial_files_deleted": len(partialOrphanFileIDs),
		"promoted_files_deleted": len(promotedBookOrphanFileIDs),
		"books_deleted":         len(bookIDsToDelete),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/worker/ -run "TestCleanupOrphanedFiles_PartialOrphan" -v -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan_orphans.go pkg/worker/scan_orphans_test.go
git commit -m "[Backend] Add cleanupOrphanedFiles batch method for scan orphan cleanup"
```

---

### Task 5: Add remaining `cleanupOrphanedFiles` test scenarios

**Files:**
- Test: `pkg/worker/scan_orphans_test.go`

- [ ] **Step 1: Add test for full orphan with no supplements (book deleted)**

Add to `pkg/worker/scan_orphans_test.go`:

```go
func TestCleanupOrphanedFiles_FullOrphan_NoSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Single File")
	testgen.GenerateEPUB(t, bookDir, "only.epub", testgen.EPUBOptions{
		Title:   "Single File",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// No files in scannedPaths = all orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book and file should both be deleted
	assert.Len(t, tc.listBooks(), 0)
	assert.Len(t, tc.listFiles(), 0)
}
```

- [ ] **Step 2: Add test for full orphan with promotable supplement**

```go
func TestCleanupOrphanedFiles_FullOrphan_PromotesSupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] With Supplement")
	testgen.GenerateEPUB(t, bookDir, "main.epub", testgen.EPUBOptions{
		Title:   "With Supplement",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)

	allBooks := tc.listBooks()
	bookID := allBooks[0].ID

	// Manually add a supplement file in the DB (a CBZ supplement that can be promoted)
	supplement := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      filepath.Join(bookDir, "supplement.cbz"),
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 100,
	}
	err = tc.bookService.CreateFile(tc.ctx, supplement)
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1, "only main files are returned")

	// No main files in scannedPaths = all main files orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book should still exist
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)

	// Main file should be deleted, supplement should remain and be promoted
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, supplement.ID, remainingFiles[0].ID)
	assert.Equal(t, models.FileRoleMain, remainingFiles[0].FileRole)
}
```

- [ ] **Step 3: Add test for no orphans (no-op)**

```go
func TestCleanupOrphanedFiles_NoOrphans(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Healthy Book")
	testgen.GenerateEPUB(t, bookDir, "file.epub", testgen.EPUBOptions{
		Title:   "Healthy Book",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// All files are in scannedPaths — no orphans
	scannedPaths := make(map[string]struct{})
	for _, f := range existingFiles {
		scannedPaths[f.Filepath] = struct{}{}
	}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Everything should remain unchanged
	assert.Len(t, tc.listBooks(), 1)
	assert.Len(t, tc.listFiles(), 1)
}
```

- [ ] **Step 4: Run all orphan cleanup tests**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/worker/ -run "TestCleanupOrphanedFiles" -v -count=1`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan_orphans_test.go
git commit -m "[Test] Add comprehensive tests for batch orphan file cleanup"
```

---

### Task 6: Wire `cleanupOrphanedFiles` into scan.go

**Files:**
- Modify: `pkg/worker/scan.go` (lines 417-434)

- [ ] **Step 1: Replace the orphan cleanup loop**

In `pkg/worker/scan.go`, replace lines 417-434:

```go
		// Cleanup orphaned files (in DB but not on disk)
		// Uses the pre-loaded files from before the scan to avoid a second DB query
		if existingFiles != nil {
			scannedPaths := make(map[string]struct{}, len(filesToScan))
			for _, path := range filesToScan {
				scannedPaths[path] = struct{}{}
			}

			for _, file := range existingFiles {
				if _, seen := scannedPaths[file.Filepath]; !seen {
					jobLog.Info("cleaning up orphaned file", logger.Data{"file_id": file.ID, "filepath": file.Filepath})
					_, err := w.scanInternal(ctx, ScanOptions{FileID: file.ID}, nil)
					if err != nil {
						jobLog.Warn("failed to cleanup orphaned file", logger.Data{"file_id": file.ID, "error": err.Error()})
					}
				}
			}
		}
```

With:

```go
		// Cleanup orphaned files (in DB but not on disk) using batch operations.
		// Uses the pre-loaded files from before the scan to avoid a second DB query.
		if existingFiles != nil {
			scannedPaths := make(map[string]struct{}, len(filesToScan))
			for _, path := range filesToScan {
				scannedPaths[path] = struct{}{}
			}
			w.cleanupOrphanedFiles(ctx, existingFiles, scannedPaths, library, jobLog)
		}
```

- [ ] **Step 2: Run existing scan tests to verify no regressions**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/worker/ -v -count=1 -timeout=120s`

Expected: All existing tests PASS

- [ ] **Step 3: Run the full check suite**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && make check:quiet`

Expected: All checks pass

- [ ] **Step 4: Commit**

```bash
git add pkg/worker/scan.go
git commit -m "[Backend] Replace sequential orphan cleanup loop with batch operations"
```

---

### Task 7: Final verification and cleanup

- [ ] **Step 1: Run all new tests together**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/books/ -run "TestDelete|TestPromote" -v -count=1 && go test ./pkg/worker/ -run "TestCleanupOrphanedFiles" -v -count=1`

Expected: All PASS

- [ ] **Step 2: Run the full test suite with race detection**

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && make check:quiet`

Expected: All checks pass

- [ ] **Step 3: Verify the existing scan tests still pass (regression check)**

Run tests that exercise the full scan-then-rescan flow, specifically tests that delete files then rescan:

Run: `cd /Users/robinjoseph/.t3/worktrees/shisho/t3code-ad631582 && go test ./pkg/worker/ -run "TestScanFileByID_MissingFile" -v -count=1`

Expected: All PASS. These tests use `scanInternal` directly (single-file resync) which is unchanged. The batch path is only used during full library scans.

- [ ] **Step 4: Final commit (if any cleanup needed)**

Only if adjustments were needed in previous steps. Otherwise, no commit needed.
