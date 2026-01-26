# Book Merge and Split Design

## Overview

Add the ability to move files between books, enabling two key workflows:
1. **Merge books** - combine files from multiple books into one (for content that was scanned/grouped separately)
2. **Split files to new book** - move files out of a book into a newly created book (for incorrectly grouped content)

Both operations are variations of a single core action: moving files from one book to another.

## Core Operation

The atomic operation is `MoveFilesToBook`:

```
MoveFilesToBook(fileIDs []string, targetBookID *string, libraryID string)
```

- If `targetBookID` is provided: move files to that existing book
- If `targetBookID` is nil: create a new book from the first file's metadata, then move all files to it

### Behavior

1. Validate all files and target book are in the same library
2. If creating new book: create book record from first file's metadata
3. For each file:
   - Update `file.book_id` to target book
   - If `OrganizeFileStructure` is enabled, move the physical file to the target book's directory
   - Use existing `uniqueFilename` helper for filename conflicts
4. Check each source book - if it now has zero files, delete it
5. Update search indexes:
   - Call `IndexBook()` on the target book
   - Call `IndexBook()` on any source books that still have files
   - Call `DeleteFromBookIndex()` for any deleted source books

### Atomicity and Rollback

The operation should be as atomic as possible:

1. Track which files have been physically moved (`{fileID, originalPath, newPath}`)
2. If an error occurs mid-operation:
   - Rollback the database transaction (automatic)
   - Attempt to move files back to their original locations
   - If any file restore fails, log the details clearly for manual intervention
3. Only commit the DB transaction after all file moves succeed

## API Endpoints

### POST `/api/books/:id/move-files`

Move selected files from this book to another book or a new book.

**Request:**
```json
{
  "file_ids": ["file-uuid-1", "file-uuid-2"],
  "target_book_id": "existing-book-uuid"  // null = create new book
}
```

**Response:**
```json
{
  "target_book": { /* full book object */ },
  "files_moved": 2,
  "source_book_deleted": true
}
```

### POST `/api/books/merge`

Merge multiple books into one (bulk action from library view).

**Request:**
```json
{
  "source_book_ids": ["book-uuid-1", "book-uuid-2"],
  "target_book_id": "book-uuid-3"
}
```

**Response:**
```json
{
  "target_book": { /* full book object */ },
  "files_moved": 5,
  "books_deleted": 2
}
```

## Frontend: Book Detail Page

On the book detail page's file list, add file selection and a move action.

### UI Changes

1. **Selection mode**: Add checkboxes next to each file (or a "Select" toggle that enables multi-select)
2. **Action bar**: When files are selected, show a floating action bar with "Move to..." button
3. **Move dialog**: Clicking "Move to..." opens a dialog with:
   - "Create new book" option at the top
   - Search/filter for existing books in the same library
   - Warning banner (see below)
4. **Confirmation**: After selecting target, show summary and confirm button

### Warning Messages

**When OrganizeFileStructure enabled:**
> "The selected files will be moved to the target book's folder. Metadata from the source book will not be transferred."

**When OrganizeFileStructure disabled:**
> "The selected files will be reassigned to the target book. Metadata from the source book will not be transferred."

### Mobile Considerations

- Touch-friendly checkboxes with adequate tap targets
- Bottom sheet dialog instead of centered modal on small screens
- Full-width action bar

## Frontend: Library Bulk Merge

On the library view, add bulk merge capability.

### UI Changes

1. **Selection mode**: Add checkboxes on book cards when in "select mode"
2. **Bulk action bar**: When 2+ books are selected, show action bar with "Merge" button
3. **Target selection dialog**:
   - Shows the selected books with key info (title, file count, has cover indicator)
   - Radio buttons to pick which book becomes the target
   - Warning banner (see below)
4. **Confirmation**: Summary of what will happen, confirm button

### Warning Messages

**When OrganizeFileStructure enabled:**
> "Other books will be deleted. Their files will move to the selected target's folder. Metadata from deleted books will not be transferred."

**When OrganizeFileStructure disabled:**
> "Other books will be deleted. Their files will be reassigned to the selected target. Metadata from deleted books will not be transferred."

### Mobile Considerations

- Bottom sheet dialog instead of centered modal
- Large touch targets for radio buttons
- Scrollable book list if many selected

## Backend Implementation

### New Files

**`pkg/books/merge.go`**
- `MoveFilesToBook(ctx, fileIDs, targetBookID, libraryID)` - the atomic operation
- `CreateBookFromFile(ctx, file)` - creates new book using file's metadata
- `rollbackFileMoves(movedFiles)` - restore files on error
- Helper types for tracking file moves

### Modified Files

**`pkg/books/handlers.go`**
- `HandleMoveFiles` - handles `POST /api/books/:id/move-files`
- `HandleMergeBooks` - handles `POST /api/books/merge`

**`pkg/books/routes.go`**
- Register the new routes

## Error Handling

### Validation Errors (fail fast, no partial changes)

| Condition | Response |
|-----------|----------|
| Files not found | 404 |
| Target book not found | 404 |
| Files/books in different libraries | 400 "All items must be in the same library" |
| Moving files to their current book | 400 "Files already belong to this book" |
| No files selected | 400 "No files selected" |

### Runtime Errors (attempt rollback)

- **File move fails** (permissions, disk full): rollback moved files, return 500 with details
- **DB error**: transaction auto-rollback, attempt file rollback

### Edge Cases

| Case | Handling |
|------|----------|
| Target book has 0 files before merge | Valid - will have files after |
| Source book becomes empty | Auto-delete the source book |
| Filename conflict | Use `uniqueFilename` to rename |
| All files moved to new book | Source book deleted, new book created |

## Database Changes

None required. The existing schema supports this feature - we're only updating `file.book_id` values.

## Implementation Tasks

1. **Backend: Core operation** - `MoveFilesToBook` in `pkg/books/merge.go`
2. **Backend: Handlers** - `HandleMoveFiles` and `HandleMergeBooks`
3. **Backend: Routes** - Register new endpoints
4. **Frontend: File selection** - Add selection mode to book detail file list
5. **Frontend: Move dialog** - Book search/select dialog with warning
6. **Frontend: Library selection** - Add selection mode to library view
7. **Frontend: Merge dialog** - Target picker dialog with warning
8. **Frontend: Mobile** - Ensure all new UI works on mobile devices
