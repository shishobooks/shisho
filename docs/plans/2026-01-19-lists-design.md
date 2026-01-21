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

---

# Lists Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a cross-library book lists feature with sharing capabilities and bulk selection.

**Architecture:** Backend uses Go/Echo/Bun following existing service/handler patterns. Frontend uses React/TypeScript with Tanstack Query. Lists are user-scoped (not library-scoped) with permission-based sharing.

**Tech Stack:** Go, Echo, Bun ORM, SQLite, React 19, TypeScript, Tanstack Query, TailwindCSS

---

## Phase 1: Database & Models

### Task 1.1: Create Migration File

**Files:**
- Create: `pkg/migrations/20260119000000_create_lists_tables.go`

**Step 1: Create the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		// Create lists table
		_, err := db.Exec(`
			CREATE TABLE lists (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				name TEXT NOT NULL,
				description TEXT,
				is_ordered BOOLEAN NOT NULL DEFAULT FALSE,
				default_sort TEXT NOT NULL DEFAULT 'added_at_desc'
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_lists_user_name ON lists (user_id, name COLLATE NOCASE)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_lists_user_id ON lists (user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create list_books table
		_, err = db.Exec(`
			CREATE TABLE list_books (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
				book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
				added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				added_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
				sort_order INTEGER
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_books ON list_books (list_id, book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_books_book_id ON list_books (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_books_list_sort ON list_books (list_id, sort_order)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Create list_shares table
		_, err = db.Exec(`
			CREATE TABLE list_shares (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				permission TEXT NOT NULL CHECK (permission IN ('viewer', 'editor', 'manager')),
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				shared_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_shares ON list_shares (list_id, user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		_, err = db.Exec(`CREATE INDEX ix_list_shares_user_id ON list_shares (user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`DROP TABLE IF EXISTS list_shares`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS list_books`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`DROP TABLE IF EXISTS lists`)
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run migration to verify**

Run: `make db:migrate`
Expected: Migration runs successfully

**Step 3: Test rollback**

Run: `make db:rollback && make db:migrate`
Expected: Rollback and re-migrate both succeed

**Step 4: Commit**

```bash
git add pkg/migrations/20260119000000_create_lists_tables.go
git commit -m "$(cat <<'EOF'
[Feature] Add lists database schema

Create tables for lists, list_books, and list_shares with appropriate
indexes for user lookups, book membership, and sharing.
EOF
)"
```

---

### Task 1.2: Create List Model

**Files:**
- Create: `pkg/models/list.go`

**Step 1: Create the model file**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

// List permission levels.
const (
	ListPermissionViewer  = "viewer"
	ListPermissionEditor  = "editor"
	ListPermissionManager = "manager"
)

// List default sort options.
const (
	ListSortAddedAtDesc  = "added_at_desc"
	ListSortAddedAtAsc   = "added_at_asc"
	ListSortTitleAsc     = "title_asc"
	ListSortTitleDesc    = "title_desc"
	ListSortAuthorAsc    = "author_asc"
	ListSortAuthorDesc   = "author_desc"
	ListSortManual       = "manual"
)

type List struct {
	bun.BaseModel `bun:"table:lists,alias:l" tstype:"-"`

	ID          int       `bun:",pk,nullzero" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UserID      int       `bun:",nullzero" json:"user_id"`
	User        *User     `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty" tstype:"User"`
	Name        string    `bun:",nullzero" json:"name"`
	Description *string   `json:"description"`
	IsOrdered   bool      `json:"is_ordered"`
	DefaultSort string    `bun:",nullzero" json:"default_sort"`

	// Relations
	ListBooks  []*ListBook  `bun:"rel:has-many,join:id=list_id" json:"list_books,omitempty" tstype:"ListBook[]"`
	ListShares []*ListShare `bun:"rel:has-many,join:id=list_id" json:"list_shares,omitempty" tstype:"ListShare[]"`
}

type ListBook struct {
	bun.BaseModel `bun:"table:list_books,alias:lb" tstype:"-"`

	ID            int       `bun:",pk,nullzero" json:"id"`
	ListID        int       `bun:",nullzero" json:"list_id"`
	List          *List     `bun:"rel:belongs-to,join:list_id=id" json:"list,omitempty" tstype:"List"`
	BookID        int       `bun:",nullzero" json:"book_id"`
	Book          *Book     `bun:"rel:belongs-to,join:book_id=id" json:"book,omitempty" tstype:"Book"`
	AddedAt       time.Time `json:"added_at"`
	AddedByUserID *int      `json:"added_by_user_id"`
	AddedByUser   *User     `bun:"rel:belongs-to,join:added_by_user_id=id" json:"added_by_user,omitempty" tstype:"User"`
	SortOrder     *int      `json:"sort_order"`
}

type ListShare struct {
	bun.BaseModel `bun:"table:list_shares,alias:ls" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	ListID         int       `bun:",nullzero" json:"list_id"`
	List           *List     `bun:"rel:belongs-to,join:list_id=id" json:"list,omitempty" tstype:"List"`
	UserID         int       `bun:",nullzero" json:"user_id"`
	User           *User     `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty" tstype:"User"`
	Permission     string    `bun:",nullzero" json:"permission"`
	CreatedAt      time.Time `json:"created_at"`
	SharedByUserID *int      `json:"shared_by_user_id"`
	SharedByUser   *User     `bun:"rel:belongs-to,join:shared_by_user_id=id" json:"shared_by_user,omitempty" tstype:"User"`
}
```

**Step 2: Run tygo to generate TypeScript types**

Run: `make tygo`
Expected: Types generated (or "Nothing to be done" if already up-to-date)

**Step 3: Commit**

```bash
git add pkg/models/list.go
git commit -m "$(cat <<'EOF'
[Feature] Add List, ListBook, and ListShare models

Define models for lists feature with permission constants and sort options.
EOF
)"
```

---

## Phase 2: Backend Service Layer

### Task 2.1: Create Lists Service - Core CRUD

**Files:**
- Create: `pkg/lists/service.go`

**Step 1: Create the service file with core CRUD operations**

```go
package lists

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db}
}

type CreateListOptions struct {
	UserID      int
	Name        string
	Description *string
	IsOrdered   bool
	DefaultSort string
}

func (svc *Service) CreateList(ctx context.Context, opts CreateListOptions) (*models.List, error) {
	now := time.Now()

	defaultSort := opts.DefaultSort
	if defaultSort == "" {
		if opts.IsOrdered {
			defaultSort = models.ListSortManual
		} else {
			defaultSort = models.ListSortAddedAtDesc
		}
	}

	list := &models.List{
		CreatedAt:   now,
		UpdatedAt:   now,
		UserID:      opts.UserID,
		Name:        opts.Name,
		Description: opts.Description,
		IsOrdered:   opts.IsOrdered,
		DefaultSort: defaultSort,
	}

	_, err := svc.db.
		NewInsert().
		Model(list).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return list, nil
}

type RetrieveListOptions struct {
	ID     *int
	UserID *int // For ownership check
}

func (svc *Service) RetrieveList(ctx context.Context, opts RetrieveListOptions) (*models.List, error) {
	list := &models.List{}

	q := svc.db.
		NewSelect().
		Model(list).
		Relation("User")

	if opts.ID != nil {
		q = q.Where("l.id = ?", *opts.ID)
	}

	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcodes.NotFound("List")
		}
		return nil, errors.WithStack(err)
	}

	return list, nil
}

type ListListsOptions struct {
	UserID     int // Required - shows owned + shared lists
	Limit      *int
	Offset     *int
	includeTotal bool
}

func (svc *Service) ListLists(ctx context.Context, opts ListListsOptions) ([]*models.List, error) {
	lists, _, err := svc.listListsWithTotal(ctx, opts)
	return lists, err
}

func (svc *Service) ListListsWithTotal(ctx context.Context, opts ListListsOptions) ([]*models.List, int, error) {
	opts.includeTotal = true
	return svc.listListsWithTotal(ctx, opts)
}

func (svc *Service) listListsWithTotal(ctx context.Context, opts ListListsOptions) ([]*models.List, int, error) {
	var lists []*models.List
	var total int
	var err error

	// Get lists owned by user OR shared with user
	q := svc.db.
		NewSelect().
		Model(&lists).
		Relation("User").
		Where("l.user_id = ? OR l.id IN (SELECT list_id FROM list_shares WHERE user_id = ?)", opts.UserID, opts.UserID).
		Order("l.name ASC")

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return lists, total, nil
}

type UpdateListOptions struct {
	Columns []string
}

func (svc *Service) UpdateList(ctx context.Context, list *models.List, opts UpdateListOptions) error {
	if len(opts.Columns) == 0 {
		return nil
	}

	list.UpdatedAt = time.Now()
	columns := append(opts.Columns, "updated_at")

	_, err := svc.db.
		NewUpdate().
		Model(list).
		Column(columns...).
		WherePK().
		Exec(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errcodes.NotFound("List")
		}
		return errors.WithStack(err)
	}
	return nil
}

func (svc *Service) DeleteList(ctx context.Context, listID int) error {
	_, err := svc.db.
		NewDelete().
		Model((*models.List)(nil)).
		Where("id = ?", listID).
		Exec(ctx)
	return errors.WithStack(err)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/service.go
git commit -m "$(cat <<'EOF'
[Feature] Add lists service with core CRUD operations

Create, retrieve, list, update, and delete operations for lists.
EOF
)"
```

---

### Task 2.2: Add List Books Operations to Service

**Files:**
- Modify: `pkg/lists/service.go`

**Step 1: Add book operations to the service**

Append to `pkg/lists/service.go`:

```go
// GetListBookCount returns the number of books in a list visible to the user.
func (svc *Service) GetListBookCount(ctx context.Context, listID int, libraryIDs []int) (int, error) {
	q := svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Join("JOIN books b ON b.id = lb.book_id").
		Where("lb.list_id = ?", listID)

	if libraryIDs != nil {
		q = q.Where("b.library_id IN (?)", bun.In(libraryIDs))
	}

	count, err := q.Count(ctx)
	return count, errors.WithStack(err)
}

type ListBooksOptions struct {
	ListID     int
	LibraryIDs []int // Filter by user's accessible libraries (nil = all)
	Sort       string
	Limit      *int
	Offset     *int
	includeTotal bool
}

func (svc *Service) ListBooks(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, error) {
	books, _, err := svc.listBooksWithTotal(ctx, opts)
	return books, err
}

func (svc *Service) ListBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, int, error) {
	opts.includeTotal = true
	return svc.listBooksWithTotal(ctx, opts)
}

func (svc *Service) listBooksWithTotal(ctx context.Context, opts ListBooksOptions) ([]*models.ListBook, int, error) {
	var listBooks []*models.ListBook
	var total int
	var err error

	q := svc.db.
		NewSelect().
		Model(&listBooks).
		Relation("Book").
		Relation("Book.Authors").
		Relation("Book.Authors.Person").
		Relation("Book.BookSeries").
		Relation("Book.BookSeries.Series").
		Relation("Book.Files").
		Relation("AddedByUser").
		Where("lb.list_id = ?", opts.ListID)

	// Filter by library access
	if opts.LibraryIDs != nil {
		q = q.Join("JOIN books b ON b.id = lb.book_id").
			Where("b.library_id IN (?)", bun.In(opts.LibraryIDs))
	}

	// Apply sort
	switch opts.Sort {
	case models.ListSortManual:
		q = q.Order("lb.sort_order ASC NULLS LAST", "lb.added_at DESC")
	case models.ListSortAddedAtAsc:
		q = q.Order("lb.added_at ASC")
	case models.ListSortTitleAsc:
		q = q.OrderExpr("(SELECT sort_title FROM books WHERE id = lb.book_id) ASC")
	case models.ListSortTitleDesc:
		q = q.OrderExpr("(SELECT sort_title FROM books WHERE id = lb.book_id) DESC")
	case models.ListSortAuthorAsc:
		q = q.OrderExpr("(SELECT p.sort_name FROM authors a JOIN people p ON p.id = a.person_id WHERE a.book_id = lb.book_id LIMIT 1) ASC NULLS LAST")
	case models.ListSortAuthorDesc:
		q = q.OrderExpr("(SELECT p.sort_name FROM authors a JOIN people p ON p.id = a.person_id WHERE a.book_id = lb.book_id LIMIT 1) DESC NULLS LAST")
	default: // added_at_desc
		q = q.Order("lb.added_at DESC")
	}

	if opts.Limit != nil {
		q = q.Limit(*opts.Limit)
	}
	if opts.Offset != nil {
		q = q.Offset(*opts.Offset)
	}

	if opts.includeTotal {
		total, err = q.ScanAndCount(ctx)
	} else {
		err = q.Scan(ctx)
	}
	if err != nil {
		return nil, 0, errors.WithStack(err)
	}

	return listBooks, total, nil
}

type AddBooksOptions struct {
	ListID      int
	BookIDs     []int
	AddedByUserID int
}

func (svc *Service) AddBooks(ctx context.Context, opts AddBooksOptions) error {
	if len(opts.BookIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Get the list to check if ordered
		list := &models.List{}
		err := tx.NewSelect().Model(list).Where("id = ?", opts.ListID).Scan(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Get max sort_order if ordered list
		var maxSortOrder int
		if list.IsOrdered {
			err = tx.NewSelect().
				Model((*models.ListBook)(nil)).
				ColumnExpr("COALESCE(MAX(sort_order), 0)").
				Where("list_id = ?", opts.ListID).
				Scan(ctx, &maxSortOrder)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		now := time.Now()
		listBooks := make([]*models.ListBook, 0, len(opts.BookIDs))

		for i, bookID := range opts.BookIDs {
			lb := &models.ListBook{
				ListID:        opts.ListID,
				BookID:        bookID,
				AddedAt:       now,
				AddedByUserID: &opts.AddedByUserID,
			}
			if list.IsOrdered {
				sortOrder := maxSortOrder + i + 1
				lb.SortOrder = &sortOrder
			}
			listBooks = append(listBooks, lb)
		}

		_, err = tx.NewInsert().
			Model(&listBooks).
			On("CONFLICT (list_id, book_id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Update list's updated_at
		_, err = tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", now).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

type RemoveBooksOptions struct {
	ListID  int
	BookIDs []int
}

func (svc *Service) RemoveBooks(ctx context.Context, opts RemoveBooksOptions) error {
	if len(opts.BookIDs) == 0 {
		return nil
	}

	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().
			Model((*models.ListBook)(nil)).
			Where("list_id = ?", opts.ListID).
			Where("book_id IN (?)", bun.In(opts.BookIDs)).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Update list's updated_at
		_, err = tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

type ReorderBooksOptions struct {
	ListID   int
	BookIDs  []int // New order - book IDs in desired sequence
}

func (svc *Service) ReorderBooks(ctx context.Context, opts ReorderBooksOptions) error {
	return svc.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		for i, bookID := range opts.BookIDs {
			sortOrder := i + 1
			_, err := tx.NewUpdate().
				Model((*models.ListBook)(nil)).
				Set("sort_order = ?", sortOrder).
				Where("list_id = ? AND book_id = ?", opts.ListID, bookID).
				Exec(ctx)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		// Update list's updated_at
		_, err := tx.NewUpdate().
			Model((*models.List)(nil)).
			Set("updated_at = ?", time.Now()).
			Where("id = ?", opts.ListID).
			Exec(ctx)
		return errors.WithStack(err)
	})
}

// GetBookLists returns lists that contain a specific book (for the user).
func (svc *Service) GetBookLists(ctx context.Context, bookID, userID int) ([]*models.List, error) {
	var lists []*models.List

	err := svc.db.NewSelect().
		Model(&lists).
		Where("l.id IN (SELECT list_id FROM list_books WHERE book_id = ?)", bookID).
		Where("l.user_id = ? OR l.id IN (SELECT list_id FROM list_shares WHERE user_id = ?)", userID, userID).
		Order("l.name ASC").
		Scan(ctx)

	return lists, errors.WithStack(err)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/service.go
git commit -m "$(cat <<'EOF'
[Feature] Add list books operations to service

Add/remove books, reorder, get book count, and list books with sorting.
EOF
)"
```

---

### Task 2.3: Add Sharing Operations to Service

**Files:**
- Modify: `pkg/lists/service.go`

**Step 1: Add sharing operations**

Append to `pkg/lists/service.go`:

```go
// Permission check helpers

// CanView returns true if the user can view the list.
func (svc *Service) CanView(ctx context.Context, listID, userID int) (bool, error) {
	// Owner can always view
	var ownerID int
	err := svc.db.NewSelect().
		Model((*models.List)(nil)).
		Column("user_id").
		Where("id = ?", listID).
		Scan(ctx, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ?", listID, userID).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// CanEdit returns true if the user can add/remove books.
func (svc *Service) CanEdit(ctx context.Context, listID, userID int) (bool, error) {
	// Owner can always edit
	var ownerID int
	err := svc.db.NewSelect().
		Model((*models.List)(nil)).
		Column("user_id").
		Where("id = ?", listID).
		Scan(ctx, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares for editor or manager permission
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ?", listID, userID).
		Where("permission IN (?)", bun.In([]string{models.ListPermissionEditor, models.ListPermissionManager})).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// CanManage returns true if the user can edit metadata and share.
func (svc *Service) CanManage(ctx context.Context, listID, userID int) (bool, error) {
	// Owner can always manage
	var ownerID int
	err := svc.db.NewSelect().
		Model((*models.List)(nil)).
		Column("user_id").
		Where("id = ?", listID).
		Scan(ctx, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	if ownerID == userID {
		return true, nil
	}

	// Check shares for manager permission
	count, err := svc.db.NewSelect().
		Model((*models.ListShare)(nil)).
		Where("list_id = ? AND user_id = ? AND permission = ?", listID, userID, models.ListPermissionManager).
		Count(ctx)
	return count > 0, errors.WithStack(err)
}

// IsOwner returns true if the user owns the list.
func (svc *Service) IsOwner(ctx context.Context, listID, userID int) (bool, error) {
	var ownerID int
	err := svc.db.NewSelect().
		Model((*models.List)(nil)).
		Column("user_id").
		Where("id = ?", listID).
		Scan(ctx, &ownerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, errors.WithStack(err)
	}
	return ownerID == userID, nil
}

// Sharing operations

func (svc *Service) ListShares(ctx context.Context, listID int) ([]*models.ListShare, error) {
	var shares []*models.ListShare

	err := svc.db.NewSelect().
		Model(&shares).
		Relation("User").
		Relation("SharedByUser").
		Where("ls.list_id = ?", listID).
		Order("ls.created_at ASC").
		Scan(ctx)

	return shares, errors.WithStack(err)
}

type CreateShareOptions struct {
	ListID         int
	UserID         int
	Permission     string
	SharedByUserID int
}

func (svc *Service) CreateShare(ctx context.Context, opts CreateShareOptions) (*models.ListShare, error) {
	share := &models.ListShare{
		ListID:         opts.ListID,
		UserID:         opts.UserID,
		Permission:     opts.Permission,
		CreatedAt:      time.Now(),
		SharedByUserID: &opts.SharedByUserID,
	}

	_, err := svc.db.
		NewInsert().
		Model(share).
		Returning("*").
		Exec(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Load relations
	err = svc.db.NewSelect().
		Model(share).
		Relation("User").
		Relation("SharedByUser").
		WherePK().
		Scan(ctx)

	return share, errors.WithStack(err)
}

func (svc *Service) UpdateShare(ctx context.Context, shareID int, permission string) error {
	_, err := svc.db.NewUpdate().
		Model((*models.ListShare)(nil)).
		Set("permission = ?", permission).
		Where("id = ?", shareID).
		Exec(ctx)
	return errors.WithStack(err)
}

func (svc *Service) DeleteShare(ctx context.Context, shareID int) error {
	_, err := svc.db.NewDelete().
		Model((*models.ListShare)(nil)).
		Where("id = ?", shareID).
		Exec(ctx)
	return errors.WithStack(err)
}

// CheckBookVisibility returns counts of visible/total books for a user.
func (svc *Service) CheckBookVisibility(ctx context.Context, listID int, targetUserLibraryIDs []int) (visible, total int, err error) {
	// Total books in list
	total, err = svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Where("list_id = ?", listID).
		Count(ctx)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}

	// Visible books (filtered by target user's library access)
	q := svc.db.NewSelect().
		Model((*models.ListBook)(nil)).
		Join("JOIN books b ON b.id = lb.book_id").
		Where("lb.list_id = ?", listID)

	if targetUserLibraryIDs != nil {
		q = q.Where("b.library_id IN (?)", bun.In(targetUserLibraryIDs))
	}

	visible, err = q.Count(ctx)
	return visible, total, errors.WithStack(err)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/service.go
git commit -m "$(cat <<'EOF'
[Feature] Add sharing operations and permission checks to lists service

Permission helpers for view/edit/manage/owner checks, plus share CRUD.
EOF
)"
```

---

### Task 2.4: Create Lists Validators

**Files:**
- Create: `pkg/lists/validators.go`

**Step 1: Create validators file**

```go
package lists

// Query params for list endpoints
type ListListsQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"50" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type ListBooksQuery struct {
	Limit  int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=100"`
	Offset int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Sort   *string `query:"sort" json:"sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

// Payloads for create/update endpoints
type CreateListPayload struct {
	Name        string  `json:"name" validate:"required,min=1,max=200"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   bool    `json:"is_ordered"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

type UpdateListPayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=200" tstype:"string"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   *bool   `json:"is_ordered,omitempty" tstype:"boolean"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

type AddBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type RemoveBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type ReorderBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type CreateSharePayload struct {
	UserID     int    `json:"user_id" validate:"required,min=1"`
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager"`
}

type UpdateSharePayload struct {
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager"`
}

type UpdateBookListsPayload struct {
	ListIDs []int `json:"list_ids" validate:"dive,min=1"`
}

type CheckVisibilityQuery struct {
	UserID int `query:"user_id" json:"user_id" validate:"required,min=1" tstype:"number"`
}

type CreateFromTemplatePayload struct {
	// No fields needed - template name comes from URL
}
```

**Step 2: Run tygo**

Run: `make tygo`
Expected: Types generated

**Step 3: Commit**

```bash
git add pkg/lists/validators.go
git commit -m "$(cat <<'EOF'
[Feature] Add lists validators for API payloads and queries
EOF
)"
```

---

## Phase 3: Backend Handlers

### Task 3.1: Create Lists Handlers - Core CRUD

**Files:**
- Create: `pkg/lists/handlers.go`

**Step 1: Create handlers file with core CRUD**

```go
package lists

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	listsService *Service
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	params := ListListsQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	opts := ListListsOptions{
		UserID: user.ID,
		Limit:  &params.Limit,
		Offset: &params.Offset,
	}

	lists, total, err := h.listsService.ListListsWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Augment with book counts
	type ListWithCount struct {
		*models.List
		BookCount  int    `json:"book_count"`
		Permission string `json:"permission"` // owner, manager, editor, viewer
	}

	result := make([]ListWithCount, len(lists))
	libraryIDs := user.GetAccessibleLibraryIDs()

	for i, l := range lists {
		count, _ := h.listsService.GetListBookCount(ctx, l.ID, libraryIDs)

		// Determine permission level
		permission := "viewer"
		if l.UserID == user.ID {
			permission = "owner"
		} else if canManage, _ := h.listsService.CanManage(ctx, l.ID, user.ID); canManage {
			permission = "manager"
		} else if canEdit, _ := h.listsService.CanEdit(ctx, l.ID, user.ID); canEdit {
			permission = "editor"
		}

		result[i] = ListWithCount{l, count, permission}
	}

	return c.JSON(http.StatusOK, echo.Map{
		"lists": result,
		"total": total,
	})
}

func (h *handler) retrieve(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check view permission
	canView, err := h.listsService.CanView(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canView {
		return errcodes.NotFound("List")
	}

	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	// Get book count
	libraryIDs := user.GetAccessibleLibraryIDs()
	bookCount, _ := h.listsService.GetListBookCount(ctx, id, libraryIDs)

	// Determine permission
	permission := "viewer"
	if list.UserID == user.ID {
		permission = "owner"
	} else if canManage, _ := h.listsService.CanManage(ctx, id, user.ID); canManage {
		permission = "manager"
	} else if canEdit, _ := h.listsService.CanEdit(ctx, id, user.ID); canEdit {
		permission = "editor"
	}

	return c.JSON(http.StatusOK, echo.Map{
		"list":       list,
		"book_count": bookCount,
		"permission": permission,
	})
}

func (h *handler) create(c echo.Context) error {
	ctx := c.Request().Context()

	params := CreateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	defaultSort := ""
	if params.DefaultSort != nil {
		defaultSort = *params.DefaultSort
	}

	list, err := h.listsService.CreateList(ctx, CreateListOptions{
		UserID:      user.ID,
		Name:        params.Name,
		Description: params.Description,
		IsOrdered:   params.IsOrdered,
		DefaultSort: defaultSort,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, list)
}

func (h *handler) update(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := UpdateListPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to edit this list")
	}

	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	opts := UpdateListOptions{Columns: []string{}}

	if params.Name != nil && *params.Name != list.Name {
		list.Name = *params.Name
		opts.Columns = append(opts.Columns, "name")
	}
	if params.Description != nil {
		list.Description = params.Description
		opts.Columns = append(opts.Columns, "description")
	}
	if params.IsOrdered != nil && *params.IsOrdered != list.IsOrdered {
		list.IsOrdered = *params.IsOrdered
		opts.Columns = append(opts.Columns, "is_ordered")
	}
	if params.DefaultSort != nil && *params.DefaultSort != list.DefaultSort {
		list.DefaultSort = *params.DefaultSort
		opts.Columns = append(opts.Columns, "default_sort")
	}

	err = h.listsService.UpdateList(ctx, list, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	// Reload
	list, err = h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, list)
}

func (h *handler) delete(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Only owner can delete
	isOwner, err := h.listsService.IsOwner(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !isOwner {
		return errcodes.Forbidden("Only the owner can delete this list")
	}

	err = h.listsService.DeleteList(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/handlers.go
git commit -m "$(cat <<'EOF'
[Feature] Add lists handlers for core CRUD operations
EOF
)"
```

---

### Task 3.2: Add Book and Share Handlers

**Files:**
- Modify: `pkg/lists/handlers.go`

**Step 1: Add book and share handlers**

Append to `pkg/lists/handlers.go`:

```go
// Book handlers

func (h *handler) listBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := ListBooksQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check view permission
	canView, err := h.listsService.CanView(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canView {
		return errcodes.NotFound("List")
	}

	// Get list to determine default sort
	list, err := h.listsService.RetrieveList(ctx, RetrieveListOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}

	sort := list.DefaultSort
	if params.Sort != nil {
		sort = *params.Sort
	}

	opts := ListBooksOptions{
		ListID:     id,
		LibraryIDs: user.GetAccessibleLibraryIDs(),
		Sort:       sort,
		Limit:      &params.Limit,
		Offset:     &params.Offset,
	}

	listBooks, total, err := h.listsService.ListBooksWithTotal(ctx, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, echo.Map{
		"books": listBooks,
		"total": total,
	})
}

func (h *handler) addBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := AddBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to add books to this list")
	}

	err = h.listsService.AddBooks(ctx, AddBooksOptions{
		ListID:        id,
		BookIDs:       params.BookIDs,
		AddedByUserID: user.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) removeBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := RemoveBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to remove books from this list")
	}

	err = h.listsService.RemoveBooks(ctx, RemoveBooksOptions{
		ListID:  id,
		BookIDs: params.BookIDs,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) reorderBooks(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := ReorderBooksPayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check edit permission
	canEdit, err := h.listsService.CanEdit(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canEdit {
		return errcodes.Forbidden("You don't have permission to reorder books in this list")
	}

	err = h.listsService.ReorderBooks(ctx, ReorderBooksOptions{
		ListID:  id,
		BookIDs: params.BookIDs,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// Share handlers

func (h *handler) listShares(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission to view shares
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to view shares for this list")
	}

	shares, err := h.listsService.ListShares(ctx, id)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, shares)
}

func (h *handler) createShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := CreateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to share this list")
	}

	// Can't share with yourself
	if params.UserID == user.ID {
		return errcodes.ValidationError("You cannot share a list with yourself")
	}

	share, err := h.listsService.CreateShare(ctx, CreateShareOptions{
		ListID:         id,
		UserID:         params.UserID,
		Permission:     params.Permission,
		SharedByUserID: user.ID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, share)
}

func (h *handler) updateShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	shareID, err := strconv.Atoi(c.Param("shareId"))
	if err != nil {
		return errcodes.NotFound("Share")
	}

	params := UpdateSharePayload{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to update shares for this list")
	}

	err = h.listsService.UpdateShare(ctx, shareID, params.Permission)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) deleteShare(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	shareID, err := strconv.Atoi(c.Param("shareId"))
	if err != nil {
		return errcodes.NotFound("Share")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to remove shares from this list")
	}

	err = h.listsService.DeleteShare(ctx, shareID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *handler) checkVisibility(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("List")
	}

	params := CheckVisibilityQuery{}
	if err := c.Bind(&params); err != nil {
		return errors.WithStack(err)
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Check manage permission
	canManage, err := h.listsService.CanManage(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}
	if !canManage {
		return errcodes.Forbidden("You don't have permission to check visibility for this list")
	}

	// TODO: Get target user's library access
	// For now, return placeholder - need to inject users service
	visible, total, err := h.listsService.CheckBookVisibility(ctx, id, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, echo.Map{
		"visible": visible,
		"total":   total,
	})
}

// Template handler

func (h *handler) createFromTemplate(c echo.Context) error {
	ctx := c.Request().Context()

	templateName := c.Param("name")

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	var name, description string
	var isOrdered bool
	var defaultSort string

	switch templateName {
	case "tbr":
		name = "To Be Read"
		description = "Books I want to read next"
		isOrdered = true
		defaultSort = models.ListSortManual
	case "favorites":
		name = "Favorites"
		description = "My favorite books"
		isOrdered = false
		defaultSort = models.ListSortAddedAtDesc
	default:
		return errcodes.NotFound("Template")
	}

	list, err := h.listsService.CreateList(ctx, CreateListOptions{
		UserID:      user.ID,
		Name:        name,
		Description: &description,
		IsOrdered:   isOrdered,
		DefaultSort: defaultSort,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusCreated, list)
}

// Templates list handler

func (h *handler) templates(c echo.Context) error {
	templates := []map[string]interface{}{
		{
			"name":         "tbr",
			"display_name": "To Be Read",
			"description":  "Books I want to read next",
			"is_ordered":   true,
			"default_sort": models.ListSortManual,
		},
		{
			"name":         "favorites",
			"display_name": "Favorites",
			"description":  "My favorite books",
			"is_ordered":   false,
			"default_sort": models.ListSortAddedAtDesc,
		},
	}

	return c.JSON(http.StatusOK, templates)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/handlers.go
git commit -m "$(cat <<'EOF'
[Feature] Add book and share handlers for lists
EOF
)"
```

---

### Task 3.3: Create Routes File

**Files:**
- Create: `pkg/lists/routes.go`

**Step 1: Create routes file**

```go
package lists

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/uptrace/bun"
)

func RegisterRoutesWithGroup(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	listsService := NewService(db)

	h := &handler{
		listsService: listsService,
	}

	// List CRUD
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.retrieve)
	g.PATCH("/:id", h.update)
	g.DELETE("/:id", h.delete)

	// List books
	g.GET("/:id/books", h.listBooks)
	g.POST("/:id/books", h.addBooks)
	g.DELETE("/:id/books", h.removeBooks)
	g.PATCH("/:id/books/reorder", h.reorderBooks)

	// Sharing
	g.GET("/:id/shares", h.listShares)
	g.POST("/:id/shares", h.createShare)
	g.PATCH("/:id/shares/:shareId", h.updateShare)
	g.DELETE("/:id/shares/:shareId", h.deleteShare)
	g.GET("/:id/shares/check", h.checkVisibility)

	// Templates
	g.GET("/templates", h.templates)
	g.POST("/templates/:name", h.createFromTemplate)
}
```

**Step 2: Commit**

```bash
git add pkg/lists/routes.go
git commit -m "$(cat <<'EOF'
[Feature] Add lists routes registration
EOF
)"
```

---

### Task 3.4: Register Routes in Server

**Files:**
- Modify: `pkg/server/server.go`

**Step 1: Add lists routes registration**

Find the section where other routes are registered (around line 138-142) and add:

```go
// Lists routes
listsGroup := e.Group("/lists")
listsGroup.Use(authMiddleware.Authenticate)
lists.RegisterRoutesWithGroup(listsGroup, db, authMiddleware)
```

Also add the import at the top:

```go
"github.com/shishobooks/shisho/pkg/lists"
```

**Step 2: Run build to verify**

Run: `make build`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add pkg/server/server.go
git commit -m "$(cat <<'EOF'
[Feature] Register lists routes in server
EOF
)"
```

---

### Task 3.5: Add Book Lists Endpoint to Books Package

**Files:**
- Modify: `pkg/books/handlers.go`
- Modify: `pkg/books/routes.go`

**Step 1: Add handler for getting book's lists**

Add to `pkg/books/handlers.go`:

```go
func (h *handler) bookLists(c echo.Context) error {
	ctx := c.Request().Context()

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("Book")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}

	// Verify book exists and user has library access
	book, err := h.bookService.RetrieveBook(ctx, RetrieveBookOptions{ID: &id})
	if err != nil {
		return errors.WithStack(err)
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.NotFound("Book")
	}

	lists, err := h.listsService.GetBookLists(ctx, id, user.ID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, lists)
}
```

**Step 2: Add lists service to handler struct and routes**

Update handler struct in `pkg/books/handlers.go` to include `listsService *lists.Service`

Update `pkg/books/routes.go` to:
- Import lists package
- Create lists service
- Add route: `g.GET("/:id/lists", h.bookLists)`

**Step 3: Run build to verify**

Run: `make build`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/books/handlers.go pkg/books/routes.go
git commit -m "$(cat <<'EOF'
[Feature] Add endpoint to get lists containing a book
EOF
)"
```

---

## Phase 4: Backend Tests

### Task 4.1: Create Lists Service Tests

**Files:**
- Create: `pkg/lists/service_test.go`

**Step 1: Create basic service tests**

```go
package lists

import (
	"context"
	"database/sql"
	"testing"

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

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			username TEXT NOT NULL,
			email TEXT,
			password_hash TEXT NOT NULL,
			role_id INTEGER NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			is_ordered BOOLEAN NOT NULL DEFAULT FALSE,
			default_sort TEXT NOT NULL DEFAULT 'added_at_desc'
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE UNIQUE INDEX ux_lists_user_name ON lists (user_id, name COLLATE NOCASE)`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE list_books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
			book_id INTEGER REFERENCES books (id) ON DELETE CASCADE NOT NULL,
			added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			added_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
			sort_order INTEGER
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_books ON list_books (list_id, book_id)`)
	require.NoError(t, err)

	_, err = db.Exec(`
		CREATE TABLE list_shares (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER REFERENCES lists (id) ON DELETE CASCADE NOT NULL,
			user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
			permission TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			shared_by_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`CREATE UNIQUE INDEX ux_list_shares ON list_shares (list_id, user_id)`)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createTestUser(t *testing.T, db *bun.DB, username string) *models.User {
	t.Helper()
	user := &models.User{
		Username:     username,
		PasswordHash: "test",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)
	return user
}

func TestService_CreateList(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user := createTestUser(t, db, "testuser")

	t.Run("creates unordered list with default sort", func(t *testing.T) {
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:    user.ID,
			Name:      "My List",
			IsOrdered: false,
		})
		require.NoError(t, err)
		assert.Equal(t, "My List", list.Name)
		assert.Equal(t, user.ID, list.UserID)
		assert.False(t, list.IsOrdered)
		assert.Equal(t, models.ListSortAddedAtDesc, list.DefaultSort)
	})

	t.Run("creates ordered list with manual sort", func(t *testing.T) {
		list, err := svc.CreateList(ctx, CreateListOptions{
			UserID:    user.ID,
			Name:      "Ordered List",
			IsOrdered: true,
		})
		require.NoError(t, err)
		assert.True(t, list.IsOrdered)
		assert.Equal(t, models.ListSortManual, list.DefaultSort)
	})
}

func TestService_ListLists(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	user1 := createTestUser(t, db, "user1")
	user2 := createTestUser(t, db, "user2")

	// User1 creates two lists
	_, err := svc.CreateList(ctx, CreateListOptions{UserID: user1.ID, Name: "User1 List1"})
	require.NoError(t, err)
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user1.ID, Name: "User1 List2"})
	require.NoError(t, err)

	// User2 creates one list
	_, err = svc.CreateList(ctx, CreateListOptions{UserID: user2.ID, Name: "User2 List"})
	require.NoError(t, err)

	t.Run("returns only user's owned lists", func(t *testing.T) {
		lists, total, err := svc.ListListsWithTotal(ctx, ListListsOptions{UserID: user1.ID})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, lists, 2)
	})
}

func TestService_Permissions(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	owner := createTestUser(t, db, "owner")
	viewer := createTestUser(t, db, "viewer")
	editor := createTestUser(t, db, "editor")
	manager := createTestUser(t, db, "manager")
	outsider := createTestUser(t, db, "outsider")

	list, err := svc.CreateList(ctx, CreateListOptions{UserID: owner.ID, Name: "Test List"})
	require.NoError(t, err)

	// Create shares
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: viewer.ID, Permission: models.ListPermissionViewer, SharedByUserID: owner.ID})
	require.NoError(t, err)
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: editor.ID, Permission: models.ListPermissionEditor, SharedByUserID: owner.ID})
	require.NoError(t, err)
	_, err = svc.CreateShare(ctx, CreateShareOptions{ListID: list.ID, UserID: manager.ID, Permission: models.ListPermissionManager, SharedByUserID: owner.ID})
	require.NoError(t, err)

	t.Run("owner has all permissions", func(t *testing.T) {
		isOwner, _ := svc.IsOwner(ctx, list.ID, owner.ID)
		canView, _ := svc.CanView(ctx, list.ID, owner.ID)
		canEdit, _ := svc.CanEdit(ctx, list.ID, owner.ID)
		canManage, _ := svc.CanManage(ctx, list.ID, owner.ID)
		assert.True(t, isOwner)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.True(t, canManage)
	})

	t.Run("viewer can only view", func(t *testing.T) {
		canView, _ := svc.CanView(ctx, list.ID, viewer.ID)
		canEdit, _ := svc.CanEdit(ctx, list.ID, viewer.ID)
		canManage, _ := svc.CanManage(ctx, list.ID, viewer.ID)
		assert.True(t, canView)
		assert.False(t, canEdit)
		assert.False(t, canManage)
	})

	t.Run("editor can view and edit", func(t *testing.T) {
		canView, _ := svc.CanView(ctx, list.ID, editor.ID)
		canEdit, _ := svc.CanEdit(ctx, list.ID, editor.ID)
		canManage, _ := svc.CanManage(ctx, list.ID, editor.ID)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.False(t, canManage)
	})

	t.Run("manager can view, edit, and manage", func(t *testing.T) {
		canView, _ := svc.CanView(ctx, list.ID, manager.ID)
		canEdit, _ := svc.CanEdit(ctx, list.ID, manager.ID)
		canManage, _ := svc.CanManage(ctx, list.ID, manager.ID)
		assert.True(t, canView)
		assert.True(t, canEdit)
		assert.True(t, canManage)
	})

	t.Run("outsider has no permissions", func(t *testing.T) {
		canView, _ := svc.CanView(ctx, list.ID, outsider.ID)
		canEdit, _ := svc.CanEdit(ctx, list.ID, outsider.ID)
		canManage, _ := svc.CanManage(ctx, list.ID, outsider.ID)
		assert.False(t, canView)
		assert.False(t, canEdit)
		assert.False(t, canManage)
	})
}
```

**Step 2: Run tests**

Run: `TZ=America/Chicago CI=true go test ./pkg/lists/... -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add pkg/lists/service_test.go
git commit -m "$(cat <<'EOF'
[Test] Add lists service tests for CRUD and permissions
EOF
)"
```

---

## Phase 5: Frontend - API Hooks

### Task 5.1: Create Lists Query Hooks

**Files:**
- Create: `app/hooks/queries/lists.ts`

**Step 1: Create the hooks file**

```typescript
import { useMutation, useQuery, useQueryClient, UseQueryOptions } from "@tanstack/react-query";

import API, { ShishoAPIError } from "@/libraries/api";
import type {
  AddBooksPayload,
  CreateListPayload,
  CreateSharePayload,
  List,
  ListBook,
  ListShare,
  RemoveBooksPayload,
  ReorderBooksPayload,
  UpdateListPayload,
  UpdateSharePayload,
} from "@/types";

export enum QueryKey {
  ListLists = "ListLists",
  RetrieveList = "RetrieveList",
  ListBooks = "ListBooks",
  ListShares = "ListShares",
  ListTemplates = "ListTemplates",
  BookLists = "BookLists",
}

// Types
export interface ListWithCount extends List {
  book_count: number;
  permission: "owner" | "manager" | "editor" | "viewer";
}

export interface ListListsData {
  lists: ListWithCount[];
  total: number;
}

export interface ListBooksData {
  books: ListBook[];
  total: number;
}

export interface RetrieveListData {
  list: List;
  book_count: number;
  permission: "owner" | "manager" | "editor" | "viewer";
}

export interface ListTemplate {
  name: string;
  display_name: string;
  description: string;
  is_ordered: boolean;
  default_sort: string;
}

// List queries
export const useListLists = (
  query: { limit?: number; offset?: number } = {},
  options: Omit<UseQueryOptions<ListListsData, ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<ListListsData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListLists, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/lists", null, query, signal);
    },
  });
};

export const useList = (
  listId?: number,
  options: Omit<UseQueryOptions<RetrieveListData, ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<RetrieveListData, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.RetrieveList, listId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}`, null, null, signal);
    },
  });
};

export const useListBooks = (
  listId?: number,
  query: { limit?: number; offset?: number; sort?: string } = {},
  options: Omit<UseQueryOptions<ListBooksData, ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<ListBooksData, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.ListBooks, listId, query],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}/books`, null, query, signal);
    },
  });
};

export const useListShares = (
  listId?: number,
  options: Omit<UseQueryOptions<ListShare[], ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<ListShare[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.ListShares, listId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}/shares`, null, null, signal);
    },
  });
};

export const useListTemplates = (
  options: Omit<UseQueryOptions<ListTemplate[], ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<ListTemplate[], ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListTemplates],
    queryFn: ({ signal }) => {
      return API.request("GET", "/lists/templates", null, null, signal);
    },
  });
};

export const useBookLists = (
  bookId?: number,
  options: Omit<UseQueryOptions<List[], ShishoAPIError>, "queryKey" | "queryFn"> = {},
) => {
  return useQuery<List[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(bookId),
    ...options,
    queryKey: [QueryKey.BookLists, bookId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/books/${bookId}/lists`, null, null, signal);
    },
  });
};

// Mutations
export const useCreateList = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ payload }: { payload: CreateListPayload }) => {
      return API.request<List>("POST", "/lists", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};

export const useUpdateList = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, payload }: { listId: number; payload: UpdateListPayload }) => {
      return API.request<List>("PATCH", `/lists/${listId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveList, variables.listId] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};

export const useDeleteList = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId }: { listId: number }) => {
      return API.request("DELETE", `/lists/${listId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};

export const useAddBooksToList = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, payload }: { listId: number; payload: AddBooksPayload }) => {
      return API.request("POST", `/lists/${listId}/books`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks, variables.listId] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveList, variables.listId] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookLists] });
    },
  });
};

export const useRemoveBooksFromList = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, payload }: { listId: number; payload: RemoveBooksPayload }) => {
      return API.request("DELETE", `/lists/${listId}/books`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks, variables.listId] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveList, variables.listId] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookLists] });
    },
  });
};

export const useReorderListBooks = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, payload }: { listId: number; payload: ReorderBooksPayload }) => {
      return API.request("PATCH", `/lists/${listId}/books/reorder`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks, variables.listId] });
    },
  });
};

export const useCreateShare = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, payload }: { listId: number; payload: CreateSharePayload }) => {
      return API.request<ListShare>("POST", `/lists/${listId}/shares`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListShares, variables.listId] });
    },
  });
};

export const useUpdateShare = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      listId,
      shareId,
      payload,
    }: {
      listId: number;
      shareId: number;
      payload: UpdateSharePayload;
    }) => {
      return API.request("PATCH", `/lists/${listId}/shares/${shareId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListShares, variables.listId] });
    },
  });
};

export const useDeleteShare = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ listId, shareId }: { listId: number; shareId: number }) => {
      return API.request("DELETE", `/lists/${listId}/shares/${shareId}`);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListShares, variables.listId] });
    },
  });
};

export const useCreateListFromTemplate = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ templateName }: { templateName: string }) => {
      return API.request<List>("POST", `/lists/templates/${templateName}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};
```

**Step 2: Run tygo and lint**

Run: `make tygo && yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/hooks/queries/lists.ts
git commit -m "$(cat <<'EOF'
[Feature] Add lists query hooks for frontend API integration
EOF
)"
```

---

## Phase 6: Frontend - Pages and Components

Due to the length of this plan, I'll provide high-level task descriptions for the remaining frontend work. Each task follows the same pattern of detailed steps.

### Task 6.1: Create Lists Index Page

**Files:**
- Create: `app/components/pages/ListsIndex.tsx`

Create a page that displays all user lists with:
- Empty state with template buttons for new users
- Grid of list cards showing name, book count, and permission badge
- "Create List" button in header

### Task 6.2: Create List Detail Page

**Files:**
- Create: `app/components/pages/ListDetail.tsx`

Create a page that displays a single list with:
- Header with name, description, metadata
- Permission-based action buttons (Edit, Share, Delete)
- Books gallery using existing Gallery component
- Sort dropdown for unordered lists

### Task 6.3: Create List Dialog Component

**Files:**
- Create: `app/components/library/CreateListDialog.tsx`

Create a dialog for creating/editing lists with:
- Name input (required)
- Description textarea (optional)
- Ordered toggle with helper text
- Create/Save button

### Task 6.4: Create Add to List Popover

**Files:**
- Create: `app/components/library/AddToListPopover.tsx`

Create a popover for quick add/remove from lists:
- Shows user's lists with checkmarks for membership
- Click to toggle (immediate save)
- "Create New List" link
- Loading states during mutations

### Task 6.5: Add Lists to TopNav

**Files:**
- Modify: `app/components/library/TopNav.tsx`

Add a "Lists" button to the navigation bar that links to `/lists`.

### Task 6.6: Add Routes

**Files:**
- Modify: `app/router.tsx`

Add routes:
- `/lists` - Lists index page
- `/lists/:id` - List detail page

### Task 6.7: Add "Add to List" to BookItem Context Menu

**Files:**
- Modify: `app/components/library/BookItem.tsx`

Add "Add to List" option in the book card context menu that opens the AddToListPopover.

---

## Phase 7: Bulk Selection System

### Task 7.1: Create Bulk Selection Context

**Files:**
- Create: `app/contexts/BulkSelectionContext.tsx`

Create a React context for managing bulk selection state across galleries.

### Task 7.2: Create Selection Toolbar Component

**Files:**
- Create: `app/components/library/SelectionToolbar.tsx`

Create a floating bottom toolbar showing selection count and bulk action buttons.

### Task 7.3: Update Gallery for Selection Mode

**Files:**
- Modify: `app/components/library/Gallery.tsx`
- Modify: `app/components/library/BookItem.tsx`

Add selection mode support with checkboxes and shift+click range selection.

---

## Phase 8: Share Dialog

### Task 8.1: Create Share Dialog Component

**Files:**
- Create: `app/components/library/ShareListDialog.tsx`

Create a dialog for managing list shares with:
- Current shares list with remove buttons
- User search/select for adding shares
- Permission dropdown
- Visibility warning for book access

---

## Final Steps

### Task F.1: Run Full Test Suite

Run: `make check`
Expected: All tests pass, no lint errors

### Task F.2: Manual Testing Checklist

- [ ] Create list (ordered and unordered)
- [ ] Add books from library gallery
- [ ] Remove books from list
- [ ] Reorder books in ordered list
- [ ] Share list with another user
- [ ] Verify permission restrictions
- [ ] Verify library access filtering
- [ ] Test templates (TBR, Favorites)

### Task F.3: Final Commit

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Feature] Complete lists feature implementation

Full implementation of cross-library book lists with sharing capabilities,
bulk selection, and permission-based access control.
EOF
)"
```
