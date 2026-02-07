# Primary File Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "primary file" concept to books so only one file per book syncs to Kobo devices.

**Architecture:** Each book has a `primary_file_id` foreign key pointing to one of its files. When books are created, the first file becomes primary. When the primary file is deleted, another file is auto-promoted. Users can manually change the primary via a new API endpoint. Kobo sync queries filter by primary file instead of file_role.

**Tech Stack:** Go/Echo/Bun (backend), React/TypeScript/TanStack Query (frontend), SQLite (database)

---

## Task 1: Database Migration

**Files:**
- Create: `pkg/migrations/20260202000000_add_primary_file.go`

**Step 1: Create the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		// Add the column as nullable first
		_, err := db.Exec(`ALTER TABLE books ADD COLUMN primary_file_id INTEGER REFERENCES files(id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Populate primary_file_id for all existing books
		// Prefer main files over supplements, ordered by created_at
		_, err = db.Exec(`
			UPDATE books SET primary_file_id = (
				SELECT id FROM files
				WHERE files.book_id = books.id
				ORDER BY
					CASE WHEN file_role = 'main' THEN 0 ELSE 1 END,
					created_at ASC
				LIMIT 1
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE books DROP COLUMN primary_file_id`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run the migration**

```bash
make db:migrate
```

Expected: Migration runs successfully, no errors.

**Step 3: Verify migration applied**

```bash
sqlite3 tmp/data.sqlite ".schema books" | grep primary_file_id
```

Expected: Shows `primary_file_id INTEGER REFERENCES files(id)`

**Step 4: Commit**

```bash
git add pkg/migrations/20260202000000_add_primary_file.go
git commit -m "$(cat <<'EOF'
[Backend] Add primary_file_id column to books table

Adds migration to store which file is the "primary" for Kobo sync.
Existing books get their oldest main file set as primary.
EOF
)"
```

---

## Task 2: Update Book Model

**Files:**
- Modify: `pkg/models/book.go:9-34`

**Step 1: Add PrimaryFileID field to Book struct**

In `pkg/models/book.go`, add the new field after the `TagSource` field (line 32):

```go
type Book struct {
	bun.BaseModel `bun:"table:books,alias:b" tstype:"-"`

	ID                int           `bun:",pk,nullzero" json:"id"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	LibraryID         int           `bun:",nullzero" json:"library_id"`
	Library           *Library      `bun:"rel:belongs-to" json:"library" tstype:"Library"`
	Filepath          string        `bun:",nullzero" json:"filepath"`
	Title             string        `bun:",nullzero" json:"title"`
	TitleSource       string        `bun:",nullzero" json:"title_source" tstype:"DataSource"`
	SortTitle         string        `bun:",notnull" json:"sort_title"`
	SortTitleSource   string        `bun:",notnull" json:"sort_title_source" tstype:"DataSource"`
	Subtitle          *string       `json:"subtitle"`
	SubtitleSource    *string       `json:"subtitle_source" tstype:"DataSource"`
	Description       *string       `json:"description"`
	DescriptionSource *string       `json:"description_source" tstype:"DataSource"`
	Authors           []*Author     `bun:"rel:has-many,join:id=book_id" json:"authors,omitempty" tstype:"Author[]"`
	AuthorSource      string        `bun:",nullzero" json:"author_source" tstype:"DataSource"`
	BookSeries        []*BookSeries `bun:"rel:has-many,join:id=book_id" json:"book_series,omitempty" tstype:"BookSeries[]"`
	BookGenres        []*BookGenre  `bun:"rel:has-many,join:id=book_id" json:"book_genres,omitempty" tstype:"BookGenre[]"`
	GenreSource       *string       `json:"genre_source" tstype:"DataSource"`
	BookTags          []*BookTag    `bun:"rel:has-many,join:id=book_id" json:"book_tags,omitempty" tstype:"BookTag[]"`
	TagSource         *string       `json:"tag_source" tstype:"DataSource"`
	PrimaryFileID     *int          `json:"primary_file_id"`
	Files             []*File       `bun:"rel:has-many" json:"files" tstype:"File[]"`
}
```

Note: Using `*int` (nullable pointer) because the column allows NULL in SQLite (we populated it for existing books, but the constraint isn't NOT NULL).

**Step 2: Regenerate TypeScript types**

```bash
make tygo
```

Expected: Types regenerated (may say "Nothing to be done" if already up-to-date from `make start`).

**Step 3: Verify the Go code compiles**

```bash
go build ./...
```

Expected: No compilation errors.

**Step 4: Commit**

```bash
git add pkg/models/book.go
git commit -m "$(cat <<'EOF'
[Backend] Add PrimaryFileID field to Book model
EOF
)"
```

---

## Task 3: Backend - Set Primary on First File Creation

**Files:**
- Modify: `pkg/books/service.go:413-433` (CreateFile function)

**Step 1: Write test for primary file auto-assignment**

Create test in `pkg/books/service_primary_file_test.go`:

```go
package books

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFile_SetsPrimaryForFirstFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)
	assert.Nil(t, book.PrimaryFileID)

	// Create the first file
	file := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
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
	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create the first file
	file1 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file1.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	// Create a second file
	file2 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file2.pdf",
		FileType:  "pdf",
		FileRole:  models.FileRoleMain,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Reload the book - primary should still be first file
	err = db.NewSelect().Model(book).Where("id = ?", book.ID).Scan(ctx)
	require.NoError(t, err)
	require.NotNil(t, book.PrimaryFileID)
	assert.Equal(t, file1.ID, *book.PrimaryFileID, "Primary should remain the first file")
}
```

**Step 2: Run the test to verify it fails**

```bash
go test -v ./pkg/books/... -run TestCreateFile_SetsPrimaryForFirstFile
```

Expected: FAIL - book.PrimaryFileID is nil after CreateFile.

**Step 3: Update CreateFile to set primary for first file**

In `pkg/books/service.go`, modify the `CreateFile` function (around line 413):

```go
func (svc *Service) CreateFile(ctx context.Context, file *models.File) error {
	now := time.Now()
	if file.CreatedAt.IsZero() {
		file.CreatedAt = now
	}
	file.UpdatedAt = file.CreatedAt

	// Insert file.
	_, err := svc.db.
		NewInsert().
		Model(file).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	// If this is the first file for the book, set it as primary
	var book models.Book
	err = svc.db.NewSelect().
		Model(&book).
		Where("id = ?", file.BookID).
		Scan(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if book.PrimaryFileID == nil {
		book.PrimaryFileID = &file.ID
		_, err = svc.db.NewUpdate().
			Model(&book).
			Column("primary_file_id").
			Where("id = ?", book.ID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	// Note: FileNarrators are created separately via CreateFileNarrator after person creation

	return nil
}
```

**Step 4: Run the tests to verify they pass**

```bash
go test -v ./pkg/books/... -run TestCreateFile_SetsPrimary
```

Expected: Both tests PASS.

**Step 5: Run full test suite to check for regressions**

```bash
make test
```

Expected: All tests pass.

**Step 6: Commit**

```bash
git add pkg/books/service.go pkg/books/service_primary_file_test.go
git commit -m "$(cat <<'EOF'
[Backend] Set primary file when first file is created for a book
EOF
)"
```

---

## Task 4: Backend - Auto-Promote Primary on File Deletion

**Files:**
- Modify: `pkg/books/service.go:1233-1265` (DeleteFile function)

**Step 1: Write test for primary file promotion**

Add to `pkg/books/service_primary_file_test.go`:

```go
func TestDeleteFile_PromotesPrimaryWhenPrimaryDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files - first becomes primary
	file1 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file1.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now(),
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file2.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now().Add(time.Second), // Slightly later
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
	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create primary file (main)
	filePrimary := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/primary.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now(),
	}
	err = svc.CreateFile(ctx, filePrimary)
	require.NoError(t, err)

	// Create supplement (older timestamp to test priority logic)
	fileSupplement := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/supplement.pdf",
		FileType:  "pdf",
		FileRole:  models.FileRoleSupplement,
		CreatedAt: time.Now().Add(-time.Hour), // Older
	}
	err = svc.CreateFile(ctx, fileSupplement)
	require.NoError(t, err)

	// Create another main file (newest)
	fileMain2 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/main2.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now().Add(time.Second),
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
	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)

	// Create a library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files
	file1 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file1.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now(),
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file2.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
		CreatedAt: time.Now().Add(time.Second),
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
```

**Step 2: Run the tests to verify they fail**

```bash
go test -v ./pkg/books/... -run TestDeleteFile_Promotes
```

Expected: FAIL - primary is not updated after deletion.

**Step 3: Update DeleteFile to handle primary file promotion**

In `pkg/books/service.go`, replace the `DeleteFile` function:

```go
// DeleteFile deletes a file and its associated records (narrators, identifiers).
// If the deleted file was the book's primary file, promotes another file to primary.
func (svc *Service) DeleteFile(ctx context.Context, fileID int) error {
	return svc.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Get the file to find its book_id
		var file models.File
		err := tx.NewSelect().
			Model(&file).
			Where("id = ?", fileID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete narrators for this file
		_, err = tx.NewDelete().
			Model((*models.Narrator)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete identifiers for this file
		_, err = tx.NewDelete().
			Model((*models.FileIdentifier)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Delete the file record
		_, err = tx.NewDelete().
			Model((*models.File)(nil)).
			Where("id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Check if this was the primary file and promote another if needed
		var book models.Book
		err = tx.NewSelect().
			Model(&book).
			Where("id = ?", file.BookID).
			Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if book.PrimaryFileID != nil && *book.PrimaryFileID == fileID {
			// Find the next primary: prefer main files, then oldest
			var newPrimary models.File
			err = tx.NewSelect().
				Model(&newPrimary).
				Where("book_id = ?", file.BookID).
				Where("id != ?", fileID).
				OrderExpr("CASE WHEN file_role = ? THEN 0 ELSE 1 END", models.FileRoleMain).
				Order("created_at ASC").
				Limit(1).
				Scan(ctx)
			if err == nil {
				// Found a file to promote
				_, err = tx.NewUpdate().
					Model(&book).
					Set("primary_file_id = ?", newPrimary.ID).
					Where("id = ?", book.ID).
					Exec(ctx)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			// If no files remain, the book deletion cascade will handle cleanup
		}

		return nil
	})
}
```

**Step 4: Run the tests to verify they pass**

```bash
go test -v ./pkg/books/... -run TestDeleteFile
```

Expected: All tests PASS.

**Step 5: Run full test suite**

```bash
make test
```

Expected: All tests pass.

**Step 6: Commit**

```bash
git add pkg/books/service.go pkg/books/service_primary_file_test.go
git commit -m "$(cat <<'EOF'
[Backend] Auto-promote new primary file when primary is deleted

Prefers main files over supplements, ordered by created_at.
EOF
)"
```

---

## Task 5: Backend - New Endpoint to Set Primary File

**Files:**
- Modify: `pkg/books/handlers.go` (add new handler)
- Modify: `pkg/books/routes.go` (register route)

**Step 1: Write test for the new endpoint**

Create `pkg/books/handlers_primary_file_test.go`:

```go
package books

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPrimaryFile(t *testing.T) {
	t.Parallel()

	db := testhelpers.SetupTestDB(t)
	svc := NewService(db)
	ctx := t.Context()

	// Create library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create two files
	file1 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file1.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	err = svc.CreateFile(ctx, file1)
	require.NoError(t, err)

	file2 := &models.File{
		LibraryID: library.ID,
		BookID:    book.ID,
		Filepath:  "/tmp/test/book/file2.epub",
		FileType:  models.FileTypeEPUB,
		FileRole:  models.FileRoleMain,
	}
	err = svc.CreateFile(ctx, file2)
	require.NoError(t, err)

	// Create admin user
	user := testhelpers.CreateAdminUser(t, db)

	// Setup handler
	h := &handler{bookService: svc}
	e := echo.New()

	t.Run("sets primary file successfully", func(t *testing.T) {
		t.Parallel()

		payload := map[string]int{"file_id": file2.ID}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(testhelpers.Itoa(book.ID))
		c.Set("user", user)

		err := h.setPrimaryFile(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify the book was updated
		var updatedBook models.Book
		err = db.NewSelect().Model(&updatedBook).Where("id = ?", book.ID).Scan(ctx)
		require.NoError(t, err)
		require.NotNil(t, updatedBook.PrimaryFileID)
		assert.Equal(t, file2.ID, *updatedBook.PrimaryFileID)
	})

	t.Run("rejects file from different book", func(t *testing.T) {
		t.Parallel()

		// Create another book with a file
		otherBook := &models.Book{
			LibraryID:       library.ID,
			Filepath:        "/tmp/test/other",
			Title:           "Other Book",
			SortTitle:       "Other Book",
			TitleSource:     models.DataSourceFilepath,
			SortTitleSource: models.DataSourceFilepath,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}
		_, err := db.NewInsert().Model(otherBook).Exec(ctx)
		require.NoError(t, err)

		otherFile := &models.File{
			LibraryID: library.ID,
			BookID:    otherBook.ID,
			Filepath:  "/tmp/test/other/file.epub",
			FileType:  models.FileTypeEPUB,
			FileRole:  models.FileRoleMain,
		}
		err = svc.CreateFile(ctx, otherFile)
		require.NoError(t, err)

		// Try to set other book's file as primary for our book
		payload := map[string]int{"file_id": otherFile.ID}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(testhelpers.Itoa(book.ID))
		c.Set("user", user)

		err = h.setPrimaryFile(c)
		require.Error(t, err)

		// Should be a 400 Bad Request
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
	})
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./pkg/books/... -run TestSetPrimaryFile
```

Expected: FAIL - handler does not exist.

**Step 3: Add the handler**

In `pkg/books/handlers.go`, add the new handler (add near other book handlers, e.g., after `update`):

```go
// SetPrimaryFilePayload is the request body for setting a book's primary file.
type SetPrimaryFilePayload struct {
	FileID int `json:"file_id"`
}

func (h *handler) setPrimaryFile(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	// Bind params
	params := SetPrimaryFilePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Fetch the book
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{
		ID: &id,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Verify the file belongs to this book
	fileBelongsToBook := false
	for _, f := range book.Files {
		if f.ID == params.FileID {
			fileBelongsToBook = true
			break
		}
	}
	if !fileBelongsToBook {
		return errcodes.BadRequest("File does not belong to this book")
	}

	// Update the book's primary file
	book.PrimaryFileID = &params.FileID
	err = h.bookService.UpdateBook(ctx, book, UpdateBookOptions{
		Columns: []string{"primary_file_id"},
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, book)
}
```

**Step 4: Register the route**

In `pkg/books/routes.go`, add after line 64 (after `g.GET("/:id/lists", h.bookLists)`):

```go
	g.PUT("/:id/primary-file", h.setPrimaryFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

**Step 5: Run tests to verify they pass**

```bash
go test -v ./pkg/books/... -run TestSetPrimaryFile
```

Expected: All tests PASS.

**Step 6: Run full test suite**

```bash
make test
```

Expected: All tests pass.

**Step 7: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/books/handlers_primary_file_test.go
git commit -m "$(cat <<'EOF'
[Backend] Add PUT /books/:id/primary-file endpoint

Allows users to manually set which file is primary for Kobo sync.
EOF
)"
```

---

## Task 6: Backend - Update Kobo Sync Query

**Files:**
- Modify: `pkg/kobo/service.go:177-239` (GetScopedFiles function)

**Step 1: Write test for Kobo sync filtering by primary file**

Create `pkg/kobo/service_primary_file_test.go`:

```go
package kobo

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetScopedFiles_OnlyReturnsPrimaryFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testhelpers.SetupTestDB(t)
	bookSvc := books.NewService(db)
	koboSvc := NewService(db)

	// Create library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with library access
	user := testhelpers.CreateAdminUser(t, db)
	_, err = db.NewInsert().Model(&models.LibraryAccess{
		UserID:    user.ID,
		LibraryID: library.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
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

	// Verify file1 is primary
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
	db := testhelpers.SetupTestDB(t)
	bookSvc := books.NewService(db)
	koboSvc := NewService(db)

	// Create library
	library := &models.Library{
		Name:      "Test Library",
		Path:      "/tmp/test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create user with library access
	user := testhelpers.CreateAdminUser(t, db)
	_, err = db.NewInsert().Model(&models.LibraryAccess{
		UserID:    user.ID,
		LibraryID: library.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Exec(ctx)
	require.NoError(t, err)

	// Create book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/tmp/test/book",
		Title:           "Test Book",
		SortTitle:       "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitleSource: models.DataSourceFilepath,
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
```

**Step 2: Run tests to verify they fail**

```bash
go test -v ./pkg/kobo/... -run TestGetScopedFiles_OnlyReturnsPrimaryFiles
```

Expected: FAIL - returns both files.

**Step 3: Update GetScopedFiles to filter by primary file**

In `pkg/kobo/service.go`, modify the `GetScopedFiles` function. Replace line 198:

```go
Where("f.file_role = ?", models.FileRoleMain)
```

With:

```go
Join("JOIN books AS b ON b.id = f.book_id").
Where("f.id = b.primary_file_id")
```

The updated query section (lines 191-199) should look like:

```go
	// Query files with relations.
	var files []models.File
	q := svc.db.NewSelect().
		Model(&files).
		Relation("Book").
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries.Series").
		Relation("Publisher").
		Where("f.file_type IN (?)", bun.In([]string{models.FileTypeEPUB, models.FileTypeCBZ})).
		Join("JOIN books AS b ON b.id = f.book_id").
		Where("f.id = b.primary_file_id")
```

**Step 4: Run tests to verify they pass**

```bash
go test -v ./pkg/kobo/... -run TestGetScopedFiles
```

Expected: All tests PASS.

**Step 5: Run full test suite**

```bash
make test
```

Expected: All tests pass.

**Step 6: Commit**

```bash
git add pkg/kobo/service.go pkg/kobo/service_primary_file_test.go
git commit -m "$(cat <<'EOF'
[Backend] Update Kobo sync to use primary_file_id instead of file_role

Only syncs the designated primary file per book to prevent duplicates.
EOF
)"
```

---

## Task 7: Frontend - Add Primary File Indicator

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add isPrimary prop to FileRowProps**

In `app/components/pages/BookDetail.tsx`, update `FileRowProps` interface (around line 130):

```typescript
interface FileRowProps {
  file: File;
  libraryId: string;
  libraryDownloadPreference: string | undefined;
  isExpanded: boolean;
  hasExpandableMetadata: boolean;
  onToggleExpand: () => void;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadKepub: () => void;
  onDownloadOriginal: () => void;
  onDownloadWithEndpoint: (endpoint: string) => void;
  onCancelDownload: () => void;
  onEdit: () => void;
  onScanMetadata: () => void;
  onRefreshMetadata: () => void;
  isResyncing: boolean;
  isSupplement?: boolean;
  isSelectMode?: boolean;
  isFileSelected?: boolean;
  onToggleSelect?: () => void;
  onMoveFile?: () => void;
  cacheBuster?: number;
  isPrimary?: boolean;
  showPrimaryBadge?: boolean;
  onSetPrimary?: () => void;
}
```

**Step 2: Add isPrimary, showPrimaryBadge, and onSetPrimary to FileRow destructure**

Update the FileRow function parameters (around line 155):

```typescript
const FileRow = ({
  file,
  libraryId,
  libraryDownloadPreference,
  isExpanded,
  hasExpandableMetadata,
  onToggleExpand,
  isDownloading,
  onDownload,
  onDownloadKepub,
  onDownloadOriginal,
  onDownloadWithEndpoint,
  onCancelDownload,
  onEdit,
  onScanMetadata,
  onRefreshMetadata,
  isResyncing,
  isSupplement = false,
  isSelectMode = false,
  isFileSelected = false,
  onToggleSelect,
  onMoveFile,
  cacheBuster,
  isPrimary = false,
  showPrimaryBadge = false,
  onSetPrimary,
}: FileRowProps) => {
```

**Step 3: Add Star icon import**

At the top of the file, add `Star` to the lucide-react imports:

```typescript
import {
  ArrowLeft,
  ArrowRightLeft,
  Check,
  ChevronDown,
  ChevronRight,
  Download,
  Edit,
  ExternalLink,
  FileAudio,
  FileText,
  GitMerge,
  Image,
  List,
  Loader2,
  MoreVertical,
  RefreshCw,
  Star,
  X,
} from "lucide-react";
```

**Step 4: Add primary badge display in FileRow**

Inside the FileRow component, after the file type badge (around line 240-250, look for where file_type is displayed), add the primary indicator:

Find this section (the file type and title area):
```typescript
          <Link
            className="hover:underline"
            to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}`}
          >
            {formatFileTypeLabel(file.file_type)}
          </Link>
```

And add the primary badge after it:

```typescript
          <Link
            className="hover:underline"
            to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}`}
          >
            {formatFileTypeLabel(file.file_type)}
          </Link>
          {showPrimaryBadge && isPrimary && (
            <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-500">
              <Star className="h-3 w-3 fill-current" />
              Primary
            </span>
          )}
```

**Step 5: Add "Set as primary" menu item**

In the FileRow component, find the DropdownMenuContent section for main files (not supplements). Look for the section with "Move to another book" (around line 388-395).

Add the "Set as primary" option before the "Move to another book" section:

```typescript
                {onSetPrimary && !isPrimary && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={onSetPrimary}>
                      <Star className="h-4 w-4 mr-2" />
                      Set as primary
                    </DropdownMenuItem>
                  </>
                )}
                {onMoveFile && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem onClick={onMoveFile}>
                      <ArrowRightLeft className="h-4 w-4 mr-2" />
                      Move to another book
                    </DropdownMenuItem>
                  </>
                )}
```

**Step 6: Pass the new props when rendering FileRow for main files**

Find where FileRow is rendered for main files (around line 1260). Update it to include the new props:

```typescript
                {mainFiles.map((file) => (
                  <FileRow
                    cacheBuster={coverCacheBuster}
                    file={file}
                    hasExpandableMetadata={hasExpandableMetadata(file)}
                    isDownloading={downloadingFileId === file.id}
                    isExpanded={expandedFileIds.has(file.id)}
                    isFileSelected={selectedFileIds.has(file.id)}
                    isPrimary={book.primary_file_id === file.id}
                    isResyncing={resyncingFileId === file.id}
                    isSelectMode={isFileSelectMode}
                    key={file.id}
                    libraryDownloadPreference={
                      libraryQuery.data?.download_format_preference
                    }
                    libraryId={libraryId!}
                    onCancelDownload={handleCancelDownload}
                    onDownload={() => handleDownload(file.id, file.file_type)}
                    onDownloadKepub={() => handleDownloadKepub(file.id)}
                    onDownloadOriginal={() => handleDownloadOriginal(file.id)}
                    onDownloadWithEndpoint={(endpoint) =>
                      handleDownloadWithEndpoint(file.id, endpoint)
                    }
                    onEdit={() => setEditingFile(file)}
                    onMoveFile={() => setSingleFileMoveId(file.id)}
                    onRefreshMetadata={() => handleRefreshFileMetadata(file.id)}
                    onScanMetadata={() => handleScanFileMetadata(file.id)}
                    onSetPrimary={
                      mainFiles.length > 1
                        ? () => handleSetPrimaryFile(file.id)
                        : undefined
                    }
                    onToggleExpand={() => toggleFileExpanded(file.id)}
                    onToggleSelect={() => toggleFileSelection(file.id)}
                    showPrimaryBadge={mainFiles.length > 1}
                  />
                ))}
```

**Step 7: Verify TypeScript compiles**

```bash
yarn lint:types
```

Expected: No type errors.

**Step 8: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Add primary file indicator and menu item

Shows star badge on primary file when book has multiple files.
Adds "Set as primary" option in file overflow menu.
EOF
)"
```

---

## Task 8: Frontend - Implement Set Primary File Mutation

**Files:**
- Modify: `app/hooks/queries/books.ts`
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add useSetPrimaryFile mutation**

In `app/hooks/queries/books.ts`, add the new mutation (after `useMergeBooks`):

```typescript
interface SetPrimaryFileMutationVariables {
  bookId: number;
  fileId: number;
}

export const useSetPrimaryFile = () => {
  const queryClient = useQueryClient();

  return useMutation<Book, ShishoAPIError, SetPrimaryFileMutationVariables>({
    mutationFn: ({ bookId, fileId }) => {
      return API.request("PUT", `/books/${bookId}/primary-file`, { file_id: fileId }, null);
    },
    onSuccess: (data: Book) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.setQueryData([QueryKey.RetrieveBook, String(data.id)], data);
    },
  });
};
```

**Step 2: Import and use the mutation in BookDetail**

In `app/components/pages/BookDetail.tsx`, add the import:

```typescript
import {
  QueryKey,
  useBook,
  useMergeBooks,
  useMoveFiles,
  useSetPrimaryFile,
  useUpdateBook,
  useUpdateFile,
  useUploadFileCover,
} from "@/hooks/queries/books";
```

**Step 3: Add mutation hook and handler in BookDetail component**

Inside the BookDetail component (after the other mutation hooks like `const moveFilesMutation = useMoveFiles();`), add:

```typescript
  const setPrimaryFileMutation = useSetPrimaryFile();

  const handleSetPrimaryFile = (fileId: number) => {
    if (!book) return;
    setPrimaryFileMutation.mutate(
      { bookId: book.id, fileId },
      {
        onSuccess: () => {
          toast.success("Primary file updated");
        },
        onError: (error) => {
          toast.error(error.message || "Failed to set primary file");
        },
      }
    );
  };
```

**Step 4: Verify the frontend builds**

```bash
yarn build
```

Expected: Build succeeds with no errors.

**Step 5: Verify linting passes**

```bash
yarn lint
```

Expected: No lint errors.

**Step 6: Commit**

```bash
git add app/hooks/queries/books.ts app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Frontend] Implement set primary file mutation and handler
EOF
)"
```

---

## Task 9: Run Full Validation

**Step 1: Run all backend tests**

```bash
make test
```

Expected: All tests pass.

**Step 2: Run frontend linting**

```bash
yarn lint
```

Expected: No errors.

**Step 3: Run full check**

```bash
make check
```

Expected: All checks pass.

**Step 4: Manual testing checklist**

- [ ] Start the app with `make start`
- [ ] View a book with multiple files - primary badge should show
- [ ] View a book with single file - no primary badge should show
- [ ] Click "Set as primary" on a non-primary file - it should become primary
- [ ] Delete the primary file - another file should be promoted
- [ ] Verify Kobo sync only shows primary files (requires Kobo device or API testing)

---

## Task 10: Final Commit (if needed)

If any fixes were made during validation:

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Fix] Address issues found during primary file validation
EOF
)"
```

---

## Summary

This plan implements the primary file feature in 9 main tasks:

1. **Migration** - Adds `primary_file_id` column and populates existing data
2. **Model** - Adds `PrimaryFileID` field to Book struct
3. **CreateFile** - Auto-sets primary when first file is created
4. **DeleteFile** - Auto-promotes new primary when primary is deleted
5. **API Endpoint** - New `PUT /books/:id/primary-file` endpoint
6. **Kobo Sync** - Filters by `primary_file_id` instead of `file_role`
7. **Frontend Badge** - Shows primary indicator for multi-file books
8. **Frontend Mutation** - Implements the "Set as primary" action
9. **Validation** - Full test suite and manual testing
