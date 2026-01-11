# Book Genres & Tags Editing Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add viewing and editing of genres and tags on the book detail page.

**Architecture:** Display genres/tags as badge sections on BookDetail page. Edit via multi-select combobox in BookEditDialog. Follow existing series combobox pattern but allow multiple selections.

**Tech Stack:** React, TypeScript, Radix UI (Command/Popover), TanStack Query, TailwindCSS

---

## Task 1: Create MultiSelectCombobox Component

**Files:**

- Create: `app/components/ui/MultiSelectCombobox.tsx`

**Step 1: Create the component file**

```typescript
import { Check, ChevronsUpDown, Plus, X } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

interface MultiSelectComboboxProps {
  values: string[];
  onChange: (values: string[]) => void;
  options: string[];
  onSearch: (query: string) => void;
  searchValue: string;
  placeholder?: string;
  isLoading?: boolean;
  label: string;
}

export function MultiSelectCombobox({
  values,
  onChange,
  options,
  onSearch,
  searchValue,
  placeholder = "Search...",
  isLoading = false,
  label,
}: MultiSelectComboboxProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = (value: string) => {
    if (!values.includes(value)) {
      onChange([...values, value]);
    }
    onSearch("");
  };

  const handleCreate = () => {
    const trimmed = searchValue.trim();
    if (trimmed && !values.some((v) => v.toLowerCase() === trimmed.toLowerCase())) {
      onChange([...values, trimmed]);
    }
    onSearch("");
  };

  const handleRemove = (value: string) => {
    onChange(values.filter((v) => v !== value));
  };

  // Filter out already-selected values from options
  const filteredOptions = options.filter(
    (opt) => !values.some((v) => v.toLowerCase() === opt.toLowerCase())
  );

  const showCreateOption =
    searchValue.trim() &&
    !filteredOptions.some(
      (opt) => opt.toLowerCase() === searchValue.toLowerCase()
    ) &&
    !values.some((v) => v.toLowerCase() === searchValue.toLowerCase());

  return (
    <div className="space-y-2">
      {/* Selected values as badges */}
      {values.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {values.map((value) => (
            <Badge className="flex items-center gap-1" key={value} variant="secondary">
              {value}
              <button
                className="ml-1 cursor-pointer hover:text-destructive"
                onClick={() => handleRemove(value)}
                type="button"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Combobox */}
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className="w-full justify-between"
            role="combobox"
            variant="outline"
          >
            {placeholder}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-full p-0">
          <Command shouldFilter={false}>
            <CommandInput
              onValueChange={onSearch}
              placeholder={`Search ${label.toLowerCase()}...`}
              value={searchValue}
            />
            <CommandList>
              {isLoading && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  Loading...
                </div>
              )}
              {!isLoading && filteredOptions.length === 0 && !showCreateOption && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  {!searchValue
                    ? `No ${label.toLowerCase()} available. Type to create one.`
                    : `No matching ${label.toLowerCase()}.`}
                </div>
              )}
              {!isLoading && (
                <CommandGroup>
                  {filteredOptions.map((opt) => (
                    <CommandItem key={opt} onSelect={() => handleSelect(opt)} value={opt}>
                      <Check className="mr-2 h-4 w-4 opacity-0" />
                      {opt}
                    </CommandItem>
                  ))}
                  {showCreateOption && (
                    <CommandItem onSelect={handleCreate} value={`create-${searchValue}`}>
                      <Plus className="mr-2 h-4 w-4" />
                      Create "{searchValue}"
                    </CommandItem>
                  )}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
```

**Step 2: Verify file was created**

Run: `ls -la app/components/ui/MultiSelectCombobox.tsx`
Expected: File exists

**Step 3: Commit**

```bash
git add app/components/ui/MultiSelectCombobox.tsx
git commit -m "feat: add MultiSelectCombobox component for genre/tag editing"
```

---

## Task 2: Add Genres/Tags Editing to BookEditDialog

**Files:**

- Modify: `app/components/library/BookEditDialog.tsx`

**Step 1: Add imports for genres/tags hooks**

Add after the existing imports (around line 36):

```typescript
import { useGenresList } from "@/hooks/queries/genres";
import { useTagsList } from "@/hooks/queries/tags";
```

Also add the MultiSelectCombobox import near the top:

```typescript
import { MultiSelectCombobox } from "@/components/ui/MultiSelectCombobox";
```

**Step 2: Add state for genres and tags**

Add after the `seriesSearch` state (around line 101-102):

```typescript
const [genres, setGenres] = useState<string[]>(
  book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [],
);
const [genreSearch, setGenreSearch] = useState("");
const debouncedGenreSearch = useDebounce(genreSearch, 200);

const [tags, setTags] = useState<string[]>(
  book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [],
);
const [tagSearch, setTagSearch] = useState("");
const debouncedTagSearch = useDebounce(tagSearch, 200);
```

**Step 3: Add queries for genres and tags**

Add after the series query (around line 117):

```typescript
// Query for genres in this library with server-side search
const { data: genresData, isLoading: isLoadingGenres } = useGenresList(
  {
    library_id: book.library_id,
    limit: 50,
    search: debouncedGenreSearch || undefined,
  },
  { enabled: open && !!book.library_id },
);

// Query for tags in this library with server-side search
const { data: tagsData, isLoading: isLoadingTags } = useTagsList(
  {
    library_id: book.library_id,
    limit: 50,
    search: debouncedTagSearch || undefined,
  },
  { enabled: open && !!book.library_id },
);
```

**Step 4: Reset genres/tags in useEffect**

Add to the useEffect that resets form (around line 138, inside the `if (open)` block):

```typescript
setGenres(
  book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [],
);
setGenreSearch("");
setTags(book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || []);
setTagSearch("");
```

**Step 5: Update payload type in handleSubmit**

Update the payload type definition (around line 210) to include genres and tags:

```typescript
const payload: {
  title?: string;
  sort_title?: string;
  subtitle?: string;
  authors?: AuthorInput[];
  series?: SeriesInput[];
  genres?: string[];
  tags?: string[];
} = {};
```

**Step 6: Add genres/tags change detection in handleSubmit**

Add after the series change detection (around line 262):

```typescript
// Check if genres changed
const originalGenres =
  book.book_genres?.map((bg) => bg.genre?.name || "").filter(Boolean) || [];
if (JSON.stringify(genres.sort()) !== JSON.stringify(originalGenres.sort())) {
  payload.genres = genres.filter((g) => g.trim());
}

// Check if tags changed
const originalTags =
  book.book_tags?.map((bt) => bt.tag?.name || "").filter(Boolean) || [];
if (JSON.stringify(tags.sort()) !== JSON.stringify(originalTags.sort())) {
  payload.tags = tags.filter((t) => t.trim());
}
```

**Step 7: Add Genres section to the form UI**

Add after the Series section closing `</div>` (around line 565), before the `</div>` that closes the `space-y-6 py-4` div:

```typescript
{/* Genres */}
<div className="space-y-2">
  <Label>Genres</Label>
  <MultiSelectCombobox
    isLoading={isLoadingGenres}
    label="Genres"
    onChange={setGenres}
    onSearch={setGenreSearch}
    options={genresData?.genres.map((g) => g.name) || []}
    placeholder="Add genres..."
    searchValue={genreSearch}
    values={genres}
  />
</div>

{/* Tags */}
<div className="space-y-2">
  <Label>Tags</Label>
  <MultiSelectCombobox
    isLoading={isLoadingTags}
    label="Tags"
    onChange={setTags}
    onSearch={setTagSearch}
    options={tagsData?.tags.map((t) => t.name) || []}
    placeholder="Add tags..."
    searchValue={tagSearch}
    values={tags}
  />
</div>
```

**Step 8: Verify lint passes**

Run: `yarn lint:types && yarn lint:eslint`
Expected: No errors

**Step 9: Commit**

```bash
git add app/components/library/BookEditDialog.tsx
git commit -m "feat: add genres and tags editing to book edit dialog"
```

---

## Task 3: Add Genres/Tags Display to BookDetail Page

**Files:**

- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add Genres section after Series section**

Find the Series section (ends around line 354) and add after it, before the `<Separator />`:

```typescript
{/* Genres */}
{book.book_genres && book.book_genres.length > 0 && (
  <div>
    <h3 className="font-semibold mb-2">Genres</h3>
    <div className="flex flex-wrap gap-2">
      {book.book_genres.map((bg) => (
        <Link
          key={bg.id}
          to={`/libraries/${libraryId}?genre_ids=${bg.genre_id}`}
        >
          <Badge
            className="cursor-pointer hover:bg-secondary/80"
            variant="secondary"
          >
            {bg.genre?.name ?? "Unknown"}
          </Badge>
        </Link>
      ))}
    </div>
  </div>
)}

{/* Tags */}
{book.book_tags && book.book_tags.length > 0 && (
  <div>
    <h3 className="font-semibold mb-2">Tags</h3>
    <div className="flex flex-wrap gap-2">
      {book.book_tags.map((bt) => (
        <Link
          key={bt.id}
          to={`/libraries/${libraryId}?tag_ids=${bt.tag_id}`}
        >
          <Badge
            className="cursor-pointer hover:bg-secondary/80"
            variant="secondary"
          >
            {bt.tag?.name ?? "Unknown"}
          </Badge>
        </Link>
      ))}
    </div>
  </div>
)}
```

**Step 2: Verify lint passes**

Run: `yarn lint:types && yarn lint:eslint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "feat: display genres and tags on book detail page"
```

---

## Task 4: Manual Testing

**Step 1: Start the development server**

Run: `make start` (or verify it's already running)

**Step 2: Test display**

- Navigate to a book detail page
- Verify genres section appears if book has genres
- Verify tags section appears if book has tags
- Click a genre badge - should filter library by that genre
- Click a tag badge - should filter library by that tag

**Step 3: Test editing**

- Click "Edit" button on book detail page
- Scroll to Genres section in dialog
- Click "Add genres..." button
- Type to search existing genres
- Select a genre - should appear as badge
- Type new genre name, click "Create" - should appear as badge
- Click X on badge to remove
- Repeat for Tags section
- Click "Save Changes"
- Verify genres/tags updated on book detail page

**Step 4: Final lint check**

Run: `make check`
Expected: All checks pass
