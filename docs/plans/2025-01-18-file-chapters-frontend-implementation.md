# File Chapters Frontend Implementation Plan

**Goal:** Add a File Detail page with tabbed interface for viewing/editing file metadata and chapters, with file-type-specific editing capabilities.

**Design Spec:** `docs/plans/2025-01-18-file-chapters-frontend-design.md`

**Architecture:** New FileDetail page with two tabs (Details and Chapters). Details tab shows read-only metadata with edit button opening existing FileEditDialog. Chapters tab shows hierarchical chapter list with inline editing for CBZ/M4B, limited editing for EPUB. M4B chapters include audio preview via new streaming endpoint.

**Tech Stack:** React 19, TypeScript, TailwindCSS, Tanstack Query, Go/Echo backend

---

## Feature 1: Add Tabs UI Component

- [x] Task 1.1 Create tabs.tsx in app/components/ui/
  - Use shadcn/ui Tabs component pattern (Radix Tabs primitives)
  - Export: Tabs, TabsList, TabsTrigger, TabsContent
  - Style TabsTrigger with active state indicator (border-b-2 or similar)
  - Follow existing UI component patterns from dialog.tsx
  - File: `app/components/ui/tabs.tsx`

- [x] Task 1.2 Run `yarn lint:types` to verify no type errors

## Feature 2: Add ChapterInput Type Generation

- [x] Task 2.1 Add ChapterInput to tygo.yaml for generation
  - Add `chapters.ChapterInput` to the types list in tygo.yaml
  - This makes the type available in frontend for API requests
  - File: `tygo.yaml`

- [x] Task 2.2 Run `make tygo` to generate types
  - Verify ChapterInput appears in `app/types/generated/`

- [x] Task 2.3 Export ChapterInput from types index
  - Add to the exports in `app/types/index.ts` if not auto-exported
  - Only if needed (check generated output first)

## Feature 3: Add useUpdateFileChapters Mutation Hook

- [x] Task 3.1 Add useUpdateFileChapters hook
  - Add to `app/hooks/queries/chapters.ts`
  - Signature: `useUpdateFileChapters(fileId: number)`
  - MutationFn: `PUT /books/files/${fileId}/chapters` with `{ chapters: ChapterInput[] }`
  - onSuccess: invalidate `[QueryKey.FileChapters, fileId]`
  - Follow pattern from useUpdateFile in books.ts
  - Use the generated ChapterInput type

- [x] Task 3.2 Run `yarn lint:types` to verify no type errors

## Feature 4: Add M4B Audio Stream Endpoint (Backend)

- [x] Task 4.1 Write failing test for GET /books/files/:id/stream
  - Test that endpoint returns audio/mp4 content type for M4B files
  - Test that endpoint returns 404 for non-M4B files (EPUB/CBZ)
  - Test that endpoint returns 404 for non-existent file
  - Test that endpoint respects Range headers:
    - Without Range header: returns full file with 200
    - With `Range: bytes=0-999`: returns 206 Partial Content with `Content-Range: bytes 0-999/totalsize`
    - Verify returned bytes match expected range
  - Test that endpoint returns 403 for unauthorized library access
  - Test that Accept-Ranges: bytes header is present in response
  - File: `pkg/books/handlers_test.go` (add new test function)

- [x] Task 4.2 Run tests to verify they fail
  - Run `make test` and confirm the new tests fail with "route not found"

- [x] Task 4.3 Implement streamFile handler
  - Check file exists and is M4B type
  - Check library access permissions
  - Use echo's Stream or File response with proper headers
  - Support Range header for seeking (Accept-Ranges: bytes)
  - Return Content-Type: audio/mp4
  - File: `pkg/books/handlers.go`

- [x] Task 4.4 Register route
  - `GET /books/files/:id/stream` (matches existing /books/files/ pattern)
  - Add to `pkg/books/routes.go` alongside other file routes
  - Use authMiddleware.RequireLibraryAccess for the file's library

- [x] Task 4.5 Run tests to verify they pass

## Feature 5: Add File Detail Route and Navigation

- [x] Task 5.1 Add route to router.tsx
  - Path: `libraries/:libraryId/books/:bookId/files/:fileId`
  - Element: `<ProtectedRoute checkLibraryAccess><FileDetail /></ProtectedRoute>`
  - Place after the book detail route
  - File: `app/router.tsx`

- [x] Task 5.2 Update BookDetail.tsx to link to FileDetail
  - Make file names/rows clickable to navigate to file detail page
  - Use Link component: `<Link to={`/libraries/${libraryId}/books/${book.id}/files/${file.id}`}>`
  - Apply hover:underline or cursor-pointer for visual affordance
  - Apply to both main files and supplement files sections
  - File: `app/components/pages/BookDetail.tsx`

- [x] Task 5.3 Run `yarn lint:types` to verify (will fail until FileDetail exists)

## Feature 6: Create FileDetail Page Shell

- [x] Task 6.1 Create FileDetail.tsx page component
  - Location: `app/components/pages/FileDetail.tsx`
  - Extract fileId, bookId, libraryId from useParams
  - Fetch file data using existing useBook hook (file comes with book)
  - Find file in book.files array by fileId
  - Show loading state while fetching
  - Show "File not found" if file doesn't exist
  - Structure:
    ```
    TopNav
    max-w-7xl container
      Header (filename, breadcrumbs, back button)
      Tabs (Details | Chapters)
      Tab content area
    ```

- [x] Task 6.2 Add TopNav breadcrumbs
  - Format: `Library Name > Book Title > File Name`
  - Fetch library name using useLibrary(libraryId) hook
  - Library name links to `/libraries/:libraryId`
  - Book title links to `/libraries/:libraryId/books/:bookId`
  - File name is current page (no link)
  - Back button returns to book detail page

- [x] Task 6.3 Add tab state management
  - State: `const [activeTab, setActiveTab] = useState<'details' | 'chapters'>('details')`
  - Import Tabs, TabsList, TabsTrigger, TabsContent from `@/components/ui/tabs` (created in Task 1.1)
  - Render Tabs component with Details and Chapters triggers
  - Edit button behavior per-tab:
    - Details tab: Edit button opens FileEditDialog (state: `editingFile: File | null`)
    - Chapters tab: Edit button toggles chapter editing mode (state: `isEditingChapters: boolean`)
  - Edit button appears in header area (right side), changes action based on activeTab

- [x] Task 6.4 Run `yarn lint:types` to verify no type errors

- [x] Task 6.5 Verify page renders in browser
  - Navigate to `/libraries/1/books/1/files/1` (adjust IDs for your test data)
  - Confirm breadcrumbs, tabs, and basic structure appear

## Feature 7: Create FileDetailsTab Component

- [x] Task 7.1 Create FileDetailsTab.tsx
  - Location: `app/components/files/FileDetailsTab.tsx`
  - Props: `{ file: File }`
  - Display read-only metadata:
    - File type (EPUB/CBZ/M4B badge)
    - File role (main/supplement)
    - File size
    - File name
    - Page count (CBZ only)
    - Duration (M4B only)
    - Narrators (M4B only)
    - Publisher/Imprint
    - Release date
    - Identifiers
    - URL
  - Follow BookDetail's metadata section styling (label: value rows)

- [x] Task 7.2 Wire up FileEditDialog for edit action
  - When Edit button clicked on Details tab:
    - Open FileEditDialog with current file
    - FileEditDialog already handles all file metadata editing
  - State in FileDetail.tsx: `const [editingFile, setEditingFile] = useState<File | null>(null)`

- [x] Task 7.3 Run `yarn lint:types` to verify no type errors

- [x] Task 7.4 Verify Details tab renders correctly in browser
  - Check all metadata fields display properly for each file type

## Feature 8: Create FileChaptersTab Component (View Mode)

- [x] Task 8.1 Create FileChaptersTab.tsx shell
  - Location: `app/components/files/FileChaptersTab.tsx`
  - Props: `{ file: File, isEditing: boolean, onEditingChange: (editing: boolean) => void }`
  - Fetch chapters using useFileChapters(file.id)
  - Show loading spinner while fetching
  - Show empty state when no chapters exist

- [x] Task 8.2 Implement view mode chapter list
  - Map over chapters array recursively (handle children)
  - Display differs by file type:
    - EPUB: Indented hierarchy with expand/collapse, no position
    - CBZ: "Page N" position, small thumbnail (~60px) of start page
    - M4B: "HH:MM:SS.mmm" timestamp format
  - Use existing `/books/files/:id/page/:pageNum` endpoint for CBZ thumbnails

- [x] Task 8.3 Add empty state
  - Message: "No chapters"
  - "Add Chapter" button: shown for CBZ and M4B, hidden for EPUB files
  - Button triggers edit mode with one new chapter

- [x] Task 8.4 Add CBZ uncovered pages warning
  - If file is CBZ and first chapter's start_page > 0:
    - Show highlighted row at top: "Pages 0-N not in any chapter"
    - Muted hint: "Click to add chapter"
    - Clicking enters edit mode with new chapter at page 0 (start_page=0, title="")

- [x] Task 8.5 Run `yarn lint:types` to verify no type errors

- [x] Task 8.6 Verify view mode renders correctly for each file type

## Feature 9: Create ChapterRow Component

- [x] Task 9.1 Create ChapterRow.tsx
  - Location: `app/components/files/ChapterRow.tsx`
  - Props:
    ```typescript
    {
      chapter: Chapter,
      fileType: FileType,
      isEditing: boolean,
      depth: number,
      // Edit mode callbacks (only needed when isEditing=true)
      onTitleChange?: (title: string) => void,
      onStartPageChange?: (page: number) => void,
      onStartTimestampChange?: (ms: number) => void,
      onDelete?: () => void,
      onValidationChange?: (chapterId: number, hasError: boolean) => void,
      // M4B playback
      playingChapterId?: number | null,
      onPlay?: (chapterId: number) => void,
      onStop?: () => void,
      // CBZ
      fileId?: number,
      pageCount?: number,
      // M4B
      maxDurationMs?: number,
    }
    ```
  - View mode: Display title, position (format based on fileType), thumbnail/play button
  - Indentation: `pl-${depth * 4}` or similar for hierarchy
  - ChapterRow handles rendering its own children recursively when has children array

- [x] Task 9.2 Add EPUB-specific view rendering
  - Show expand/collapse chevron if has children
  - State for expanded: `const [expanded, setExpanded] = useState(true)` (per-session only, not persisted)
  - Recursively render children when expanded:
    ```tsx
    {expanded && chapter.children?.map(child => (
      <ChapterRow key={child.id} chapter={child} depth={depth + 1} fileType={fileType} ... />
    ))}
    ```
  - No position column needed

- [x] Task 9.3 Add CBZ-specific view rendering
  - Show "Page N" in position column
  - Show small thumbnail (~60px) using CBZPageThumbnail component

- [x] Task 9.4 Add M4B-specific view rendering
  - Show "HH:MM:SS.mmm" timestamp in position column
  - Show play button (implement in later feature)

- [x] Task 9.5 Run `yarn lint:types` to verify no type errors

## Feature 10: Create CBZPageThumbnail Component

- [x] Task 10.1 Create CBZPageThumbnail.tsx
  - Location: `app/components/files/CBZPageThumbnail.tsx`
  - Props: `{ fileId: number, page: number, size?: number, onClick?: () => void }`
  - Default size: 60px
  - Image src: `/api/books/files/${fileId}/page/${page}`
  - Add loading state (grey placeholder while loading)
  - Add error state (show page number text if image fails)
  - cursor-pointer if onClick provided

- [x] Task 10.2 Run `yarn lint:types` to verify no type errors

## Feature 11: Implement EPUB Chapter Editing

- [x] Task 11.1 Add edit mode to ChapterRow for EPUB
  - Title: inline text input (editable)
  - Delete button (trash icon)
  - No add/reorder capabilities

- [x] Task 11.2 Implement delete with confirmation
  - If chapter has children: "Delete [title] and its N chapters?"
  - Use Dialog component for confirmation
  - On confirm: remove chapter and all descendants from editedChapters

- [x] Task 11.3 Add edit state management in FileChaptersTab
  - On entering edit mode: `setEditedChapters(chaptersToInputArray(chapters))`
  - Helper function `chaptersToInputArray(chapters: Chapter[]): ChapterInput[]`
    - Recursively converts Chapter tree to ChapterInput array
    - Preserves nested structure (children array)
    - Strips id, created_at, updated_at, file_id, parent_id (API regenerates these)
  - Track changes in editedChapters state
  - Save button: call useUpdateFileChapters mutation with editedChapters
  - Cancel button: exit edit mode, discard changes

- [x] Task 11.4 Implement chaptersToInputArray helper
  - Location: add to `app/components/files/chapterUtils.ts`
  - Input: Chapter[] (nested tree)
  - Output: ChapterInput[] (nested tree, same structure but different type)
  - Recursive: for each chapter, create ChapterInput with { title, start_page?, start_timestamp_ms?, href?, children: recurse(children) }
  - Note: sort_order is NOT included in ChapterInput - the API derives it from array order

- [x] Task 11.5 Run `yarn lint:types` to verify no type errors

- [x] Task 11.6 Test EPUB editing in browser
  - Edit a title, verify it updates locally
  - Delete a chapter with children, verify confirmation appears
  - Save changes, verify they persist

## Feature 12: Implement CBZ Chapter Editing

- [x] Task 12.1 Add edit mode to ChapterRow for CBZ
  - Title: inline text input
  - Start page: number input with prev/next buttons (±1)
    - Validation: start_page cannot be < 0 or >= pageCount
    - Clamp value when using ±1 buttons
    - Show error state (red border) if manually entered value is out of range
  - Small thumbnail showing selected page (clickable)
  - Delete button (immediate delete, no confirmation - CBZ chapters have no children)

- [x] Task 12.2a Create CBZPagePicker dialog shell
  - Location: `app/components/files/CBZPagePicker.tsx`
  - Props: `{ fileId: number, pageCount: number, currentPage: number | null, onSelect: (page: number) => void, open: boolean, onOpenChange }`
  - Use Dialog component wrapper with title "Select Page"
  - Grid layout for thumbnails (use CSS grid, 4-5 columns)
  - State: `visibleRange: { start: number, end: number }`

- [x] Task 12.2b Implement initial page loading
  - Calculate initial range: current page ±10 (or pages 0-9 if no current page)
  - Render CBZPageThumbnail for each page in range
  - Clamp range to valid bounds (0 to pageCount-1)

- [x] Task 12.2c Add Load previous/next 10 buttons
  - "Load previous 10" button at top
    - Hidden when visibleRange.start <= 0
    - onClick: expand range start by 10 (clamped to 0)
  - "Load next 10" button at bottom
    - Hidden when visibleRange.end >= pageCount - 1
    - onClick: expand range end by 10 (clamped to pageCount - 1)

- [x] Task 12.2d Implement selection and click-to-close
  - Current selection highlighted with ring-2 ring-primary
  - Track selected page in local state
  - Clicking thumbnail: call onSelect(page), then onOpenChange(false)

- [x] Task 12.3 Implement auto-reorder on blur
  - When page input loses focus, sort editedChapters array by start_page
  - Array position determines chapter order (API derives sort_order from array index)

- [x] Task 12.4 Implement Add Chapter
  - Button appears at bottom of chapter list in edit mode
  - Defaults:
    - Start page: last chapter's start_page + 1 (or 0 if no chapters)
    - Title: pattern detection from last chapter title
  - Pattern detection rules:
    - "Chapter 3" → "Chapter 4"
    - "Ch. 3" → "Ch. 4"
    - "Chapter 3: The Title" → "Chapter 4"
    - "3: The Title" → "4"
    - No pattern → empty string

- [x] Task 12.5 Implement title pattern detection helper
  - Location: `app/components/files/chapterUtils.ts` (add to file created in 11.4)
  - Function: `getNextChapterTitle(previousTitle: string): string`
  - Input: previous chapter title (string)
  - Output: suggested next title (string)
  - Regex patterns to detect numbered chapter formats:
    - `/^(Chapter\s+)(\d+)(.*)$/i` → increment number, keep prefix, drop suffix
    - `/^(Ch\.\s*)(\d+)(.*)$/i` → increment number, keep prefix, drop suffix
    - `/^(\d+)(.*)$/` → increment number only, drop suffix
  - Return empty string if no pattern matches

- [x] Task 12.6 Write unit tests for title pattern detection
  - Location: `app/components/files/chapterUtils.test.ts`
  - Test cases:
    - "Chapter 3" → "Chapter 4"
    - "Ch. 3" → "Ch. 4"
    - "Chapter 3: The Title" → "Chapter 4"
    - "3: The Title" → "4"
    - "Introduction" → "" (no pattern)
    - "" → "" (empty input)
  - Run with `yarn test` (may need to add vitest config if not present)

- [x] Task 12.7 Run `yarn lint:types` to verify no type errors

- [x] Task 12.8 Test CBZ editing in browser
  - Change page numbers, verify reorder on blur
  - Use page picker, verify selection works
  - Add new chapter, verify defaults
  - Delete chapter, verify immediate removal (no confirmation)

## Feature 13: Implement M4B Chapter Editing

- [x] Task 13.1 Add edit mode to ChapterRow for M4B
  - Title: inline text input
  - Timestamp: text input with HH:MM:SS.mmm format
  - −/+ buttons to adjust by 1 second
  - Play button (toggle to stop while playing)
  - Delete button (immediate delete, no confirmation - M4B chapters have no children)

- [x] Task 13.2a Create timestamp formatting/parsing helpers
  - Location: add to `app/components/files/chapterUtils.ts`
  - `formatTimestampMs(ms: number): string` - Convert ms to "HH:MM:SS.mmm" format
  - `parseTimestampMs(str: string): number | null` - Parse "HH:MM:SS.mmm" to ms, return null if invalid
  - Handle edge cases: negative input, malformed strings

- [x] Task 13.2b Write unit tests for timestamp helpers
  - Location: add to `app/components/files/chapterUtils.test.ts`
  - Test formatTimestampMs: 0 → "00:00:00.000", 3661001 → "01:01:01.001"
  - Test parseTimestampMs: "01:30:00.500" → 5400500, "invalid" → null, "" → null

- [x] Task 13.2c Implement timestamp input in ChapterRow
  - Use text input with formatTimestampMs for initial display value
  - Track local input value in state (for typing)
  - On blur: parse with parseTimestampMs
    - If valid and within bounds: call onStartTimestampChange(ms), clear error
    - If invalid: show red border, set hasError flag

- [x] Task 13.2d Add timestamp validation error state
  - ChapterRow tracks `hasTimestampError: boolean` in local state
  - Validation: parseTimestampMs returns null OR value > file.audiobook_duration_seconds * 1000
  - Show red border (border-red-500) when hasTimestampError is true
  - Pass validation state up to FileChaptersTab via callback `onValidationChange?: (chapterId: number, hasError: boolean) => void`
  - FileChaptersTab aggregates all chapter validation states to compute hasValidationErrors

- [x] Task 13.3 Implement −/+ buttons
  - − button: subtract 1000ms from start_timestamp_ms
  - + button: add 1000ms to start_timestamp_ms
  - Clamp to 0 minimum, duration maximum
  - Update editedChapters state

- [x] Task 13.4 Implement auto-reorder on blur
  - When timestamp input loses focus, sort editedChapters array by start_timestamp_ms
  - Array position determines chapter order (API derives sort_order from array index)

- [x] Task 13.5 Implement Add Chapter for M4B
  - Button appears at bottom of chapter list in edit mode
  - Defaults:
    - Start timestamp: last chapter's start_timestamp_ms + 1000 (or 0 if no chapters)
    - Title: pattern detection from last chapter title (use getNextChapterTitle helper from Task 12.5)
  - Same pattern detection rules as CBZ

- [x] Task 13.6 Run `yarn lint:types` to verify no type errors

## Feature 14: Implement M4B Audio Preview

- [x] Task 14.1 Add audio element to FileChaptersTab
  - Hidden audio element: `<audio ref={audioRef} />`
  - State: `const [playingChapterId, setPlayingChapterId] = useState<number | null>(null)`
  - Audio src: `/api/books/files/${file.id}/stream`

- [x] Task 14.2 Implement play/stop functionality
  - When play clicked on chapter row:
    - If another chapter playing: stop it first
    - Set audio.currentTime to chapter's timestamp in seconds
    - Play audio
    - Set playingChapterId to this chapter's ID
  - Auto-stop after ~10 seconds:
    - Use setTimeout to pause after 10 seconds
    - Clear timeout if manually stopped or different chapter played
  - When stop clicked:
    - Pause audio
    - Set playingChapterId to null

- [x] Task 14.3 Update ChapterRow to show play/stop state
  - Show play icon (Play) when not playing
  - Show stop icon (Square) when this chapter is playing
  - Pass playingChapterId and onPlay/onStop callbacks as props

- [x] Task 14.4 Run `yarn lint:types` to verify no type errors

- [x] Task 14.5 Test M4B editing and playback in browser
  - Adjust timestamps, verify reorder
  - Play a chapter, verify audio plays from correct timestamp
  - Verify auto-stop after 10 seconds
  - Play different chapter, verify previous stops

## Feature 15: Save/Cancel Flow

- [x] Task 15.1 Implement Save button
  - Disabled while mutation is pending OR while any chapter has validation errors
  - Track validation state: `hasValidationErrors: boolean` in FileChaptersTab
  - Update hasValidationErrors when any chapter has:
    - Invalid timestamp (parseTimestampMs returns null)
    - Timestamp exceeding max duration
    - Page number < 0 or >= pageCount
  - Show loading spinner while saving
  - Call useUpdateFileChapters.mutate(editedChapters)
  - On success: exit edit mode, show success toast "Chapters saved"
  - On error: show error toast with API error message (use ShishoAPIError.message), stay in edit mode

- [x] Task 15.2 Implement Cancel button
  - Exit edit mode
  - Discard editedChapters (reset to original chapters on next edit entry)
  - No confirmation needed (data isn't lost permanently)

- [x] Task 15.3 Add unsaved changes warning
  - If user tries to navigate away while editing with changes:
    - Could use beforeunload event
    - Or just let them lose changes (simpler, matches spec)
  - Decision: Per spec, Cancel just discards - no warning needed

- [x] Task 15.4 Run `yarn lint:types` to verify no type errors

## Feature 16: Final Integration and Testing

- [x] Task 16.1 Run full lint check
  - Run `make check` to verify all tests pass and no lint errors

- [x] Task 16.2 Manual testing checklist
  - Navigate to file from book detail page
  - Details tab: all metadata displays correctly
  - Details tab: Edit opens FileEditDialog, changes save
  - EPUB Chapters: view hierarchy, edit titles, delete chapters
  - CBZ Chapters: view thumbnails, edit pages, use page picker, add/delete
  - M4B Chapters: view timestamps, edit, adjust ±1s, play preview, add/delete
  - Empty states work correctly
  - Uncovered pages warning appears for CBZ when applicable
  - Breadcrumbs navigate correctly

- [x] Task 16.3 Fix any issues found during testing
