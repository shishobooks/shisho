# File Chapters Frontend Design

This document describes the frontend implementation for viewing and editing chapters across different file types (EPUB, CBZ, M4B).

## Overview

Add a dedicated File Detail page with tabbed interface for viewing file metadata and managing chapters. Each file type has different chapter editing capabilities based on its format constraints.

## Page Structure

### Route

`/libraries/:libraryId/books/:bookId/files/:fileId`

### Layout

- **Header** - File name, breadcrumbs (`Library > Book Title > File Name`), back button
- **Tabs** - Details | Chapters
- **Tab-specific actions** - Each tab has its own Edit button in the top-right

### Tab Behaviors

| Tab | View Mode | Edit Mode |
|-----|-----------|-----------|
| Details | Read-only metadata display | Edit button opens existing FileEditDialog |
| Chapters | Read-only chapter list | Edit button enables inline editing with Save/Cancel |

## Chapter Display (View Mode)

The chapter list adapts based on file type:

| Type | Position Display | Additional Info |
|------|------------------|-----------------|
| EPUB | None | Indented hierarchy with expand/collapse toggles |
| CBZ | "Page N" | Small thumbnail (~60px) of start page |
| M4B | "HH:MM:SS.mmm" | Timestamp format |

### CBZ Uncovered Pages Warning

If page 0 isn't covered by any chapter, show a highlighted row at the top:
- Display: "Pages 0-N not in any chapter"
- Muted hint: "Click to add chapter"
- Only appears when first chapter starts after page 0

### Empty State

- "No chapters" message
- "Add Chapter" button (hidden for EPUB)

## Chapter Editing by File Type

### EPUB

Most restricted editing - hrefs are tied to internal file structure.

**Allowed:**
- Edit chapter title (inline text input)
- Delete chapter (removes chapter and all children)

**Not allowed:**
- Add new chapters
- Reorder chapters
- Edit hrefs

**Delete confirmation:** When deleting a parent chapter, confirm: "Delete [title] and its N chapters?"

### CBZ

Full editing with visual page selection.

**Allowed:**
- Edit chapter title
- Change start page
- Add new chapter
- Delete chapter

**Page selector:**
- Number input with prev/next buttons (±1)
- Small thumbnail (~60-80px) showing selected page
- Clicking thumbnail opens Page Picker Dialog

**Page Picker Dialog:**
- Grid of page thumbnails
- Initial load: current page ±10 (or first 10 if no current page)
- "Load previous/next 10" buttons at edges
- Buttons hidden when at bounds (page < 0 or ≥ pageCount)
- Current selection highlighted
- Clicking thumbnail selects and closes dialog

**Auto-reorder:** On blur, chapters reorder by start page; sort_orders update accordingly.

**Add Chapter defaults:**
- Start page: last chapter's start page + 1
- Title pattern detection from last chapter:
  - "Chapter 3" → "Chapter 4"
  - "Ch. 3" → "Ch. 4"
  - "Chapter 3: The Title" → "Chapter 4"
  - "3: The Title" → "4"
  - No pattern → empty input

### M4B

Full editing with timestamp positioning and audio preview.

**Allowed:**
- Edit chapter title
- Change start timestamp
- Add new chapter
- Delete chapter
- Play audio preview from timestamp

**Timestamp input:**
- Format: `HH:MM:SS.mmm`
- `−` / `+` buttons adjust by 1 second
- On blur, chapters auto-reorder by timestamp
- Validation: cannot exceed file's `audiobook_duration_seconds`

**Play button:**
- Plays audio from chapter's timestamp for ~10 seconds
- Button toggles to stop icon while playing
- Clicking stop halts playback immediately
- Only one chapter plays at a time

**Add Chapter defaults:**
- Start timestamp: last chapter's timestamp + 1 second
- Title pattern detection (same rules as CBZ)
- No pattern → empty input

## Data Flow

### Existing API

- `GET /books/files/:id/chapters` - Returns nested chapter tree
- `PUT /books/files/:id/chapters` - Replaces all chapters atomically

### Frontend State

```tsx
const [isEditing, setIsEditing] = useState(false);
const [editedChapters, setEditedChapters] = useState<ChapterInput[]>([]);
```

- Enter edit mode: copy chapters to local state
- Cancel: discard local changes
- Save: submit entire chapter list via PUT

### New Mutation Hook

```tsx
// app/hooks/queries/chapters.ts
export const useUpdateFileChapters = (fileId: number) => {
  return useMutation({
    mutationFn: (chapters: ChapterInput[]) =>
      API.request("PUT", `/books/files/${fileId}/chapters`, { chapters }),
    onSuccess: () => {
      queryClient.invalidateQueries([QueryKey.FileChapters, fileId]);
    },
  });
};
```

## M4B Audio Player

Simple per-chapter play buttons, no persistent player bar.

```tsx
const audioRef = useRef<HTMLAudioElement>(null);
const [playingChapterId, setPlayingChapterId] = useState<number | null>(null);

// Hidden audio element
<audio ref={audioRef} src={`/api/books/files/${fileId}/stream`} />
```

**New API endpoint:** `GET /books/files/:id/stream`
- Streams M4B audio file
- Supports `Range` headers for seeking
- Returns `audio/mp4` content type

## Component Structure

```
app/
├── components/
│   ├── pages/
│   │   └── FileDetail.tsx              # Main page with tabs
│   └── files/
│       ├── FileDetailsTab.tsx          # Read-only metadata display
│       ├── FileChaptersTab.tsx         # Chapter list + editing
│       ├── ChapterRow.tsx              # Single chapter row (view/edit)
│       ├── CBZPagePicker.tsx           # Thumbnail grid dialog
│       └── CBZPageThumbnail.tsx        # Single page thumbnail
├── hooks/
│   └── queries/
│       └── chapters.ts                 # Add useUpdateFileChapters
└── router.tsx                          # Add file detail route

pkg/
└── books/
    └── handlers.go                     # Add file stream endpoint
```

## Dependencies

- CBZPageThumbnail reuses existing `/books/files/:id/pages/:page` endpoint
- Audio streaming requires new endpoint with Range header support
