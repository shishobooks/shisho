# Unified Scan Refactor Implementation Plan

**Goal:** Unify batch library scans and single book/file resyncs into a single `Scan()` function, eliminating ~400 lines of duplicate code and fixing the "delete if missing" bug.

**Design Spec:** `docs/plans/2026-01-18-unified-scan-refactor-design.md`

**Architecture:** Create a single entry point `Scan(ctx, ScanOptions)` that routes to internal handlers based on which entry point is set (FilePath, FileID, or BookID). Extract the metadata update logic from the existing 1800-line `scanFile` function into a reusable `scanFileCore` function. The existing `resync.go` handlers become thin wrappers calling the unified `Scan()`.

**Tech Stack:** Go, SQLite (Bun ORM), existing test infrastructure

**Execution Mode:** oneshot

**Out of Scope:** `ForceRefresh` support on batch scan jobs (noted as "future" in design spec)

---

## Task 1: Create ScanOptions and ScanResult Types

Define the new public types that will serve as the unified interface for all scan operations.

- [ ] Task 1.1 Create new file `pkg/worker/scan_unified.go`
  - This file will contain the new unified scan types and entry point
  - Keep separate from `scan.go` initially to avoid merge conflicts during development

- [ ] Task 1.2 Define `ScanOptions` struct
  - Entry points (mutually exclusive): `FilePath string`, `FileID int`, `BookID int`
  - Context: `LibraryID int` (required for FilePath mode)
  - Behavior: `ForceRefresh bool`
  - Logging: `JobLog *joblogs.JobLogger` (optional, for batch scan context)
  - Add godoc explaining the mutually exclusive entry points

- [ ] Task 1.3 Define `ScanResult` struct
  - Single file results: `File *models.File`, `Book *models.Book`, `FileCreated bool`, `FileDeleted bool`, `BookDeleted bool`
  - Book scan results: `Files []*ScanResult`
  - Add godoc explaining when each field is populated

- [ ] Task 1.4 Run `make check` to verify types compile

- [ ] Task 1.5 Commit
  - Message: "[Resync] Add ScanOptions and ScanResult types"

---

## Task 2: Implement Scan() Router Function

Create the main entry point that validates options and routes to appropriate internal handlers.

- [ ] Task 2.1 Write test for `Scan()` entry point validation in `pkg/worker/scan_unified_test.go`
  - Test `TestScan_ZeroEntryPoints`: call `Scan(ctx, ScanOptions{})`, assert error message contains "exactly one of FilePath, FileID, or BookID must be set"
  - Test `TestScan_MultipleEntryPoints`: call `Scan(ctx, ScanOptions{FileID: 1, BookID: 2})`, assert same error message
  - Test `TestScan_SingleEntryPoint_FileID`: call `Scan(ctx, ScanOptions{FileID: 1})`, assert no validation error (may return other errors, but not validation error)

- [ ] Task 2.2 Run tests to verify they fail
  - Expected: "Scan not found" or similar

- [ ] Task 2.3 Implement `Scan()` function in `pkg/worker/scan_unified.go`
  - Validate exactly one of FilePath, FileID, BookID is set
  - Return `errors.New("exactly one of FilePath, FileID, or BookID must be set")` on validation failure
  - Route to `scanBook()`, `scanFileByID()`, or `scanFileByPath()` based on which is set
  - Initially have the internal functions return `nil, nil` (stubs)

- [ ] Task 2.4 Run tests to verify validation tests pass

- [ ] Task 2.5 Commit
  - Message: "[Resync] Add Scan() router with entry point validation"

---

## Task 3: Implement scanFileByID (Delete-If-Missing Logic)

This is the core handler for single file resyncs that includes the delete-if-missing behavior.

- [ ] Task 3.1 Write tests for `scanFileByID` deletion behavior in `pkg/worker/scan_unified_test.go`
  - Test `TestScanFileByID_MissingFile_DeletesFile`:
    - Setup: Create book with 2 files in DB, delete one physical file from disk
    - Call: `Scan(ctx, ScanOptions{FileID: deletedFileID})`
    - Assert: `result.FileDeleted == true`, `result.BookDeleted == false`
    - Assert: File is gone from DB (RetrieveFile returns NotFound)
  - Test `TestScanFileByID_MissingFile_LastFile_DeletesBook`:
    - Setup: Create book with 1 file in DB, delete the physical file from disk
    - Call: `Scan(ctx, ScanOptions{FileID: deletedFileID})`
    - Assert: `result.FileDeleted == true`, `result.BookDeleted == true`
    - Assert: Book is gone from DB (RetrieveBook returns NotFound)
  - Test `TestScanFileByID_NotFound`:
    - Call: `Scan(ctx, ScanOptions{FileID: 99999})`
    - Assert: Error contains "not found" or is errcodes.NotFound

- [ ] Task 3.2 Write tests for `scanFileByID` error handling
  - Test `TestScanFileByID_UnreadableFile`:
    - Setup: Create file with `chmod 000` on disk
    - Call: `Scan(ctx, ScanOptions{FileID: fileID})`
    - Assert: Error returned (file exists but can't be read)
    - Cleanup: `chmod 644` to restore permissions
  - Test `TestScanFileByID_CorruptFile`:
    - Setup: Create file with invalid content (not valid EPUB/CBZ/M4B)
    - Call: `Scan(ctx, ScanOptions{FileID: fileID})`
    - Assert: Error returned containing parse failure details

- [ ] Task 3.3 Run tests to verify they fail

- [ ] Task 3.4 Implement `scanFileByID` function
  - Fetch file from DB with `w.bookService.RetrieveFileWithRelations(ctx, opts.FileID)` (defined in `pkg/books/service.go:389`)
  - Return wrapped error if file not found
  - Check if file exists on disk with `os.Stat(file.Filepath)`
  - If `os.IsNotExist(err)`:
    - Get parent book: `w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})`
    - Determine if this is the last file: `bookDeleted := len(book.Files) == 1`
    - Delete file record: `w.bookService.DeleteFile(ctx, file.ID)`
    - If last file: `w.searchService.DeleteFromBookIndex(ctx, book.ID)` then `w.bookService.DeleteBook(ctx, book.ID)`
    - Return `&ScanResult{FileDeleted: true, BookDeleted: bookDeleted}, nil`
  - If file exists but not `os.IsNotExist`:
    - Return wrapped permission/stat error
  - If file exists:
    - Parse metadata with `parseFileMetadata(file.Filepath, file.FileType)` (defined in `pkg/worker/resync.go:100`)
    - If parse error, return wrapped error with details
    - Stub out the metadata update for now (will be extracted in Task 4)
    - Return file and book in result

- [ ] Task 3.5 Run tests to verify they pass

- [ ] Task 3.6 Commit
  - Message: "[Resync] Implement scanFileByID with delete-if-missing logic"

---

## Task 4: Extract scanFileCore for Book Scalar Updates

Extract the book scalar field update logic from `scanFile` into a helper. This is the first part of the metadata update extraction.

- [ ] Task 4.1 Create `scanFileCore` function signature in `pkg/worker/scan_unified.go`
  ```go
  func (w *Worker) scanFileCore(
      ctx context.Context,
      file *models.File,
      book *models.Book,
      metadata *mediafile.ParsedMetadata,
      forceRefresh bool,
      jobLog *joblogs.JobLogger,
  ) (*ScanResult, error)
  ```

- [ ] Task 4.2 Write tests for book scalar field updates in `pkg/worker/scan_unified_test.go`
  - Test `TestScanFileCore_BookTitle_HigherPriority`:
    - Setup: Book with `Title="Old"`, `TitleSource="filepath"`. Metadata with `Title="New"`, `DataSource="epub_metadata"`
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: `book.Title == "New"`, `book.TitleSource == "epub_metadata"`
  - Test `TestScanFileCore_BookTitle_LowerPriority_Skipped`:
    - Setup: Book with `Title="Manual"`, `TitleSource="manual"`. Metadata with `Title="New"`, `DataSource="epub_metadata"`
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: `book.Title == "Manual"` (unchanged)
  - Test `TestScanFileCore_BookTitle_ForceRefresh`:
    - Setup: Book with `Title="Manual"`, `TitleSource="manual"`. Metadata with `Title="New"`, `DataSource="epub_metadata"`
    - Call: `scanFileCore(ctx, file, book, metadata, true, nil)` (forceRefresh=true)
    - Assert: `book.Title == "New"` (updated despite lower priority)

- [ ] Task 4.3 Run tests to verify they fail

- [ ] Task 4.4 Implement book scalar updates in scanFileCore
  - Reference existing logic in `pkg/worker/resync.go:140-202` (updateFileAndBookMetadata)
  - Update Title using `shouldUpdateScalar()` from `pkg/worker/scan_helpers.go:8`
  - Update SortTitle (regenerate using `sortname.ForTitle()`)
  - Update Subtitle, Description (handle nil pointer cases)
  - Call `w.bookService.UpdateBook()` with column list

- [ ] Task 4.5 Run tests to verify they pass

- [ ] Task 4.6 Commit
  - Message: "[Resync] Add scanFileCore with book scalar updates"

---

## Task 5: Add Book Relationship Updates to scanFileCore

Extend scanFileCore to handle author, series, genre, and tag updates.

- [ ] Task 5.1 Write tests for book relationship updates
  - Test `TestScanFileCore_Authors_HigherPriority`:
    - Setup: Book with 1 author "Old Author", source="filepath". Metadata with authors ["New Author"], source="epub_metadata"
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: Book now has 1 author "New Author"
  - Test `TestScanFileCore_Series_HigherPriority`:
    - Setup: Book with series "Old Series", source="filepath". Metadata with series "New Series", number=2.0
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: Book now in "New Series" at position 2.0

- [ ] Task 5.2 Run tests to verify they fail

- [ ] Task 5.3 Implement book relationship updates in scanFileCore
  - Reference existing logic in `pkg/worker/resync.go:203-252` (author updates)
  - Use `shouldUpdateRelationship()` from `pkg/worker/scan_helpers.go:39`
  - Authors: Delete existing via `w.bookService.DeleteAuthors()`, create new via `w.personService.FindOrCreatePerson()` + `w.bookService.CreateAuthor()`
  - Series: Delete existing via `w.bookService.DeleteBookSeries()`, create new via `w.seriesService.FindOrCreateSeries()` + `w.bookService.CreateBookSeries()`
  - Genres and Tags follow same pattern

- [ ] Task 5.4 Run tests to verify they pass

- [ ] Task 5.5 Commit
  - Message: "[Resync] Add book relationship updates to scanFileCore"

---

## Task 6: Add File Updates and Sidecars to scanFileCore

Complete scanFileCore with file-level updates, sidecar writes, and search index updates.

- [ ] Task 6.1 Write tests for file updates
  - Test `TestScanFileCore_Narrators_HigherPriority`:
    - Setup: File with narrator "Old", source="filepath". Metadata with narrators ["New"], source="m4b_metadata"
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: File now has narrator "New"
  - Test `TestScanFileCore_FileName_CBZ`:
    - Setup: CBZ file with nil Name. Metadata with Title="Comic Title", Series="Series", SeriesNumber=1
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: `file.Name == "Series v1"` (uses generateCBZFileName logic)

- [ ] Task 6.2 Write tests for sidecar and search index
  - Test `TestScanFileCore_WritesSidecars`:
    - Setup: Book and file in temp directory
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: `<bookpath>/<filename>.metadata.json` exists
  - Test `TestScanFileCore_UpdatesSearchIndex`:
    - Setup: Mock search service, book and file
    - Call: `scanFileCore(ctx, file, book, metadata, false, nil)`
    - Assert: `searchService.IndexBook()` was called

- [ ] Task 6.3 Run tests to verify they fail

- [ ] Task 6.4 Implement file updates in scanFileCore
  - Reference existing logic in `pkg/worker/resync.go:254-327` (file name, narrators)
  - Update file Name for CBZ files using `generateCBZFileName()` from `pkg/worker/scan.go:48`
  - Update URL field
  - Update narrators: Delete existing via `w.bookService.DeleteNarratorsForFile()`, create new
  - Update identifiers: Delete existing via `w.bookService.DeleteFileIdentifiers()`, create new

- [ ] Task 6.5 Implement sidecar writes and search index
  - Reference existing logic in `pkg/worker/resync.go:330-348`
  - Reload book with full relations before writing sidecar
  - Call `sidecar.WriteBookSidecarFromModel()` and `sidecar.WriteFileSidecarFromModel()`
  - Call `w.searchService.IndexBook()` to update search index

- [ ] Task 6.6 Return `&ScanResult{File: file, Book: book, FileCreated: false}`

- [ ] Task 6.7 Run tests to verify they pass

- [ ] Task 6.8 Commit
  - Message: "[Resync] Complete scanFileCore with file updates, sidecars, and search"

---

## Task 7: Integrate scanFileCore into scanFileByID

Wire up the extracted scanFileCore function into the scanFileByID handler.

- [ ] Task 7.1 Update scanFileByID to call scanFileCore
  - After parsing metadata successfully, get parent book: `w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &file.BookID})`
  - Call `w.scanFileCore(ctx, file, book, metadata, opts.ForceRefresh, opts.JobLog)`
  - Return the result from scanFileCore

- [ ] Task 7.2 Write integration test for full scanFileByID flow
  - Test `TestScanFileByID_Integration_UpdatesMetadata`:
    - Setup: Create EPUB file with title "File Title" on disk, create book in DB with title "DB Title"
    - Call: `Scan(ctx, ScanOptions{FileID: fileID, ForceRefresh: true})`
    - Assert: `result.File.Book.Title == "File Title"`
    - Assert: DB now has updated title

- [ ] Task 7.3 Run tests to verify they pass

- [ ] Task 7.4 Commit
  - Message: "[Resync] Wire scanFileCore into scanFileByID"

---

## Task 8: Implement scanBook

Implement the book-level scan that loops through all files in a book.

- [ ] Task 8.1 Write tests for scanBook behavior in `pkg/worker/scan_unified_test.go`
  - Test `TestScanBook_NoFiles_DeletesBook`:
    - Setup: Create book with no files in DB
    - Call: `Scan(ctx, ScanOptions{BookID: bookID})`
    - Assert: `result.BookDeleted == true`
    - Assert: Book gone from DB
  - Test `TestScanBook_NotFound`:
    - Call: `Scan(ctx, ScanOptions{BookID: 99999})`
    - Assert: Error contains "not found"
  - Test `TestScanBook_MultipleFiles`:
    - Setup: Create book with 2 files
    - Call: `Scan(ctx, ScanOptions{BookID: bookID})`
    - Assert: `len(result.Files) == 2`
    - Assert: `result.Book` is populated with updated book
  - Test `TestScanBook_FileError_ContinuesWithOthers`:
    - Setup: Create book with 2 files, make one file unreadable
    - Call: `Scan(ctx, ScanOptions{BookID: bookID})`
    - Assert: Function completes (doesn't error out)
    - Assert: `len(result.Files) >= 1` (at least the readable file processed)

- [ ] Task 8.2 Run tests to verify they fail

- [ ] Task 8.3 Implement scanBook function
  - Fetch book with files: `w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &opts.BookID})`
  - Return wrapped error if not found
  - If `len(book.Files) == 0`:
    - Delete from search index: `w.searchService.DeleteFromBookIndex(ctx, book.ID)`
    - Delete book: `w.bookService.DeleteBook(ctx, book.ID)`
    - Return `&ScanResult{BookDeleted: true}, nil`
  - Initialize `fileResults := make([]*ScanResult, 0, len(book.Files))`
  - Loop through files:
    - Call `fileResult, err := w.scanFileByID(ctx, ScanOptions{FileID: file.ID, ForceRefresh: opts.ForceRefresh, JobLog: opts.JobLog})`
    - If err, log warning and continue (don't fail entire book scan)
    - If `fileResult.BookDeleted`, return immediately (book was deleted)
    - Append to `fileResults`
  - Reload book: `w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{ID: &opts.BookID})`
  - Return `&ScanResult{Book: book, Files: fileResults}`

- [ ] Task 8.4 Run tests to verify they pass

- [ ] Task 8.5 Commit
  - Message: "[Resync] Implement scanBook for book-level resyncs"

---

## Task 9: Implement scanFileByPath

Implement the path-based scan used by batch library scans.

- [ ] Task 9.1 Write tests for scanFileByPath behavior
  - Test `TestScanFileByPath_FileNotOnDisk`:
    - Call: `Scan(ctx, ScanOptions{FilePath: "/nonexistent/file.epub", LibraryID: 1})`
    - Assert: `result == nil`, `err == nil` (skip silently)
  - Test `TestScanFileByPath_MissingLibraryID`:
    - Call: `Scan(ctx, ScanOptions{FilePath: "/some/file.epub"})`
    - Assert: Error contains "LibraryID required"
  - Test `TestScanFileByPath_ExistingFile_DelegatesToScanFileByID`:
    - Setup: Create file in DB at path "/tmp/test.epub", file exists on disk
    - Call: `Scan(ctx, ScanOptions{FilePath: "/tmp/test.epub", LibraryID: 1})`
    - Assert: Returns same result as if called with FileID
  - Test `TestScanFileByPath_NewFile`:
    - Setup: Create valid EPUB at temp path, no DB record
    - Call: `Scan(ctx, ScanOptions{FilePath: tempPath, LibraryID: libID})`
    - Assert: `result.FileCreated == true`
    - Assert: File now exists in DB

- [ ] Task 9.2 Run tests to verify they fail

- [ ] Task 9.3 Implement scanFileByPath function
  - Validate LibraryID: `if opts.LibraryID == 0 { return nil, errors.New("LibraryID required for FilePath mode") }`
  - Check if file exists in DB: `w.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{Filepath: &opts.FilePath, LibraryID: &opts.LibraryID})`
  - If exists and not NotFound error, delegate: `return w.scanFileByID(ctx, ScanOptions{FileID: existingFile.ID, ForceRefresh: opts.ForceRefresh, JobLog: opts.JobLog})`
  - Check if file exists on disk: `_, err := os.Stat(opts.FilePath)`
  - If `os.IsNotExist(err)`, return `nil, nil` (skip silently)
  - If other stat error, return wrapped error
  - Parse metadata: `metadata, err := parseFileMetadata(opts.FilePath, fileType)` where fileType is derived from extension
  - Stub file/book creation for now: `return nil, errors.New("file creation not yet implemented")`

- [ ] Task 9.4 Run tests to verify passing tests pass, creation test still fails (expected)

- [ ] Task 9.5 Commit
  - Message: "[Resync] Implement scanFileByPath stub"

---

## Task 10: Wire scanFileByPath to File Creation Logic

Connect the new scanFileByPath to the existing file/book creation code.

- [ ] Task 10.1 Identify file/book creation logic in scanFile
  - Book path determination: `pkg/worker/scan.go:437-466`
  - Book creation: `pkg/worker/scan.go:1027-1048`
  - File creation: `pkg/worker/scan.go:1213-1244`
  - Author creation: `pkg/worker/scan.go:1050-1072`
  - Series creation: `pkg/worker/scan.go:1074-1095`
  - Cover extraction: `pkg/worker/scan.go:1156-1211`

- [ ] Task 10.2 Create `scanFileCreateNew` helper function
  - Signature: `func (w *Worker) scanFileCreateNew(ctx context.Context, opts ScanOptions, metadata *mediafile.ParsedMetadata) (*ScanResult, error)`
  - Move/copy creation logic from scanFile
  - This is a large extraction (~600 lines) but keeps creation logic isolated

- [ ] Task 10.3 Wire scanFileByPath to use scanFileCreateNew
  - Replace stub with: `return w.scanFileCreateNew(ctx, opts, metadata)`

- [ ] Task 10.4 Write integration test for new file creation
  - Test `TestScanFileByPath_CreatesBookAndFile`:
    - Setup: Create valid EPUB with title "Test Book", author "Test Author" in temp directory
    - Call: `Scan(ctx, ScanOptions{FilePath: tempPath, LibraryID: libID})`
    - Assert: `result.FileCreated == true`
    - Assert: `result.Book.Title == "Test Book"`
    - Assert: `result.Book.Authors[0].Person.Name == "Test Author"`
    - Assert: Cover file exists on disk

- [ ] Task 10.5 Run tests to verify they pass

- [ ] Task 10.6 Commit
  - Message: "[Resync] Wire scanFileByPath to file creation logic"

---

## Task 11: Migrate Handlers to Use Unified Scan

Update the API handlers to use the new unified Scan() function.

- [ ] Task 11.1 Update `pkg/books/handlers.go` with new Scanner interface
  - Remove `FileRescanner` interface (lines 33-56)
  - Remove `FileRescannerOptions` struct (line 33-36)
  - Remove `FileRescannerResult` struct (line 38-43)
  - Remove `BookRescannerResult` struct (line 45-49)
  - Add new interface:
    ```go
    type Scanner interface {
        Scan(ctx context.Context, opts worker.ScanOptions) (*worker.ScanResult, error)
    }
    ```
  - Replace `fileRescanner FileRescanner` with `scanner Scanner` in handler struct (line 69)

- [ ] Task 11.2 Update resyncFile handler (line 1576-1624)
  - Change call from `h.fileRescanner.ScanSingleFile(ctx, id, FileRescannerOptions{ForceRefresh: params.Refresh})`
  - To: `h.scanner.Scan(ctx, worker.ScanOptions{FileID: id, ForceRefresh: params.Refresh})`
  - Keep existing response handling (FileDeleted/BookDeleted map)

- [ ] Task 11.3 Update resyncBook handler (line 1626-1673)
  - Change call from `h.fileRescanner.ScanSingleBook(ctx, id, FileRescannerOptions{ForceRefresh: params.Refresh})`
  - To: `h.scanner.Scan(ctx, worker.ScanOptions{BookID: id, ForceRefresh: params.Refresh})`
  - Keep existing response handling

- [ ] Task 11.4 Update handler initialization in `pkg/books/routes.go`
  - Find where handler is created
  - Change `fileRescanner: worker` to `scanner: worker`

- [ ] Task 11.5 Run `make check` to verify compilation and tests pass

- [ ] Task 11.6 Commit
  - Message: "[Resync] Migrate handlers to unified Scan()"

---

## Task 12: Migrate ProcessScanJob to Use Unified Scan

Update the batch scan job to use the new unified Scan() function.

- [ ] Task 12.1 Update ProcessScanJob to call Scan() for each file
  - In `pkg/worker/scan.go:326-331`, replace:
    ```go
    err := w.scanFile(ctx, path, library.ID, booksToOrganize, jobLog)
    ```
  - With:
    ```go
    result, err := w.Scan(ctx, ScanOptions{
        FilePath:     path,
        LibraryID:    library.ID,
        ForceRefresh: false,
        JobLog:       jobLog,
    })
    if result != nil && result.File != nil {
        seenFileIDs[result.File.ID] = struct{}{}
    }
    ```
  - Add `seenFileIDs := make(map[int]struct{})` before the file loop

- [ ] Task 12.2 Add orphan cleanup after filesystem walk
  - After the `for _, path := range filesToScan` loop and before "Organize files after scan"
  - Add:
    ```go
    // Cleanup orphaned files (in DB but not on disk)
    existingFiles, err := w.bookService.ListFilesForLibrary(ctx, library.ID)
    if err != nil {
        jobLog.Warn("failed to list files for orphan cleanup", logger.Data{"error": err.Error()})
    } else {
        for _, file := range existingFiles {
            if _, seen := seenFileIDs[file.ID]; !seen {
                _, err := w.Scan(ctx, ScanOptions{FileID: file.ID})
                if err != nil {
                    jobLog.Warn("failed to cleanup orphaned file", logger.Data{"file_id": file.ID, "error": err.Error()})
                }
            }
        }
    }
    ```

- [ ] Task 12.3 Add `ListFilesForLibrary` method to book service
  - In `pkg/books/service.go`, add:
    ```go
    func (s *Service) ListFilesForLibrary(ctx context.Context, libraryID int) ([]*models.File, error) {
        var files []*models.File
        err := s.db.NewSelect().
            Model(&files).
            Where("library_id = ?", libraryID).
            Where("file_role = ?", models.FileRoleMain).
            Scan(ctx)
        return files, err
    }
    ```
  - Only fetch main files (supplements don't need orphan cleanup)

- [ ] Task 12.4 Write test for orphan cleanup
  - Test `TestProcessScanJob_CleansUpOrphanedFiles`:
    - Setup: Create library, create book+file in DB, delete physical file
    - Call: `ProcessScanJob(ctx, job, jobLog)`
    - Assert: File is gone from DB
    - Assert: Book is gone from DB (was last file)

- [ ] Task 12.5 Run `make check` to verify all tests pass

- [ ] Task 12.6 Commit
  - Message: "[Resync] Migrate ProcessScanJob to unified Scan() with orphan cleanup"

---

## Task 13: Delete Legacy Code

Remove the old resync.go and related types now that everything uses unified Scan().

- [ ] Task 13.1 Delete `pkg/worker/resync.go`
  - Verify all its functions are now replaced by scan_unified.go
  - `ScanSingleFile` -> `scanFileByID`
  - `ScanSingleBook` -> `scanBook`
  - `parseFileMetadata` -> moved to scan_unified.go
  - `updateFileAndBookMetadata` -> `scanFileCore`

- [ ] Task 13.2 Move `parseFileMetadata` if not already moved
  - If still in resync.go, move to scan_unified.go before deletion
  - This function is used by multiple paths and should be preserved

- [ ] Task 13.3 Search for remaining references
  - `grep -r "ScanSingleFile\|ScanSingleBook\|FileRescanner\|FileRescannerOptions\|FileRescannerResult\|BookRescannerResult" pkg/`
  - Update or remove any references found

- [ ] Task 13.4 Run `make check` to verify nothing is broken

- [ ] Task 13.5 Commit
  - Message: "[Resync] Delete legacy resync.go"

---

## Task 14: Final Cleanup

Consolidate and clean up the implementation.

- [ ] Task 14.1 Evaluate file organization
  - If `scan.go` is now significantly smaller after extraction, consider:
    - Keep `scan_unified.go` as is (clear separation of unified API)
    - Or merge into `scan.go` if it makes sense
  - Decision criteria: Is the split intuitive for future maintainers?

- [ ] Task 14.2 Remove unused code from scan.go
  - The old `scanFile` function may have dead code paths if scanFileCreateNew replaced it
  - Identify and remove any unused functions

- [ ] Task 14.3 Add godoc comments to public API
  - `Scan()`: Document as main entry point, explain options
  - `ScanOptions`: Document each field and mutual exclusivity
  - `ScanResult`: Document each field and when it's populated

- [ ] Task 14.4 Run full test suite and linting
  - `make check`
  - Fix any warnings or issues

- [ ] Task 14.5 Commit
  - Message: "[Resync] Final cleanup for unified scan refactor"

---

## Summary of Changes

### Files Created
- `pkg/worker/scan_unified.go` - Unified `Scan()` function, `ScanOptions`, `ScanResult`, and internal handlers
- `pkg/worker/scan_unified_test.go` - Tests for unified scan functionality

### Files Modified
- `pkg/worker/scan.go` - `ProcessScanJob` uses unified Scan(), orphan cleanup added
- `pkg/books/handlers.go` - Uses `Scanner` interface instead of `FileRescanner`
- `pkg/books/routes.go` - Handler initialization updated
- `pkg/books/service.go` - Added `ListFilesForLibrary()` method

### Files Deleted
- `pkg/worker/resync.go` - All functionality moved to unified scan

### Types Removed
- `FileRescannerOptions` - Replaced by `ScanOptions`
- `FileRescannerResult` - Replaced by `ScanResult`
- `BookRescannerResult` - Replaced by `ScanResult`
- `FileRescanner` interface - Replaced by `Scanner` interface

### Bug Fixes
- **Delete if missing now works for single file/book resyncs**: `scanFileByID` properly detects missing files and deletes DB records
- **Batch scans clean up orphaned files**: `ProcessScanJob` now tracks seen files and deletes orphans

### Out of Scope
- `ForceRefresh` on batch scan jobs (noted as "future" in design spec)
