# Book Genres & Tags Editing Design

## Overview

Add the ability to view and edit genres and tags for a specific book on the book detail page.

## Display: Book Detail Page

Two new sections added after the Series section, before Files:

### Genres Section

- Header: "Genres" (same styling as Authors, Series)
- Content: Horizontal flex-wrap list of badge-style links
- Each badge links to `/libraries/{id}?genre_ids={genreId}` to filter library
- Empty state: "No genres" in muted text

### Tags Section

- Header: "Tags" (same styling)
- Content: Horizontal flex-wrap list of badge-style links
- Each badge links to `/libraries/{id}?tag_ids={tagId}`
- Empty state: "No tags" in muted text

Visual: Uses existing `Badge` component with `variant="secondary"`, clickable with hover state.

## Editing: BookEditDialog

Two new form sections added after Series, before action buttons.

### Genres Input

- Multi-select combobox
- Dropdown shows existing genres from library via `useGenresList`
- Server-side search with 300ms debounce
- Selected genres as removable badges
- Type new name + Enter to create on-the-fly
- Placeholder: "Search or add genres..."

### Tags Input

- Same pattern as genres using `useTagsList`
- Placeholder: "Search or add tags..."

### State Management

```typescript
const [genres, setGenres] = useState<string[]>(
  book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [],
);
const [tags, setTags] = useState<string[]>(
  book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [],
);
```

Payload includes `genres`/`tags` only when changed from original.

## MultiSelectCombobox Component

New reusable component at `app/components/ui/MultiSelectCombobox.tsx`.

### Behavior

- Text input for searching/typing new values
- Selected items as inline badges with × remove button
- Dropdown opens on focus
- Shows filtered server results
- Shows "Create '{text}'" when input doesn't match existing
- Keyboard: arrows to navigate, Enter to select, Escape to close

### Props

```typescript
interface MultiSelectComboboxProps {
  values: string[];
  onChange: (values: string[]) => void;
  options: string[];
  onSearch: (query: string) => void;
  placeholder?: string;
  isLoading?: boolean;
}
```

Built on Radix Popover primitive with existing Badge and Input components.

## Files to Modify

1. **`app/components/ui/MultiSelectCombobox.tsx`** - New reusable component
2. **`app/components/library/BookEditDialog.tsx`** - Add genres/tags editing
3. **`app/components/pages/BookDetail.tsx`** - Add genres/tags display sections

## Backend

No changes required. The `UpdateBookPayload` already supports `genres?: string[]` and `tags?: string[]`. The handler performs find-or-create for each name.
