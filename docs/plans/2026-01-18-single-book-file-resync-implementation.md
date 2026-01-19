# Single Book/File Resync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add API endpoints and UI controls to resync metadata for individual books and files, with support for both priority-respecting scans and force-refresh modes.

**Architecture:** The implementation extracts reusable scanning logic from the existing `ProcessScanJob` into a new `ScanSingleFile` function. Handler endpoints call this directly (no job queue) and return updated models. The frontend adds context menu items to BookItem and FileRow components.

**Tech Stack:** Go/Echo (backend), React/TypeScript with Tanstack Query (frontend), SQLite/Bun ORM (database)

---

## Task 1: Add `forceRefresh` Parameter to Priority Functions

**Files:**
- Modify: `pkg/worker/scan_helpers.go:1-50`
- Test: `pkg/worker/scan_helpers_test.go`

**Step 1: Write the failing tests for `shouldUpdateScalar` with `forceRefresh`**

Add these test cases to `TestShouldUpdateScalar` in `pkg/worker/scan_helpers_test.go`:

```go
{
	name:           "forceRefresh updates even with lower priority source",
	newValue:       "New Title",
	existingValue:  "Manual Title",
	newSource:      models.DataSourceFilepath,
	existingSource: models.DataSourceManual,
	forceRefresh:   true,
	want:           true,
},
{
	name:           "forceRefresh still skips empty new value",
	newValue:       "",
	existingValue:  "Manual Title",
	newSource:      models.DataSourceFilepath,
	existingSource: models.DataSourceManual,
	forceRefresh:   true,
	want:           false,
},
```

Update the test struct and loop to pass `forceRefresh`:

```go
tests := []struct {
	name           string
	newValue       string
	existingValue  string
	newSource      string
	existingSource string
	forceRefresh   bool  // Add this field
	want           bool
}{
	// ... existing tests with forceRefresh: false ...
}

for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		got := shouldUpdateScalar(tt.newValue, tt.existingValue, tt.newSource, tt.existingSource, tt.forceRefresh)
		assert.Equal(t, tt.want, got)
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/worker/... -run TestShouldUpdateScalar -v`
Expected: FAIL (wrong number of arguments)

**Step 3: Update `shouldUpdateScalar` signature and implementation**

In `pkg/worker/scan_helpers.go`, update the function:

```go
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string, forceRefresh bool) bool {
	if forceRefresh {
		return newValue != ""
	}
	// ... existing priority logic unchanged ...
}
```

**Step 4: Update all call sites to pass `false` for `forceRefresh`**

Search for `shouldUpdateScalar(` in `pkg/worker/scan.go` and add `, false` to each call.

**Step 5: Run tests to verify they pass**

Run: `go test ./pkg/worker/... -run TestShouldUpdateScalar -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/worker/scan_helpers.go pkg/worker/scan_helpers_test.go pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Resync] Add forceRefresh parameter to shouldUpdateScalar

The forceRefresh flag bypasses priority checks, allowing metadata to be
overwritten regardless of source priority. Empty values are still skipped.
EOF
)"
```

---

## Task 2: Add `forceRefresh` Parameter to `shouldUpdateRelationship`

**Files:**
- Modify: `pkg/worker/scan_helpers.go`
- Test: `pkg/worker/scan_helpers_test.go`

**Step 1: Write the failing tests**

Add to `TestShouldUpdateRelationship` test cases:

```go
{
	name:           "forceRefresh updates even with lower priority source",
	newItems:       []string{"New Author"},
	existingItems:  []string{"Manual Author"},
	newSource:      models.DataSourceFilepath,
	existingSource: models.DataSourceManual,
	forceRefresh:   true,
	want:           true,
},
{
	name:           "forceRefresh still skips empty new items",
	newItems:       []string{},
	existingItems:  []string{"Manual Author"},
	newSource:      models.DataSourceFilepath,
	existingSource: models.DataSourceManual,
	forceRefresh:   true,
	want:           false,
},
```

Update test struct to include `forceRefresh bool` field and update the test loop.

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/worker/... -run TestShouldUpdateRelationship -v`
Expected: FAIL

**Step 3: Update `shouldUpdateRelationship` signature and implementation**

```go
func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string, forceRefresh bool) bool {
	if forceRefresh {
		return len(newItems) > 0
	}
	// ... existing priority logic unchanged ...
}
```

**Step 4: Update all call sites**

Search for `shouldUpdateRelationship(` in `pkg/worker/scan.go` and add `, false` to each call.

**Step 5: Run tests to verify they pass**

Run: `go test ./pkg/worker/... -run TestShouldUpdateRelationship -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/worker/scan_helpers.go pkg/worker/scan_helpers_test.go pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Resync] Add forceRefresh parameter to shouldUpdateRelationship

Completes the forceRefresh support for relationship updates.
EOF
)"
```

---

## Task 3: Extract `ScanSingleFile` Function

**Files:**
- Modify: `pkg/worker/scan.go`
- Create: `pkg/worker/resync.go`

**Step 1: Create the new resync.go file with the function signature**

Create `pkg/worker/resync.go`:

```go
package worker

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// ScanSingleFileOptions configures single file scanning behavior.
type ScanSingleFileOptions struct {
	ForceRefresh bool // Bypass priority checks when true
}

// ScanSingleFile rescans a single file, updating metadata for both the file and its parent book.
// Returns the updated file model. If the file no longer exists on disk, deletes the file record
// and returns nil (not an error).
func (w *Worker) ScanSingleFile(ctx context.Context, file *models.File, opts ScanSingleFileOptions) (*models.File, error) {
	// Check if file exists on disk
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		// File missing - delete from DB
		if err := w.bookService.DeleteFile(ctx, file.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete missing file record")
		}
		return nil, nil
	}

	// TODO: Extract scanning logic from scanFile method
	// For now, call the existing scanFile with forceRefresh support
	return nil, errors.New("not implemented")
}
```

**Step 2: Run build to verify it compiles**

Run: `go build ./...`
Expected: Build succeeds (function returns error for now)

**Step 3: Commit skeleton**

```bash
git add pkg/worker/resync.go
git commit -m "$(cat <<'EOF'
[Resync] Add ScanSingleFile skeleton

Prepares the API for single file rescanning with forceRefresh support.
EOF
)"
```

---

## Task 4: Implement `ScanSingleFile` Core Logic

**Files:**
- Modify: `pkg/worker/resync.go`
- Modify: `pkg/worker/scan.go` (extract shared helpers)

**Step 1: Extract metadata parsing helper**

The existing `scanFile` method is ~1300 lines. We need to extract the metadata parsing portion (lines 468-617 in scan.go) into a reusable helper.

Create a helper function in `pkg/worker/scan.go`:

```go
// parseFileMetadata extracts metadata from a file based on its type.
// Returns the parsed metadata and the data source identifier.
func parseFileMetadata(filepath, fileType string) (*mediafile.ParsedMetadata, string, error) {
	var metadata *mediafile.ParsedMetadata
	var dataSource string
	var err error

	switch fileType {
	case models.FileTypeEPUB:
		metadata, err = epub.Parse(filepath)
		dataSource = models.DataSourceEPUBMetadata
	case models.FileTypeCBZ:
		metadata, err = cbz.Parse(filepath)
		dataSource = models.DataSourceCBZMetadata
	case models.FileTypeM4B:
		metadata, err = mp4.Parse(filepath)
		dataSource = models.DataSourceM4BMetadata
	default:
		return nil, "", errors.Errorf("unsupported file type: %s", fileType)
	}

	if err != nil {
		return nil, "", errors.Wrap(err, "failed to parse file metadata")
	}

	return metadata, dataSource, nil
}
```

**Step 2: Implement `ScanSingleFile` using shared logic**

Update `pkg/worker/resync.go` with the full implementation:

```go
package worker

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// ScanSingleFileOptions configures single file scanning behavior.
type ScanSingleFileOptions struct {
	ForceRefresh bool // Bypass priority checks when true
}

// ScanSingleFileResult contains the results of a single file scan.
type ScanSingleFileResult struct {
	File        *models.File
	FileDeleted bool // True if file was deleted because it no longer exists on disk
	BookDeleted bool // True if parent book was also deleted (was last file)
}

// ScanSingleFile rescans a single file, updating metadata for both the file and its parent book.
// If the file no longer exists on disk, deletes the file record (and book if it was the last file).
func (w *Worker) ScanSingleFile(ctx context.Context, fileID int, opts ScanSingleFileOptions) (*ScanSingleFileResult, error) {
	log := logger.FromContext(ctx)

	// Retrieve file with relations
	file, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve file")
	}

	// Check if file exists on disk
	if _, err := os.Stat(file.Filepath); os.IsNotExist(err) {
		log.Info("file no longer exists on disk, deleting record", logger.Data{"file_id": file.ID, "path": file.Filepath})

		// Check if this is the last file in the book
		book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
		if err != nil {
			return nil, errors.Wrap(err, "failed to retrieve parent book")
		}

		bookDeleted := len(book.Files) == 1

		// Delete the file
		if err := w.bookService.DeleteFile(ctx, file.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete file record")
		}

		// If last file, delete the book too
		if bookDeleted {
			if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
				return nil, errors.Wrap(err, "failed to delete orphaned book")
			}
		}

		return &ScanSingleFileResult{
			FileDeleted: true,
			BookDeleted: bookDeleted,
		}, nil
	}

	// Parse file metadata
	metadata, dataSource, err := parseFileMetadata(file.Filepath, file.FileType)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse file metadata")
	}

	// Call the internal scan logic with forceRefresh
	// We reuse the existing scanFile logic but need to pass forceRefresh through
	// For now, we directly update the file and book

	// This is a simplification - the full implementation would extract more
	// of the scanFile logic into reusable helpers. For the initial implementation,
	// we call into the existing scan path.
	err = w.rescanFileWithMetadata(ctx, file, metadata, dataSource, opts.ForceRefresh)
	if err != nil {
		return nil, errors.Wrap(err, "failed to rescan file")
	}

	// Reload file with updated relations
	updatedFile, err := w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve updated file")
	}

	return &ScanSingleFileResult{
		File: updatedFile,
	}, nil
}
```

**Step 3: Add `rescanFileWithMetadata` helper**

This helper contains the core update logic extracted from scanFile, accepting forceRefresh:

```go
// rescanFileWithMetadata updates a file and its parent book with parsed metadata.
func (w *Worker) rescanFileWithMetadata(ctx context.Context, file *models.File, metadata *mediafile.ParsedMetadata, dataSource string, forceRefresh bool) error {
	log := logger.FromContext(ctx)

	// Get parent book
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})
	if err != nil {
		return errors.Wrap(err, "failed to retrieve parent book")
	}

	// Update book metadata using priority logic
	bookUpdateOpts := books.UpdateBookOptions{Columns: []string{}}

	// Title
	if shouldUpdateScalar(metadata.Title, book.Title, dataSource, book.TitleSource, forceRefresh) {
		book.Title = metadata.Title
		book.TitleSource = dataSource
		bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "title", "title_source")

		// Regenerate sort title
		book.SortTitle = generateSortTitle(metadata.Title)
		book.SortTitleSource = dataSource
		bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "sort_title", "sort_title_source")
	}

	// Subtitle
	if metadata.Subtitle != "" {
		existing := ""
		existingSource := ""
		if book.Subtitle != nil {
			existing = *book.Subtitle
		}
		if book.SubtitleSource != nil {
			existingSource = *book.SubtitleSource
		}
		if shouldUpdateScalar(metadata.Subtitle, existing, dataSource, existingSource, forceRefresh) {
			book.Subtitle = &metadata.Subtitle
			book.SubtitleSource = &dataSource
			bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "subtitle", "subtitle_source")
		}
	}

	// Description
	if metadata.Description != "" {
		existing := ""
		existingSource := ""
		if book.Description != nil {
			existing = *book.Description
		}
		if book.DescriptionSource != nil {
			existingSource = *book.DescriptionSource
		}
		if shouldUpdateScalar(metadata.Description, existing, dataSource, existingSource, forceRefresh) {
			book.Description = &metadata.Description
			book.DescriptionSource = &dataSource
			bookUpdateOpts.Columns = append(bookUpdateOpts.Columns, "description", "description_source")
		}
	}

	// Update book if there were changes
	if len(bookUpdateOpts.Columns) > 0 {
		if err := w.bookService.UpdateBook(ctx, book, bookUpdateOpts); err != nil {
			return errors.Wrap(err, "failed to update book")
		}
	}

	// Update authors relationship
	authorNames := make([]string, 0, len(metadata.Authors))
	for _, a := range metadata.Authors {
		authorNames = append(authorNames, a.Name)
	}
	existingAuthorNames := make([]string, 0, len(book.Authors))
	for _, a := range book.Authors {
		if a.Person != nil {
			existingAuthorNames = append(existingAuthorNames, a.Person.Name)
		}
	}

	if shouldUpdateRelationship(authorNames, existingAuthorNames, dataSource, book.AuthorSource, forceRefresh) {
		// Delete existing authors
		for _, author := range book.Authors {
			if err := w.bookService.DeleteAuthor(ctx, author.ID); err != nil {
				log.Warn("failed to delete author", logger.Data{"author_id": author.ID, "error": err.Error()})
			}
		}
		// Create new authors
		for i, parsedAuthor := range metadata.Authors {
			person, err := w.personService.FindOrCreatePerson(ctx, parsedAuthor.Name)
			if err != nil {
				return errors.Wrap(err, "failed to find or create person")
			}
			author := &models.Author{
				BookID:    book.ID,
				PersonID:  person.ID,
				Role:      parsedAuthor.Role,
				SortOrder: i,
			}
			if err := w.bookService.CreateAuthor(ctx, author); err != nil {
				return errors.Wrap(err, "failed to create author")
			}
		}
		book.AuthorSource = dataSource
		if err := w.bookService.UpdateBook(ctx, book, books.UpdateBookOptions{Columns: []string{"author_source"}}); err != nil {
			return errors.Wrap(err, "failed to update author source")
		}
	}

	// Update file metadata
	fileUpdateOpts := books.UpdateFileOptions{Columns: []string{}}

	// File name
	if file.FileType == models.FileTypeCBZ {
		newName := generateCBZFileName(metadata, filepath.Base(file.Filepath))
		existingName := ""
		existingSource := ""
		if file.Name != nil {
			existingName = *file.Name
		}
		if file.NameSource != nil {
			existingSource = *file.NameSource
		}
		if shouldUpdateScalar(newName, existingName, dataSource, existingSource, forceRefresh) {
			file.Name = &newName
			file.NameSource = &dataSource
			fileUpdateOpts.Columns = append(fileUpdateOpts.Columns, "name", "name_source")
		}
	}

	// Update file if there were changes
	if len(fileUpdateOpts.Columns) > 0 {
		if err := w.bookService.UpdateFile(ctx, file, fileUpdateOpts); err != nil {
			return errors.Wrap(err, "failed to update file")
		}
	}

	// Write sidecars
	if err := sidecar.WriteBookSidecarFromModel(book); err != nil {
		log.Warn("failed to write book sidecar", logger.Data{"error": err.Error()})
	}
	if err := sidecar.WriteFileSidecarFromModel(file); err != nil {
		log.Warn("failed to write file sidecar", logger.Data{"error": err.Error()})
	}

	return nil
}
```

**Step 4: Run build**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add pkg/worker/resync.go pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Resync] Implement ScanSingleFile core logic

Extracts metadata parsing and update logic into reusable helpers.
Supports forceRefresh to bypass priority checks.
EOF
)"
```

---

## Task 5: Add Resync Handler for Files

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/validators.go`
- Modify: `pkg/books/routes.go`

**Step 1: Add request payload type**

In `pkg/books/validators.go`, add:

```go
// ResyncPayload contains the request parameters for resync operations.
type ResyncPayload struct {
	Refresh bool `json:"refresh"`
}
```

**Step 2: Add ResyncFile handler**

In `pkg/books/handlers.go`, add:

```go
func (h *handler) resyncFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	params := ResyncPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Retrieve file to check library access
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// TODO: Need worker reference - this requires injecting worker into handler
	// For now, return not implemented
	return errcodes.InternalError("Resync not yet implemented")
}
```

**Step 3: Register route**

In `pkg/books/routes.go`, add after other file routes:

```go
g.POST("/files/:id/resync", h.resyncFile, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

**Step 4: Run build**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/validators.go pkg/books/routes.go
git commit -m "$(cat <<'EOF'
[Resync] Add ResyncFile endpoint skeleton

POST /books/files/:id/resync with { refresh: bool } payload.
EOF
)"
```

---

## Task 6: Wire Worker into Books Handler

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Modify: `cmd/api/main.go` (or wherever routes are registered)

The books handler needs access to the worker to call `ScanSingleFile`. This requires either:
1. Injecting the worker into the handler
2. Creating a separate resync service that wraps the worker

**Step 1: Add worker to handler struct**

In `pkg/books/handlers.go`, update the handler struct:

```go
type handler struct {
	bookService      *Service
	libraryService   *libraries.Service
	personService    *people.Service
	searchService    *search.Service
	genreService     *genres.Service
	tagService       *tags.Service
	publisherService *publishers.Service
	imprintService   *imprints.Service
	downloadCache    *downloadcache.Cache
	pageCache        *cbzpages.Cache
	worker           *worker.Worker  // Add this
}
```

**Step 2: Update route registration**

In `pkg/books/routes.go`, update `RegisterRoutesWithGroup` to accept worker:

```go
func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, cfg *config.Config, authMiddleware *auth.Middleware, w *worker.Worker) {
	// ... existing service creation ...

	h := &handler{
		// ... existing fields ...
		worker: w,
	}
	// ... routes ...
}
```

**Step 3: Update main.go to pass worker**

Find where `RegisterRoutesWithGroup` is called and pass the worker instance.

**Step 4: Implement resyncFile handler**

Update the handler to use the worker:

```go
func (h *handler) resyncFile(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	params := ResyncPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Retrieve file to check library access
	file, err := h.bookService.RetrieveFile(ctx, RetrieveFileOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Perform resync
	result, err := h.worker.ScanSingleFile(ctx, id, worker.ScanSingleFileOptions{
		ForceRefresh: params.Refresh,
	})
	if err != nil {
		log.Error("failed to resync file", logger.Data{"file_id": id, "error": err.Error()})
		return errcodes.UnprocessableEntity(err.Error())
	}

	// Handle deletion case
	if result.FileDeleted {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"deleted":      true,
			"book_deleted": result.BookDeleted,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.File))
}
```

**Step 5: Run build and test**

Run: `go build ./...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go cmd/api/main.go
git commit -m "$(cat <<'EOF'
[Resync] Wire worker into books handler for file resync

Completes the POST /books/files/:id/resync endpoint implementation.
EOF
)"
```

---

## Task 7: Add Resync Handler for Books

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`
- Modify: `pkg/worker/resync.go`

**Step 1: Add `ScanSingleBook` to worker**

In `pkg/worker/resync.go`:

```go
// ScanSingleBookResult contains the results of a single book scan.
type ScanSingleBookResult struct {
	Book        *models.Book
	BookDeleted bool // True if book was deleted because it has no files
}

// ScanSingleBook rescans a book and all its files.
// If the book has no files, deletes the book record.
func (w *Worker) ScanSingleBook(ctx context.Context, bookID int, opts ScanSingleFileOptions) (*ScanSingleBookResult, error) {
	log := logger.FromContext(ctx)

	// Retrieve book with files
	book, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve book")
	}

	// Check if book has files
	if len(book.Files) == 0 {
		log.Info("book has no files, deleting", logger.Data{"book_id": book.ID})
		if err := w.bookService.DeleteBook(ctx, book.ID); err != nil {
			return nil, errors.Wrap(err, "failed to delete orphaned book")
		}
		return &ScanSingleBookResult{BookDeleted: true}, nil
	}

	// Rescan each file
	for _, file := range book.Files {
		result, err := w.ScanSingleFile(ctx, file.ID, opts)
		if err != nil {
			log.Warn("failed to rescan file", logger.Data{"file_id": file.ID, "error": err.Error()})
			// Continue with other files
			continue
		}
		if result.BookDeleted {
			// Book was deleted during file resync (was last file)
			return &ScanSingleBookResult{BookDeleted: true}, nil
		}
	}

	// Reload book with updated data
	updatedBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &bookID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve updated book")
	}

	return &ScanSingleBookResult{Book: updatedBook}, nil
}
```

**Step 2: Add resyncBook handler**

In `pkg/books/handlers.go`:

```go
func (h *handler) resyncBook(c echo.Context) error {
	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	params := ResyncPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	// Retrieve book to check library access
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Check library access
	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(book.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Perform resync
	result, err := h.worker.ScanSingleBook(ctx, id, worker.ScanSingleFileOptions{
		ForceRefresh: params.Refresh,
	})
	if err != nil {
		log.Error("failed to resync book", logger.Data{"book_id": id, "error": err.Error()})
		return errcodes.UnprocessableEntity(err.Error())
	}

	// Handle deletion case
	if result.BookDeleted {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"deleted": true,
		})
	}

	return errors.WithStack(c.JSON(http.StatusOK, result.Book))
}
```

**Step 3: Register route**

In `pkg/books/routes.go`:

```go
g.POST("/:id/resync", h.resyncBook, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
```

**Step 4: Run build**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go pkg/worker/resync.go
git commit -m "$(cat <<'EOF'
[Resync] Add ResyncBook endpoint

POST /books/:id/resync rescans all files in a book.
EOF
)"
```

---

## Task 8: Add Frontend Mutation Hooks

**Files:**
- Create: `app/hooks/queries/resync.ts`
- Modify: `app/hooks/queries/books.ts`

**Step 1: Create resync mutation hooks**

Create `app/hooks/queries/resync.ts`:

```typescript
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { API } from "@/libraries/api";
import { Book, File } from "@/types";

import { BooksQueryKey } from "./books";

export interface ResyncPayload {
  refresh: boolean;
}

export interface ResyncFileResult {
  deleted?: boolean;
  book_deleted?: boolean;
}

export interface ResyncBookResult {
  deleted?: boolean;
}

export const useResyncFile = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      fileId,
      payload,
    }: {
      fileId: number;
      payload: ResyncPayload;
    }): Promise<File | ResyncFileResult> => {
      return API.request<File | ResyncFileResult>(
        "POST",
        `/books/files/${fileId}/resync`,
        payload,
      );
    },
    onSuccess: (result, { fileId }) => {
      // If not deleted, invalidate queries to refresh data
      if (!("deleted" in result && result.deleted)) {
        const file = result as File;
        queryClient.invalidateQueries({
          queryKey: [BooksQueryKey.Book, file.book_id],
        });
      }
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};

export const useResyncBook = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      bookId,
      payload,
    }: {
      bookId: number;
      payload: ResyncPayload;
    }): Promise<Book | ResyncBookResult> => {
      return API.request<Book | ResyncBookResult>(
        "POST",
        `/books/${bookId}/resync`,
        payload,
      );
    },
    onSuccess: (result, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [BooksQueryKey.Book, bookId],
      });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
    },
  });
};
```

**Step 2: Export from books.ts**

In `app/hooks/queries/books.ts`, add export:

```typescript
export * from "./resync";
```

Or update the index file if there is one.

**Step 3: Run TypeScript check**

Run: `yarn lint:types`
Expected: No errors

**Step 4: Commit**

```bash
git add app/hooks/queries/resync.ts app/hooks/queries/books.ts
git commit -m "$(cat <<'EOF'
[Resync] Add frontend mutation hooks for resync

useResyncFile and useResyncBook with cache invalidation.
EOF
)"
```

---

## Task 9: Add Resync Confirmation Dialog

**Files:**
- Create: `app/components/library/ResyncConfirmDialog.tsx`

**Step 1: Create the confirmation dialog component**

Create `app/components/library/ResyncConfirmDialog.tsx`:

```typescript
import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ResyncConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: "book" | "file";
  entityName: string;
  onConfirm: () => void;
  isPending: boolean;
}

export function ResyncConfirmDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onConfirm,
  isPending,
}: ResyncConfirmDialogProps) {
  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0" />
            <span>Refresh All Metadata</span>
          </DialogTitle>
          <DialogDescription>
            This will rescan the {entityType} "{entityName}" and overwrite all
            metadata with values from the source file(s). Any manual changes
            you've made will be lost.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm}>
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Refresh
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Run lint**

Run: `yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/library/ResyncConfirmDialog.tsx
git commit -m "$(cat <<'EOF'
[Resync] Add ResyncConfirmDialog component

Shows warning before refresh all metadata operation.
EOF
)"
```

---

## Task 10: Add Context Menu to BookItem

**Files:**
- Modify: `app/components/library/BookItem.tsx`

**Step 1: Add dropdown menu to BookItem**

Update `app/components/library/BookItem.tsx` to add a context menu:

```typescript
import { MoreVertical, RefreshCw } from "lucide-react";
import { uniqBy } from "lodash";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { ResyncConfirmDialog } from "@/components/library/ResyncConfirmDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useResyncBook } from "@/hooks/queries/resync";
import { cn } from "@/libraries/utils";
import {
  AuthorRolePenciller,
  AuthorRoleWriter,
  FileTypeCBZ,
  type Book,
  type File,
} from "@/types";

// ... existing helper functions ...

const BookItem = ({
  book,
  libraryId,
  seriesId,
  coverAspectRatio = "book",
}: BookItemProps) => {
  // ... existing state ...
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);
  const resyncBookMutation = useResyncBook();

  const handleScanMetadata = async () => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { refresh: false },
      });
      toast.success("Metadata scanned");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to scan metadata",
      );
    }
  };

  const handleRefreshMetadata = async () => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { refresh: true },
      });
      toast.success("Metadata refreshed");
      setShowRefreshDialog(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh metadata",
      );
    }
  };

  return (
    <div className="w-32 group/card relative" key={book.id}>
      {/* Context menu button - shows on hover */}
      <div className="absolute top-1 right-1 z-10 opacity-0 group-hover/card:opacity-100 transition-opacity">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              className="h-7 w-7 bg-black/50 hover:bg-black/70"
              size="icon"
              variant="ghost"
            >
              <MoreVertical className="h-4 w-4 text-white" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              disabled={resyncBookMutation.isPending}
              onClick={handleScanMetadata}
            >
              <RefreshCw className="h-4 w-4 mr-2" />
              Scan for new metadata
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Refresh all metadata
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* ... rest of existing component ... */}

      <ResyncConfirmDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRefreshMetadata}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
    </div>
  );
};

export default BookItem;
```

**Step 2: Run lint and type check**

Run: `yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/library/BookItem.tsx
git commit -m "$(cat <<'EOF'
[Resync] Add context menu to BookItem

Adds "Scan for new metadata" and "Refresh all metadata" options.
EOF
)"
```

---

## Task 11: Add Context Menu to FileRow

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add resync options to FileRow**

Update the FileRow component props and implementation:

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
  onScanMetadata: () => void;      // Add
  onRefreshMetadata: () => void;   // Add
  isResyncing: boolean;            // Add
  isSupplement?: boolean;
}

const FileRow = ({
  // ... existing props ...
  onScanMetadata,
  onRefreshMetadata,
  isResyncing,
  isSupplement = false,
}: FileRowProps) => {
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);

  // In the actions section, add a dropdown menu:
  return (
    <div className="py-2 space-y-1">
      {/* ... existing content ... */}

      {/* Stats and actions */}
      <div className="flex items-center gap-3 text-xs text-muted-foreground flex-shrink-0">
        {/* ... existing stats ... */}

        {/* Actions dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              disabled={isResyncing}
              size="sm"
              title="More actions"
              variant="ghost"
            >
              <MoreVertical className="h-3 w-3" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onEdit}>
              <Edit className="h-4 w-4 mr-2" />
              Edit
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={isResyncing}
              onClick={onScanMetadata}
            >
              <RefreshCw className="h-4 w-4 mr-2" />
              Scan for new metadata
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
              <RefreshCw className="h-4 w-4 mr-2" />
              Refresh all metadata
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* ... rest of component ... */}

      <ResyncConfirmDialog
        entityName={file.name || getFilename(file.filepath)}
        entityType="file"
        isPending={isResyncing}
        onConfirm={() => {
          onRefreshMetadata();
          setShowRefreshDialog(false);
        }}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
    </div>
  );
};
```

**Step 2: Update BookDetail to pass resync handlers**

In the BookDetail component, add resync mutation and handlers:

```typescript
const BookDetail = () => {
  // ... existing state ...
  const resyncFileMutation = useResyncFile();
  const [resyncingFileId, setResyncingFileId] = useState<number | null>(null);

  const handleScanFileMetadata = async (fileId: number) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { refresh: false },
      });
      if ("deleted" in result && result.deleted) {
        toast.success("File removed (no longer exists on disk)");
      } else {
        toast.success("Metadata scanned");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to scan metadata",
      );
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleRefreshFileMetadata = async (fileId: number) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { refresh: true },
      });
      if ("deleted" in result && result.deleted) {
        toast.success("File removed (no longer exists on disk)");
      } else {
        toast.success("Metadata refreshed");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh metadata",
      );
    } finally {
      setResyncingFileId(null);
    }
  };

  // ... in the render, pass to FileRow:
  <FileRow
    // ... existing props ...
    isResyncing={resyncingFileId === file.id}
    onRefreshMetadata={() => handleRefreshFileMetadata(file.id)}
    onScanMetadata={() => handleScanFileMetadata(file.id)}
  />
};
```

**Step 3: Run lint and type check**

Run: `yarn lint`
Expected: No errors

**Step 4: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Resync] Add resync options to FileRow

Adds "Scan for new metadata" and "Refresh all metadata" to file actions.
EOF
)"
```

---

## Task 12: Run Full Test Suite and Manual Testing

**Step 1: Run all backend tests**

Run: `make test`
Expected: All tests pass

**Step 2: Run all frontend linting**

Run: `make lint`
Expected: No errors

**Step 3: Manual testing checklist**

Test the following scenarios:
- [ ] Scan a book for new metadata (priority-respecting)
- [ ] Refresh all metadata for a book (bypasses priority)
- [ ] Scan a single file for new metadata
- [ ] Refresh all metadata for a single file
- [ ] Verify that manually-edited fields are preserved on scan
- [ ] Verify that manually-edited fields are overwritten on refresh
- [ ] Test with a file that no longer exists on disk (should delete record)
- [ ] Test with a book that has no files (should delete record)

**Step 4: Final commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Resync] Complete single book/file resync feature

- Backend: POST /books/:id/resync and POST /books/files/:id/resync
- Frontend: Context menus on BookItem and FileRow with confirmation dialogs
- Supports both priority-respecting scan and force refresh modes
EOF
)"
```

---

## Implementation Notes

### Dependencies Between Tasks

- Tasks 1-2 can be done in parallel (both modify scan_helpers.go but different functions)
- Task 3-4 depend on Tasks 1-2
- Tasks 5-7 depend on Tasks 3-4
- Task 8 can be started once the API shape is defined (after Task 5)
- Tasks 9-11 depend on Task 8
- Task 12 depends on all other tasks

### Key Files Reference

| File | Purpose |
|------|---------|
| `pkg/worker/scan_helpers.go` | Priority logic functions |
| `pkg/worker/scan.go` | Main scan job processing |
| `pkg/worker/resync.go` | New single file/book scan functions |
| `pkg/books/handlers.go` | HTTP handlers for books/files |
| `pkg/books/routes.go` | Route registration |
| `app/hooks/queries/resync.ts` | Frontend mutation hooks |
| `app/components/library/BookItem.tsx` | Book card with context menu |
| `app/components/pages/BookDetail.tsx` | Book detail with file list |
| `app/components/library/ResyncConfirmDialog.tsx` | Confirmation dialog |

### Skills to Reference

- `@.claude/skills/backend.md` - Go patterns, Echo handlers, Bun ORM
- `@.claude/skills/frontend.md` - React patterns, Tanstack Query, UI components
