# Primary File Design

## Overview

Introduce the concept of a "primary file" for each book. When a book has multiple files (e.g., EPUB + PDF, or multiple EPUB editions), the primary file is the one that gets synced to devices via Kobo sync. This prevents duplicate books appearing on e-readers.

## Data Model

### Books Table

Add a non-nullable foreign key to the `books` table:

```sql
ALTER TABLE books ADD COLUMN primary_file_id INTEGER NOT NULL REFERENCES files(id);
```

### Constraints

- `primary_file_id` must reference a file belonging to the same book
- Non-nullable: every book must have exactly one primary file
- When the last file is deleted, the book is deleted (existing behavior)

## Selection Logic

### Default Selection (New Files)

When a new file is scanned and becomes the first file for a book:
- That file automatically becomes the primary

When additional files are added to an existing book:
- The existing primary remains unchanged

### Auto-Promotion on Deletion

When the primary file is deleted:
1. Find the oldest remaining file where `file_role = 'main'`
2. If no main files exist, fall back to the oldest file (any role)
3. Set that file as the new primary

### Manual Selection

Users can manually change the primary file via the UI.

## Migration

Set `primary_file_id` for all existing books, preferring main files:

```sql
UPDATE books SET primary_file_id = (
  SELECT id FROM files
  WHERE files.book_id = books.id
  ORDER BY
    CASE WHEN file_role = 'main' THEN 0 ELSE 1 END,
    created_at ASC
  LIMIT 1
);
```

This handles edge cases where a book might only have supplement files.

## Kobo Sync Changes

### Current Behavior

Syncs all files where `file_role = 'main'` for books in the sync scope.

### New Behavior

Sync only the primary file for each book in the sync scope.

Update `pkg/kobo/service.go` query to join on `books.primary_file_id`:

```go
q = q.Where("f.id = b.primary_file_id")
```

## Frontend Changes

### File List Display

Only show primary file indicator when the book has multiple files:
- Display a badge or icon next to the primary file
- Hide for books with a single file

### Changing Primary

- Add "Set as primary" action in the `...` overflow menu for each file
- Only visible when the book has multiple files
- Action calls `PUT /api/books/:bookId/primary-file` with `{ fileId: number }`

## API Changes

### New Endpoint

```
PUT /api/books/:id/primary-file
Body: { "file_id": number }
```

### Handler Logic

1. Validate book exists and user has access
2. Validate file exists and belongs to the book
3. Update `book.primary_file_id`
4. Return updated book

## Backend Changes

### File Deletion

Update `DeleteFile` to handle primary file deletion:

```go
func (svc *Service) DeleteFile(ctx context.Context, fileID int) error {
    // ... existing deletion logic ...

    // If this was the primary file, promote another
    if book.PrimaryFileID == fileID {
        newPrimary := selectNewPrimaryFile(book.Files, fileID)
        if newPrimary != nil {
            book.PrimaryFileID = newPrimary.ID
            // update book
        }
        // If no files remain, book will be deleted by existing logic
    }
}
```

### File Creation

Update file creation to set primary if this is the first file:

```go
func (svc *Service) CreateFile(ctx context.Context, file *models.File) error {
    // ... existing creation logic ...

    // If book has no primary file, set this as primary
    if book.PrimaryFileID == 0 {
        book.PrimaryFileID = file.ID
        // update book
    }
}
```

## Implementation Tasks

1. **Migration** - Add `primary_file_id` column and populate for existing books
2. **Backend: Model updates** - Add `PrimaryFileID` to Book model
3. **Backend: CreateFile** - Set primary for first file
4. **Backend: DeleteFile** - Auto-promote on primary deletion
5. **Backend: New endpoint** - `PUT /books/:id/primary-file`
6. **Backend: Kobo sync** - Update query to use `primary_file_id`
7. **Frontend: File list** - Show primary indicator for multi-file books
8. **Frontend: Set primary action** - Add overflow menu item

## Future Enhancements

- Library-level format priority (e.g., prefer EPUB over PDF) for auto-selection
- Per-list file selection (different primary per list) - if needed
