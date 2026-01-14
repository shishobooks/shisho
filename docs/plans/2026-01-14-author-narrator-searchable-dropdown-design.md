# Author & Narrator Searchable Dropdown

## Overview

Replace text inputs for authors and narrators with searchable dropdowns using the existing `MultiSelectCombobox` pattern. This matches the existing UX for series, genres, and tags.

## Key Decisions

- **Shared people list** - One unified dropdown for both authors and narrators (matches existing `Person` data model)
- **Select person, then role** - For CBZ authors, select person first, then assign role via separate dropdown
- **Narrators in FileEditDialog** - Searchable dropdown in the file edit dialog
- **Inline creation** - Allow creating new people from the dropdown (like genres/tags)

## Author Dropdown

**Location:** `BookEditDialog.tsx`

**Current flow (text input):**
1. User types author name in text field
2. Clicks "Add" or presses Enter
3. Author appears in list (with role dropdown for CBZ)

**New flow (searchable dropdown):**
1. User clicks "Add author..." button
2. Popover opens with searchable combobox
3. User types to search existing people
4. Either selects existing person OR clicks "Create [name]" for new
5. Person is added to author list
6. For CBZ: role dropdown appears next to added author (defaults to "Writer")

**UI structure:**
```
Authors:
┌─────────────────────────────────────────┐
│ Jane Smith          [Writer ▼]    [×]   │
│ John Doe            [Penciller ▼] [×]   │
└─────────────────────────────────────────┘
[+ Add author...]

↓ (clicking "Add author...")

┌─────────────────────────────────────────┐
│ 🔍 Search people...                     │
├─────────────────────────────────────────┤
│ Jane Smith                              │
│ John Doe                                │
│ + Create "New Name"                     │
└─────────────────────────────────────────┘
```

## Narrator Dropdown

**Location:** `FileEditDialog.tsx`

**UI structure:**
```
Edit File: audiobook.m4b
┌─────────────────────────────────────────┐
│ Title: [________________]               │
│                                         │
│ Narrators:                              │
│ ┌─────────────────────────────────────┐ │
│ │ Morgan Freeman              [×]     │ │
│ │ Sarah Jones                 [×]     │ │
│ └─────────────────────────────────────┘ │
│ [+ Add narrator...]                     │
│                                         │
│              [Cancel]  [Save]           │
└─────────────────────────────────────────┘
```

**Flow:**
- Same searchable dropdown pattern as authors
- No roles for narrators (just the person)
- Uses the same `usePeopleList` hook
- Changes saved via existing `PATCH /books/:id/files/:fileId` endpoint

## Implementation

**Files to create:**
- `app/hooks/usePeopleList.ts` - Hook for searching people (mirrors `useSeriesList` pattern)

**Files to modify:**
1. `app/components/BookEditDialog.tsx` - Replace author text input with searchable dropdown
2. `app/components/FileEditDialog.tsx` - Add narrator searchable dropdown

**No backend changes** - Existing `GET /people?search=...` endpoint already supports this.

**Reused components:**
- `MultiSelectCombobox` - Already handles search, loading states, inline creation
- Existing author role dropdown (for CBZ) stays as-is, just triggered after person selection

## Out of Scope

- No changes to the people management page
- No new API endpoints
- No changes to how data is saved (same `FindOrCreatePerson` flow)
