# Delete Books and Files Feature

## Overview

Add the ability to delete books and files from Shisho, including permanent deletion of files from disk.

## API Endpoints

### DELETE /books/:id

Delete a single book and all its files from disk.

**Response:**
```json
{
  "files_deleted": 3
}
```

### DELETE /books/files/:id

Delete a single file and its associated files (cover, sidecar) from disk.

**Response:**
```json
{
  "book_deleted": false
}
```

The `book_deleted` field is `true` when this was the last file in the book and the book was also deleted.

### POST /books/delete

Bulk delete multiple books.

**Request:**
```json
{
  "book_ids": [1, 2, 3]
}
```

**Response:**
```json
{
  "books_deleted": 3,
  "files_deleted": 7
}
```

## Backend Service Logic

### DeleteBook

1. Begin transaction
2. Load book with files and library
3. If `OrganizeFileStructure` is true: delete the entire book directory
4. If false: delete each file individually with its cover and sidecar
5. Delete book from database (cascades to files, chapters, narrators, identifiers)
6. Commit transaction

### DeleteFile

1. Begin transaction
2. Load file with book and library
3. Delete file from disk with associated cover and sidecar
4. Delete file from database (cascades to chapters, narrators, identifiers)
5. Check if book has any remaining files
6. If no files remain: delete book from database and clean up empty directory
7. Commit transaction

### Bulk Delete

1. Begin transaction
2. Load all books with files
3. Delete each book using DeleteBook logic
4. If any deletion fails, entire transaction rolls back
5. Commit transaction

### Error Handling

- Bulk operations are atomic: all succeed or all fail
- File system operations happen before DB commit
- If DB commit fails after files deleted, log error (files are gone but this is acceptable)

## Frontend UI

### DeleteConfirmationDialog Component

Shared dialog used by all delete actions.

**Props:**
- `variant`: "book" | "books" | "file"
- `title`: Book/file title (for single delete)
- `files`: File list for expandable details (for book delete)
- `books`: Book list for expandable details (for bulk delete)
- `onConfirm`: Callback when user confirms
- `isPending`: Loading state

**Layout:**
- Red warning banner: "This action cannot be undone. Files will be permanently deleted from disk."
- Summary: title and file count (single) or "X books (Y files total)" (bulk)
- Collapsible "Show details" section listing affected files/books
- Cancel button (outline) + Delete button (destructive red)

**Mobile-friendly:**
- `max-h-[90vh] overflow-y-auto` on dialog content
- ScrollArea for expandable lists
- Responsive button layout via DialogFooter

### Entry Points

#### 1. Book Detail Page

- Add "Delete book" to the MoreVertical menu
- Red text, separated by divider, at bottom of menu
- On success: navigate to home page

#### 2. Home Page - Book Card Actions

- Add "Delete" to book card action popover
- Red text at bottom of menu
- On success: stay on home page, invalidate queries

#### 3. Home Page - Selection Mode

- Add "Delete" button to SelectionToolbar (destructive style)
- Opens dialog with variant="books"
- On success: exit selection mode, invalidate queries

#### 4. File Detail Page

- Add "Delete file" to page header actions
- On success: navigate to book detail if book exists, otherwise home

#### 5. Book Detail - File Row

- Add "Delete" to file action menu
- Red text at bottom of menu
- On success: stay on page if book exists, navigate home if book deleted

### Navigation After Deletion

- Use `book_deleted` response field to determine navigation
- If book still exists: stay on current page, invalidate queries
- If book deleted: navigate to home page

### Query Invalidation

After any delete operation:
```typescript
queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
queryClient.invalidateQueries({ queryKey: [QueryKey.GlobalSearch] });
queryClient.invalidateQueries({ queryKey: [QueryKey.SearchBooks] });
```

## Testing

### Backend Tests

1. **TestDeleteBook**
   - Book and files removed from DB
   - Files deleted from disk (main file, cover, sidecar)
   - Book directory deleted when OrganizeFileStructure=true

2. **TestDeleteFile**
   - File removed from DB
   - File deleted from disk with associated files
   - Book remains when other files exist
   - Book deleted when last file removed

3. **TestDeleteBooks**
   - All books and files removed
   - Atomic rollback on failure

4. **Handler tests** for each endpoint

### E2E Tests

1. Delete book from book detail page
2. Delete file from file detail page
3. Bulk delete from home page selection mode
4. Verify navigation after deletion
