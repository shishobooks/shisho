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
- Filename
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

Each file displays as a single horizontal row:

```
▶ [EPUB]  The Great Book.epub                    512 KB    ⬇  ✏️
▶ [M4B]   Audiobook Title.m4b     8h 32m • 256 kbps • 1.2 GB    ⬇  ✏️
▶ [CBZ]   Comic Issue 01.cbz      24 pages              45 MB    ⬇  ✏️
```

**Elements (left to right):**
1. **Disclosure chevron** - Points right (▶) when collapsed, down (▼) when expanded. Only visible when secondary metadata exists.
2. **File type badge** - Uppercase, subtle background color
3. **Filename** - Extracted from filepath, truncates with ellipsis if too long
4. **File-specific stats** - Separated by bullet (•):
   - M4B: Duration • Bitrate
   - CBZ: Page count
   - EPUB: None (just size)
5. **File size** - Always shown, right-aligned with stats
6. **Action buttons** - Download and Edit icons, right-aligned

**Changes from current design:**
- Remove left colored border (visual noise)
- Single row instead of multiple stacked rows
- Inline stats separated by bullets
- Chevron provides clear expand affordance

### Narrators Row

For M4B files with narrators, a second line appears immediately below the primary row:

```
▶ [M4B]   Audiobook Title.m4b     8h 32m • 256 kbps • 1.2 GB    ⬇  ✏️
          Narrated by John Smith, Jane Doe
```

**Details:**
- Indented to align with filename (past chevron and badge)
- Prefixed with "Narrated by" in muted text
- Narrator names are clickable links to their person page
- Smaller text size (text-xs) in muted color
- Always visible (not in expandable section)
- Row doesn't appear if no narrators exist

### Expandable Details Section

When the chevron is clicked, secondary metadata appears in an indented block:

```
▼ [EPUB]  The Great Book.epub                    512 KB    ⬇  ✏️
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

Supplements follow the same compact pattern with lighter visual treatment:

```
Supplements (2)

  [PDF]   Reading Guide.pdf                      1.2 MB    ⬇  ✏️
  [PDF]   Author Interview.pdf                   850 KB    ⬇  ✏️
```

**Details:**
- No disclosure chevron (supplements rarely have metadata)
- Same inline layout: badge → filename → size → actions
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
- Primary row: text-sm, font-medium for filename
- Narrator row: text-xs, text-muted-foreground
- Expanded section: text-xs for labels and values

### Animation
- Chevron rotation: 90deg, 150ms ease
- Height expansion: 200ms ease-out
