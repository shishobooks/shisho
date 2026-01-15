# Autogenerate Sort Name Design

## Problem

When editing sort name/title on resources, the current UX shows an input with placeholder text "Leave empty to auto-generate". This is unclear and doesn't show what the auto-generated value will be. Calibre handles this better with an explicit checkbox and live preview.

## Solution

Add a checkbox "Autogenerate sort name/title" (checked by default for auto-generated values) that:
- When checked: disables the input and shows a live preview of the auto-generated value
- When unchecked: enables editing, pre-filling with the auto-generated value as a starting point

## Architecture

**New files:**
- `app/utils/sortname.ts` — Port of backend sort name algorithms
- `app/components/common/SortNameInput.tsx` — Reusable checkbox + input component

**Modified files:**
- `app/components/library/BookEditDialog.tsx` — Use SortNameInput for sort_title
- `app/components/library/MetadataEditDialog.tsx` — Use SortNameInput for sort_name
- `app/components/pages/PersonDetail.tsx` — Pass sort_name_source to dialog
- `app/components/pages/SeriesDetail.tsx` — Pass sort_name_source to dialog

**No backend changes required** — existing empty-string-to-regenerate logic is sufficient.

## Sort Name Utility

Port of `pkg/sortname/sortname.go` (~100-120 lines):

```typescript
// app/utils/sortname.ts

// For books and series - moves leading articles to end
export function forTitle(title: string): string

// For people - converts to "Last, First" format
export function forPerson(name: string): string
```

**`forTitle` algorithm:**
- Handles articles: "The", "A", "An" (case-insensitive)
- "The Hobbit" → "Hobbit, The"
- "A Tale of Two Cities" → "Tale of Two Cities, A"

**`forPerson` algorithm:**
- Strip prefixes: Dr., Mr., Mrs., Prof., Rev., Sir, etc.
- Strip academic suffixes: PhD, M.D., MBA, etc.
- Preserve generational suffixes: Jr., Sr., II, III, etc.
- Handle particles: van, von, de, da, etc.
- "Stephen King" → "King, Stephen"
- "Martin Luther King Jr." → "King, Martin Luther, Jr."
- "Ludwig van Beethoven" → "Beethoven, Ludwig van"

## SortNameInput Component

```typescript
// app/components/common/SortNameInput.tsx

interface SortNameInputProps {
  nameValue: string           // The name/title being edited (for live preview)
  sortValue: string           // Current sort name/title value
  source: DataSource          // "manual" or other (from API)
  type: "title" | "person"    // Which algorithm to use
  onChange: (value: string) => void  // Empty string (auto) or actual value (manual)
}
```

**Behavior:**
- Checkbox starts checked if `source !== "manual"`
- When checked: input disabled, shows live preview, `onChange("")` called
- When unchecked: input editable, pre-fills with auto-generated value on first uncheck

**UI layout:**
```
☑ Autogenerate sort name
┌─────────────────────────────┐
│ King, Stephen               │  ← disabled, shows preview
└─────────────────────────────┘
```

Label adapts: "Autogenerate sort name" for person/series, "Autogenerate sort title" for books.

## Dialog Integration

**BookEditDialog:**
- Replace sort_title input with SortNameInput
- Props: `nameValue={title}`, `sortValue={book.sort_title}`, `source={book.sort_title_source}`, `type="title"`

**MetadataEditDialog:**
- Replace sort_name input with SortNameInput
- Add new `sortNameSource` prop
- Props: `nameValue={name}`, `sortValue={sort_name}`, `source={sortNameSource}`, `type` based on entityType

**PersonDetail / SeriesDetail:**
- Pass `sortNameSource={entity.sort_name_source}` to MetadataEditDialog

## Data Flow

```
User types name/title
       ↓
SortNameInput receives nameValue prop
       ↓
If checkbox checked → forTitle/forPerson() → displays preview in disabled input
       ↓
On save → passes empty string to API → backend auto-generates and preserves source

If checkbox unchecked → input is editable
       ↓
On save → passes actual value to API → backend marks as manual
```
