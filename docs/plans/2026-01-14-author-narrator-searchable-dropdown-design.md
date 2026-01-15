# Author & Narrator Searchable Dropdown Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace text inputs for authors (in BookEditDialog) and narrators (in FileEditDialog) with searchable dropdowns that match the existing genres/tags UX.

**Architecture:** Reuse the existing `MultiSelectCombobox` pattern for narrators (simple list). For authors, use a custom Popover-based approach matching the series dropdown since authors can have roles (CBZ files). Both use the existing `usePeopleList` hook with debounced search.

**Tech Stack:** React, TailwindCSS, shadcn/ui (Popover, Command, Badge), TanStack Query, existing `usePeopleList` hook

---

## Task 1: Add Author Searchable Dropdown to BookEditDialog (Non-CBZ)

Replace the simple text input with a searchable dropdown for non-CBZ books (the simpler case - authors without roles).

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx:87-119` (state setup)
- Modify: `app/components/library/BookEditDialog.tsx:510-555` (non-CBZ author UI)

**Step 1: Add people query imports and state**

Add to imports at top of file:

```typescript
import { usePeopleList } from "@/hooks/queries/people";
```

Add state for author search (after line 118 where `debouncedTagSearch` is defined):

```typescript
const [authorSearch, setAuthorSearch] = useState("");
const debouncedAuthorSearch = useDebounce(authorSearch, 200);
```

Add people query (after line 153 where tags query is defined):

```typescript
// Query for people in this library with server-side search
const { data: peopleData, isLoading: isLoadingPeople } = usePeopleList(
  {
    library_id: book.library_id,
    limit: 50,
    search: debouncedAuthorSearch || undefined,
  },
  { enabled: open && !!book.library_id },
);
```

**Step 2: Reset author search when dialog opens**

Add to the reset effect (inside `useEffect` after line 184):

```typescript
setAuthorSearch("");
```

**Step 3: Replace non-CBZ author UI with MultiSelectCombobox**

Replace the non-CBZ author section (lines 510-555) with:

```typescript
// Non-CBZ files: use MultiSelectCombobox for authors
<MultiSelectCombobox
  isLoading={isLoadingPeople}
  label="People"
  onChange={(names) => setAuthors(names.map((name) => ({ name })))}
  onSearch={setAuthorSearch}
  options={peopleData?.people.map((p) => p.name) || []}
  placeholder="Add author..."
  searchValue={authorSearch}
  values={authors.map((a) => a.name)}
/>
```

**Step 4: Run lint and type check**

Run: `cd app && yarn lint:types && yarn lint:eslint`
Expected: PASS with no errors

**Step 5: Manual test**

1. Open BookEditDialog on a non-CBZ book (EPUB, M4B)
2. Click "Add author..." button
3. Type to search for existing people
4. Select an existing person - verify they appear as badge
5. Type a new name and click "Create" - verify it works
6. Remove an author by clicking X - verify it removes

**Step 6: Commit**

```bash
git add app/components/library/BookEditDialog.tsx
git commit -m "$(cat <<'EOF'
feat: add searchable author dropdown for non-CBZ books

Replace the plain text input with a searchable MultiSelectCombobox
that queries existing people and allows inline creation. This matches
the existing UX for genres and tags.
EOF
)"
```

---

## Task 2: Add Author Searchable Dropdown to BookEditDialog (CBZ)

For CBZ files, authors have roles. Use a Popover-based approach matching the series dropdown pattern.

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx:425-509` (CBZ author UI)

**Step 1: Add state for author popover**

Add state (after the other popover states around line 104):

```typescript
const [authorOpen, setAuthorOpen] = useState(false);
```

**Step 2: Add handler for selecting a person (CBZ)**

Add handler function (after `handleAuthorRoleChange` around line 208):

```typescript
const handleSelectAuthor = (personName: string) => {
  // Check if author is already added (same name, any role)
  if (!authors.some((a) => a.name === personName)) {
    // Default to "Writer" role for CBZ
    setAuthors([...authors, { name: personName, role: AuthorRoleWriter }]);
  }
  setAuthorOpen(false);
  setAuthorSearch("");
};

const handleCreateAuthor = () => {
  const name = authorSearch.trim();
  if (name && !authors.some((a) => a.name === name)) {
    setAuthors([...authors, { name, role: AuthorRoleWriter }]);
  }
  setAuthorOpen(false);
  setAuthorSearch("");
};
```

**Step 3: Compute filtered people and showCreate for CBZ**

Add computed values (after `showCreateOption` for series around line 365):

```typescript
// Filter out already-selected authors from people options
const filteredPeople = useMemo(() => {
  const allPeople = peopleData?.people || [];
  return allPeople.filter(
    (p) => !authors.some((a) => a.name === p.name),
  );
}, [peopleData?.people, authors]);

const showCreateAuthorOption =
  authorSearch.trim() &&
  !filteredPeople.find(
    (p) => p.name.toLowerCase() === authorSearch.toLowerCase(),
  ) &&
  !authors.find(
    (a) => a.name.toLowerCase() === authorSearch.toLowerCase(),
  );
```

**Step 4: Replace CBZ author input with Popover**

Replace the CBZ author add input section (lines 466-508, the `<div className="flex gap-2">` section with the Input/Select/Button) with:

```typescript
{/* Author Combobox for CBZ */}
<Popover modal onOpenChange={setAuthorOpen} open={authorOpen}>
  <PopoverTrigger asChild>
    <Button
      aria-expanded={authorOpen}
      className="w-full justify-between"
      role="combobox"
      variant="outline"
    >
      Add author...
      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
    </Button>
  </PopoverTrigger>
  <PopoverContent align="start" className="w-full p-0">
    <Command shouldFilter={false}>
      <CommandInput
        onValueChange={setAuthorSearch}
        placeholder="Search people..."
        value={authorSearch}
      />
      <CommandList>
        {isLoadingPeople && (
          <div className="p-4 text-center text-sm text-muted-foreground">
            Loading people...
          </div>
        )}
        {!isLoadingPeople &&
          filteredPeople.length === 0 &&
          !showCreateAuthorOption && (
            <div className="p-4 text-center text-sm text-muted-foreground">
              {!debouncedAuthorSearch
                ? "No people in this library. Type to create one."
                : "No matching people."}
            </div>
          )}
        {!isLoadingPeople && (
          <CommandGroup>
            {filteredPeople.map((p) => (
              <CommandItem
                key={p.id}
                onSelect={() => handleSelectAuthor(p.name)}
                value={p.name}
              >
                <Check className="mr-2 h-4 w-4 opacity-0 shrink-0" />
                <span className="truncate" title={p.name}>
                  {p.name}
                </span>
              </CommandItem>
            ))}
            {showCreateAuthorOption && (
              <CommandItem
                onSelect={handleCreateAuthor}
                value={`create-${authorSearch}`}
              >
                <Plus className="mr-2 h-4 w-4 shrink-0" />
                <span className="truncate">
                  Create "{authorSearch}"
                </span>
              </CommandItem>
            )}
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  </PopoverContent>
</Popover>
```

**Step 5: Remove unused state and handlers**

Remove these now-unused items:
- `newAuthor` and `setNewAuthor` state (line 94)
- `newAuthorRole` and `setNewAuthorRole` state (lines 95-97)
- `handleAddAuthor` function (lines 188-198)
- `handleAuthorBlur` function (lines 243-253)
- Reset of `newAuthor` and `newAuthorRole` in useEffect (lines 168-169)
- The pending author logic in `handleSubmit` (lines 268-275)

**Step 6: Run lint and type check**

Run: `cd app && yarn lint:types && yarn lint:eslint`
Expected: PASS with no errors

**Step 7: Manual test**

1. Open BookEditDialog on a CBZ book
2. Click "Add author..." button
3. Search for existing people - verify they appear
4. Select a person - verify they're added with "Writer" role by default
5. Change the role dropdown - verify it updates
6. Create a new person - verify inline creation works
7. Remove an author - verify removal works

**Step 8: Commit**

```bash
git add app/components/library/BookEditDialog.tsx
git commit -m "$(cat <<'EOF'
feat: add searchable author dropdown for CBZ books

Replace the text input with a searchable Popover matching the series
dropdown pattern. New authors default to "Writer" role. Removes the
manual add/blur handlers in favor of the dropdown.
EOF
)"
```

---

## Task 3: Add Narrator Searchable Dropdown to FileEditDialog

Replace the narrator text input in FileEditDialog with a searchable dropdown.

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx:36-41` (imports)
- Modify: `app/components/library/FileEditDialog.tsx:87-90` (state)
- Modify: `app/components/library/FileEditDialog.tsx:396-441` (narrator UI)

**Step 1: Add imports**

Add to imports:

```typescript
import { MultiSelectCombobox } from "@/components/ui/MultiSelectCombobox";
import { usePeopleList } from "@/hooks/queries/people";
```

**Step 2: Add narrator search state and query**

Add state (after `newNarrator` state around line 90):

```typescript
const [narratorSearch, setNarratorSearch] = useState("");
const debouncedNarratorSearch = useDebounce(narratorSearch, 200);
```

Add people query (after the imprints query around line 137):

```typescript
// Query for people in this library with server-side search (for narrators)
const { data: peopleData, isLoading: isLoadingPeople } = usePeopleList(
  {
    library_id: file.library_id,
    limit: 50,
    search: debouncedNarratorSearch || undefined,
  },
  { enabled: open },
);
```

**Step 3: Reset narrator search in useEffect**

Add to the reset effect (inside `useEffect` after line 155):

```typescript
setNarratorSearch("");
```

**Step 4: Replace narrator UI with MultiSelectCombobox**

Replace the entire narrator section (lines 396-441) with:

```typescript
{/* Narrators (only for M4B files) */}
{file.file_type === "m4b" && (
  <div className="space-y-2">
    <Label>Narrators</Label>
    <MultiSelectCombobox
      isLoading={isLoadingPeople}
      label="People"
      onChange={setNarrators}
      onSearch={setNarratorSearch}
      options={peopleData?.people.map((p) => p.name) || []}
      placeholder="Add narrator..."
      searchValue={narratorSearch}
      values={narrators}
    />
  </div>
)}
```

**Step 5: Remove unused narrator state and handlers**

Remove:
- `newNarrator` state (line 90)
- `handleAddNarrator` function (lines 158-163)
- `handleRemoveNarrator` function (lines 165-167)

**Step 6: Run lint and type check**

Run: `cd app && yarn lint:types && yarn lint:eslint`
Expected: PASS with no errors

**Step 7: Manual test**

1. Open FileEditDialog on an M4B file
2. Verify "Narrators" section shows the searchable dropdown
3. Click "Add narrator..." button
4. Search for existing people
5. Select a person - verify they appear as badge
6. Create a new person - verify inline creation works
7. Remove a narrator - verify removal works
8. Save changes - verify they persist

**Step 8: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "$(cat <<'EOF'
feat: add searchable narrator dropdown for M4B files

Replace the text input with a MultiSelectCombobox that queries existing
people and allows inline creation. This matches the author dropdown UX.
EOF
)"
```

---

## Task 4: Run Full Test Suite and Final Cleanup

**Step 1: Run make check**

Run: `make check`
Expected: All checks pass (tests, Go lint, JS lint)

**Step 2: Remove any unused imports**

Check both modified files for unused imports and remove them:

```bash
cd app && yarn lint:eslint --fix
```

**Step 3: Manual end-to-end test**

Test the complete workflow:

1. **Non-CBZ book author dropdown:**
   - Open BookEditDialog on an EPUB
   - Add an existing author via search
   - Create a new author
   - Remove an author
   - Save and verify changes persist

2. **CBZ book author dropdown with roles:**
   - Open BookEditDialog on a CBZ
   - Add an existing author (should default to Writer)
   - Change role to Penciller
   - Create a new author
   - Save and verify changes persist with correct roles

3. **M4B narrator dropdown:**
   - Open FileEditDialog on an M4B
   - Add an existing narrator via search
   - Create a new narrator
   - Remove a narrator
   - Save and verify changes persist

**Step 4: Final commit (if any cleanup needed)**

```bash
git add -A
git commit -m "chore: cleanup unused imports and formatting"
```

---

## Summary

| File | Changes |
|------|---------|
| `app/components/library/BookEditDialog.tsx` | Add `usePeopleList` query, replace text inputs with searchable dropdowns for both CBZ and non-CBZ books |
| `app/components/library/FileEditDialog.tsx` | Add `usePeopleList` query, replace text input with `MultiSelectCombobox` for narrators |

**No backend changes required** - the existing `GET /people?search=...` endpoint already supports this feature.

**Key patterns followed:**
- Debounced search (200ms) matching series/genres/tags
- Server-side search via `usePeopleList` hook
- Inline creation via "Create [name]" option
- Badge-based display of selected items
- Popover with Command components for search UI
