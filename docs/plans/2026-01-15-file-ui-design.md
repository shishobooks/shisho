# File UI Redesign

## Problem

The current file display on the book detail page is visually busy. When a file has full metadata (narrators, publisher, imprint, release date, URL, identifiers), it spans multiple rows with inline text that's hard to scan. The design needs to remain informative but be easier to understand at a glance.

## Design Goals

1. Clean, scannable layout that works for all file types (EPUB, M4B, CBZ)
2. Primary information visible immediately
3. Secondary information accessible but not cluttering the default view
4. Consistent treatment across file types while respecting their differences

## Information Hierarchy

**Primary (always visible):**
- File type (EPUB/M4B/CBZ)
- Name (prominent display, defaults to title from metadata)
- Filename (muted, smaller text below name)
- File size
- Duration and bitrate (M4B only)
- Page count (CBZ only)
- Narrators (M4B only, when present)
- Download and Edit actions

**Secondary (expandable):**
- Publisher
- Imprint
- Release date
- URL
- Identifiers (ISBN, ASIN, etc.)

## Layout Design

### Primary Row

Each file displays with name prominently and filename underneath:

```
▶ [EPUB]  The Great Book                                512 KB    ⬇  ✏️
          The Great Book.epub

▶ [M4B]   Audiobook Title             8h 32m • 256 kbps • 1.2 GB    ⬇  ✏️
          Author Name/Series/Audiobook Title.m4b
          Narrated by John Smith, Jane Doe

▶ [CBZ]   Comic Issue 01              24 pages              45 MB    ⬇  ✏️
          Publisher/Series Name/Comic Issue 01.cbz
```

**Elements (left to right):**
1. **Disclosure chevron** - Points right (▶) when collapsed, down (▼) when expanded. Only visible when secondary metadata exists.
2. **File type badge** - Uppercase, subtle background color
3. **Name** - Prominent display (font-medium), defaults to title from file metadata, truncates with ellipsis if too long
4. **File-specific stats** - Separated by bullet (•):
   - M4B: Duration • Bitrate
   - CBZ: Page count
   - EPUB: None (just size)
5. **File size** - Always shown, right-aligned with stats
6. **Action buttons** - Download and Edit icons, right-aligned

**Filename row (below primary):**
- Indented to align with name (past chevron and badge)
- Displayed in `text-xs text-muted-foreground`
- Has `truncate` class since paths can be long
- Has `title` attribute with full filepath for hover tooltip
- Always visible (not in expandable section)

**Changes from current design:**
- Name displayed prominently instead of filename
- Filename moved to muted secondary line
- Remove left colored border (visual noise)
- Inline stats separated by bullets
- Chevron provides clear expand affordance

### Narrators Row

For M4B files with narrators, a third line appears below the filename row:

```
▶ [M4B]   Audiobook Title             8h 32m • 256 kbps • 1.2 GB    ⬇  ✏️
          Author Name/Series/Audiobook Title.m4b
          Narrated by John Smith, Jane Doe
```

**Details:**
- Indented to align with name (past chevron and badge)
- Prefixed with "Narrated by" in muted text
- Narrator names are clickable links to their person page
- Smaller text size (text-xs) in muted color
- Always visible (not in expandable section)
- Row doesn't appear if no narrators exist

### Expandable Details Section

When the chevron is clicked, secondary metadata appears in an indented block:

```
▼ [EPUB]  The Great Book                                512 KB    ⬇  ✏️
          The Great Book.epub
          ┌─────────────────────────────────────────────────────
          │  Publisher    Penguin Random House
          │  Imprint      Vintage Books
          │  Released     March 15, 2023
          │  URL          https://example.com/book
          │
          │  ISBN-13      978-0-14-028329-7
          │  ASIN         B08N5WRWNW
          └─────────────────────────────────────────────────────
```

**Details:**
- Indented to align with content area
- Subtle background tint or left border to group visually
- Two-column key-value layout
- Labels in muted text, values in regular text
- Identifiers grouped at bottom, each on its own line
- URL is clickable link, truncated if very long
- Smooth expand/collapse animation
- Only shows fields that have values
- Chevron hidden entirely if no secondary metadata exists

### Supplements Section

Supplements follow the same pattern with name prominent and filename underneath:

```
Supplements (2)

  [PDF]   Reading Guide                                 1.2 MB    ⬇  ✏️
          Reading Guide.pdf

  [PDF]   Author Interview                              850 KB    ⬇  ✏️
          Author Interview.pdf
```

**Details:**
- No disclosure chevron (supplements rarely have metadata)
- Same layout: badge → name → size → actions, with filename underneath
- Filename has `truncate` class and `title` attribute
- Slightly muted compared to main files
- Section only appears if supplements exist
- If a supplement has expandable metadata, show the chevron

## Component Changes

### Files to Modify

- `app/components/pages/BookDetail.tsx` - Main file list rendering

### New State

- `expandedFileIds: Set<number>` - Tracks which files are expanded

### Interaction

- Click chevron to toggle expanded state
- Consider: keyboard accessibility (Enter/Space to toggle)
- Consider: "Expand All" / "Collapse All" if many files

## Visual Specifications

### Spacing
- Gap between files: 8px (space-y-2)
- Padding within expanded section: 12px
- Indentation for narrator/expanded rows: align with filename start

### Colors
- Chevron: muted foreground color
- Expanded section background: subtle muted (e.g., bg-muted/50)
- Labels: text-muted-foreground
- Values: default text color

### Typography
- Primary row: text-sm, font-medium for name
- Filename row: text-xs, text-muted-foreground, truncate class, title attribute with full path
- Narrator row: text-xs, text-muted-foreground
- Expanded section: text-xs for labels and values

### Animation
- Chevron rotation: 90deg, 150ms ease
- Height expansion: 200ms ease-out
