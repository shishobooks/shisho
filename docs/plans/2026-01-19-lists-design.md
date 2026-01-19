# Lists Feature Design

## Overview

Lists are user-created collections of books that span across libraries. They enable users to organize books beyond the library/tag structure—common use cases include "To Be Read" (TBR) lists, favorites, and themed collections.

## Key Requirements

- Lists belong to a user, not a library
- Books from any accessible library can be added to a list
- Lists can be ordered (manual sort) or unordered (standard sort options)
- Lists can be shared with other users at three permission levels
- Books are filtered at query time based on viewer's library access
- Bulk selection supports adding multiple books across gallery pages

## Data Model

### lists

```sql
CREATE TABLE lists (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_ordered BOOLEAN NOT NULL DEFAULT FALSE,
    default_sort TEXT NOT NULL DEFAULT 'added_at_desc'
);

CREATE UNIQUE INDEX ux_lists_user_name ON lists (user_id, name COLLATE NOCASE);
CREATE INDEX ix_lists_user_id ON lists (user_id);
```

### list_books

```sql
CREATE TABLE list_books (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
    book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    added_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    sort_order INTEGER  -- NULL for unordered lists, sequential integers for ordered
);

CREATE UNIQUE INDEX ux_list_books ON list_books (list_id, book_id);
CREATE INDEX ix_list_books_book_id ON list_books (book_id);
CREATE INDEX ix_list_books_list_sort ON list_books (list_id, sort_order);
```

### list_shares

```sql
CREATE TABLE list_shares (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
    user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
    permission TEXT NOT NULL CHECK (permission IN ('viewer', 'editor', 'manager')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    shared_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX ux_list_shares ON list_shares (list_id, user_id);
CREATE INDEX ix_list_shares_user_id ON list_shares (user_id);
```

## Security Model

### Library Access Filtering

Books in lists are filtered at query time based on the requesting user's library access. The list data remains intact—if a user loses library access, those books become invisible but aren't deleted. If access is restored, the books reappear.

```go
func GetListBooks(listID int, user *models.User) ([]Book, error) {
    query := db.NewSelect().
        Model(&books).
        Join("JOIN list_books ON list_books.book_id = books.id").
        Where("list_books.list_id = ?", listID)

    // Filter to only books from libraries user can access
    if libraryIDs := user.GetAccessibleLibraryIDs(); libraryIDs != nil {
        query.Where("books.library_id IN (?)", bun.In(libraryIDs))
    }

    return books, query.Scan(ctx)
}
```

### Permission Levels

| Permission | View List | Add/Remove Books | Edit Metadata | Share | Delete |
|------------|-----------|------------------|---------------|-------|--------|
| Viewer     | ✓         |                  |               |       |        |
| Editor     | ✓         | ✓                |               |       |        |
| Manager    | ✓         | ✓                | ✓             | ✓     |        |
| Owner      | ✓         | ✓                | ✓             | ✓     | ✓      |

### Shared List Book Visibility

When user A shares a list with user B, user B only sees books from libraries they have access to. The book count shown reflects visible books, not total books.

## API Endpoints

### List CRUD

- `GET /lists` - List all lists (owned + shared with user)
- `POST /lists` - Create a new list
- `GET /lists/:id` - Get list details
- `PATCH /lists/:id` - Update list metadata (name, description, is_ordered, default_sort)
- `DELETE /lists/:id` - Delete list (owner only)

### List Books

- `GET /lists/:id/books` - Get books in list (filtered by user's library access, respects sort)
- `POST /lists/:id/books` - Add books to list (accepts array of book IDs, order preserved)
- `DELETE /lists/:id/books` - Remove books from list (accepts array of book IDs)
- `PATCH /lists/:id/books/reorder` - Reorder books (for ordered lists)

### Sharing

- `GET /lists/:id/shares` - Get current shares
- `POST /lists/:id/shares` - Add a share (user_id, permission)
- `PATCH /lists/:id/shares/:shareId` - Update permission level
- `DELETE /lists/:id/shares/:shareId` - Remove a share
- `GET /lists/:id/shares/check?user_id=X` - Check book visibility for a user (for warning UI)

### Book's List Membership

- `GET /books/:id/lists` - Get lists this book belongs to (for "Add to List" UI)
- `POST /books/:id/lists` - Bulk update which lists a book belongs to

### Templates

- `GET /lists/templates` - Get available templates with defaults
- `POST /lists/templates/:name` - Create list from template

## Frontend Components

### Navigation

A top-level "Lists" item in the sidebar:

```
Libraries
  ├── Library A
  ├── Library B
Lists
  ├── TBR (12)
  ├── Favorites (8)
  ├── + Create List
```

### First-Time Experience

When a user with no lists visits `/lists`:

- Empty state message explaining lists
- Template buttons: "Create TBR List" (ordered), "Create Favorites List" (unordered)
- "Create Custom List" button

### Create List Dialog

- Name (required)
- Description (optional)
- Ordered toggle with helper text
- Create button

### List Detail Page

**Route:** `/lists/:id`

**Header:**
- List name
- Description (if present)
- Metadata: "12 books · Created Jan 15, 2026 · Updated 2 days ago"
- For shared lists: "Shared by [Owner Name]" badge
- Action buttons based on permission: Edit, Share, Delete, "Enter Selection Mode"

**Books Gallery:**

For ordered lists:
- Books display in sort_order sequence
- Drag handles for reordering
- "Move to position" in context menu for cross-page moves
- New books added to end of list
- Bulk adds preserve selection order

For unordered lists:
- Sort dropdown (Title, Author, Series, Date Added)
- Default from list's `default_sort` field
- "Save as default" option for owner/manager

**Attribution:**
- Shared lists show "Added by [Name]" under each book
- Non-shared lists hide attribution to reduce noise

### Add to List (Single Book)

**Quick Popover:**
- Shows lists where user can add (owner/editor/manager)
- Checkmark for lists book is already in
- Click to toggle membership (immediate save)
- "Manage Lists..." link for full modal
- "Create New List" link

**Full Modal:**
- All lists with checkboxes
- Search/filter for many lists
- Viewer-only lists shown but disabled with tooltip: "You need editor permission to add books to this list"
- "Create New List" button
- "Done" button

**Feedback:**
- Toast on add: "Added to TBR"
- Toast on remove: "Removed from Favorites"

### Share Dialog

**Content:**
- Current shares list (user, permission, "Shared by [Name]", remove button)
- Add person section with user search
- Permission dropdown: Viewer / Editor / Manager
- Share button

**Smart Warnings:**
- All books accessible: No warning
- Some books accessible: Yellow warning with count
- No books accessible: Red warning

### Bulk Selection System

**Entering Selection Mode:**
- "Select" button in gallery toolbar
- Toggles to "Cancel" when active
- Checkboxes appear on book cards

**Selection Behavior:**
- Click card/checkbox to toggle
- Shift+click for range selection
- Selection stored as ordered array (preserves selection order)
- Persists across pagination within same gallery
- Clears when leaving gallery section

**Floating Bottom Toolbar:**

```
┌─────────────────────────────────────────────────────────┐
│  ☑ 12 selected (across 3 pages)    [Add to List ▼]  [✕] │
└─────────────────────────────────────────────────────────┘
```

**Bulk Selection Context:**

```typescript
interface BulkSelectionContext {
  selectedBookIds: number[];  // Array preserves selection order
  isSelectionMode: boolean;
  enterSelectionMode: () => void;
  exitSelectionMode: () => void;
  toggleBook: (bookId: number) => void;
  selectRange: (fromId: number, toId: number, pageBookIds: number[]) => void;
  clearSelection: () => void;
}
```

### Extensibility for Future Bulk Actions

```typescript
interface BulkAction {
  id: string;
  label: string;
  icon: ReactNode;
  isEnabled: (selectedIds: number[]) => boolean;
  execute: (selectedIds: number[]) => Promise<boolean>;
}
```

New actions implement this interface and register with the toolbar.

## Templates

| Template | Name | Description | Ordered | Default Sort |
|----------|------|-------------|---------|--------------|
| TBR | "To Be Read" | "Books I want to read next" | Yes | (manual) |
| Favorites | "Favorites" | "My favorite books" | No | added_at_desc |

Clicking a template button creates the list immediately and navigates to it.

## Ordered List Behavior

- New single book: Appended with `sort_order = MAX(sort_order) + 1`
- Bulk add: Books appended in selection order
- Reordering: Drag and drop for local changes, "Move to position" for cross-page jumps
- Switching from ordered to unordered: Warning that custom order will be lost
