# File UI Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Redesign the file display on the book detail page to be cleaner and more scannable, with name prominent, filename muted underneath, and secondary metadata collapsible.

**Architecture:** Refactor the file rendering in BookDetail.tsx to use a two-line layout (name + filename), add expand/collapse state for secondary metadata, and remove the colored left border for a cleaner look.

**Tech Stack:** React 19, TypeScript, TailwindCSS, lucide-react icons (ChevronRight, ChevronDown)

---

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

---

### Task 1: Add Expand/Collapse State

**Files:**
- Modify: `app/components/pages/BookDetail.tsx:131-145`

**Step 1: Add the expanded state**

Add a new state variable to track which files have their details expanded. Add this after line 143 (after the `coverError` state):

```typescript
const [expandedFileIds, setExpandedFileIds] = useState<Set<number>>(new Set());
```

**Step 2: Add helper function to toggle expansion**

Add this helper function after the state declarations (around line 145):

```typescript
const toggleFileExpanded = (fileId: number) => {
  setExpandedFileIds((prev) => {
    const next = new Set(prev);
    if (next.has(fileId)) {
      next.delete(fileId);
    } else {
      next.add(fileId);
    }
    return next;
  });
};
```

**Step 3: Add helper function to check if file has expandable metadata**

Add this helper function after the toggle function:

```typescript
const hasExpandableMetadata = (file: File): boolean => {
  return !!(
    file.publisher ||
    file.imprint ||
    file.release_date ||
    file.url ||
    (file.identifiers && file.identifiers.length > 0)
  );
};
```

**Step 4: Verify the build passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && yarn lint:types`
Expected: No type errors

**Step 5: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Add expand/collapse state for file metadata

Add state management for tracking which files have their secondary
metadata expanded. Includes helper functions for toggling state
and checking if a file has expandable metadata.
EOF
)"
```

---

### Task 2: Create FileRow Component Structure

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add ChevronRight and ChevronDown imports**

Update the lucide-react import at line 1 to include the chevron icons:

```typescript
import { ArrowLeft, ChevronDown, ChevronRight, Download, Edit, Loader2, X } from "lucide-react";
```

**Step 2: Create the FileRow component**

Add a new component before the `BookDetail` component (around line 130). This component will handle the rendering of a single file with all its display logic:

```typescript
interface FileRowProps {
  file: File;
  libraryId: string;
  libraryDownloadPreference: string | undefined;
  isExpanded: boolean;
  hasExpandableMetadata: boolean;
  onToggleExpand: () => void;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadKepub: () => void;
  onDownloadOriginal: () => void;
  onDownloadWithEndpoint: (endpoint: string) => void;
  onCancelDownload: () => void;
  onEdit: () => void;
  isSupplement?: boolean;
}

const FileRow = ({
  file,
  libraryId,
  libraryDownloadPreference,
  isExpanded,
  hasExpandableMetadata,
  onToggleExpand,
  isDownloading,
  onDownload,
  onDownloadKepub,
  onDownloadOriginal,
  onDownloadWithEndpoint,
  onCancelDownload,
  onEdit,
  isSupplement = false,
}: FileRowProps) => {
  const showChevron = hasExpandableMetadata && !isSupplement;

  return (
    <div className="py-2 space-y-1">
      {/* Primary row */}
      <div className="flex items-center gap-2">
        {/* Chevron for expand/collapse */}
        {showChevron ? (
          <button
            className="p-0.5 hover:bg-muted rounded"
            onClick={onToggleExpand}
            type="button"
          >
            {isExpanded ? (
              <ChevronDown className="h-4 w-4 text-muted-foreground" />
            ) : (
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            )}
          </button>
        ) : (
          <div className="w-5" /> // Spacer for alignment when no chevron
        )}

        {/* File type badge */}
        <Badge
          className="uppercase text-xs flex-shrink-0"
          variant={isSupplement ? "outline" : "secondary"}
        >
          {file.file_type}
        </Badge>

        {/* Name and filename */}
        <div className="flex flex-col min-w-0 flex-1">
          <span
            className={`truncate ${isSupplement ? "text-sm" : "text-sm font-medium"}`}
            title={file.name || getFilename(file.filepath)}
          >
            {file.name || getFilename(file.filepath)}
          </span>
        </div>

        {/* Stats and actions */}
        <div className="flex items-center gap-3 text-xs text-muted-foreground flex-shrink-0">
          {/* M4B stats */}
          {file.audiobook_duration_seconds && (
            <span>{formatDuration(file.audiobook_duration_seconds)}</span>
          )}
          {file.audiobook_bitrate_bps && (
            <span>
              {Math.round(file.audiobook_bitrate_bps / 1000)} kbps
            </span>
          )}
          {/* CBZ stats */}
          {file.page_count && (
            <span>{file.page_count} pages</span>
          )}
          {/* File size - always shown */}
          <span>{formatFileSize(file.filesize_bytes)}</span>

          {/* Download button/popover */}
          {isSupplement ? (
            <Button
              onClick={onDownloadOriginal}
              size="sm"
              title="Download"
              variant="ghost"
            >
              <Download className="h-3 w-3" />
            </Button>
          ) : libraryDownloadPreference === DownloadFormatAsk &&
            supportsKepub(file.file_type) ? (
            <DownloadFormatPopover
              disabled={isDownloading}
              isLoading={isDownloading}
              onCancel={onCancelDownload}
              onDownloadKepub={onDownloadKepub}
              onDownloadOriginal={() =>
                onDownloadWithEndpoint(`/api/books/files/${file.id}/download`)
              }
            />
          ) : isDownloading ? (
            <div className="flex items-center gap-1">
              <Loader2 className="h-3 w-3 animate-spin" />
              <Button
                className="h-6 w-6 p-0"
                onClick={onCancelDownload}
                size="sm"
                title="Cancel download"
                variant="ghost"
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          ) : (
            <Button
              onClick={onDownload}
              size="sm"
              title="Download"
              variant="ghost"
            >
              <Download className="h-3 w-3" />
            </Button>
          )}

          {/* Edit button */}
          <Button
            onClick={onEdit}
            size="sm"
            title="Edit"
            variant="ghost"
          >
            <Edit className="h-3 w-3" />
          </Button>
        </div>
      </div>

      {/* Filename row - always visible, indented past chevron and badge */}
      <div className="ml-5 pl-2">
        <span
          className="text-xs text-muted-foreground truncate block"
          title={file.filepath}
        >
          {getFilename(file.filepath)}
        </span>
      </div>

      {/* Narrators row - M4B only, always visible when present */}
      {file.narrators && file.narrators.length > 0 && (
        <div className="ml-5 pl-2 flex items-center gap-1 flex-wrap">
          <span className="text-xs text-muted-foreground">Narrated by</span>
          {file.narrators.map((narrator, index) => (
            <span key={narrator.id} className="text-xs">
              <Link
                className="hover:underline"
                to={`/libraries/${libraryId}/people/${narrator.person_id}`}
              >
                {narrator.person?.name ?? "Unknown"}
              </Link>
              {index < file.narrators!.length - 1 ? "," : ""}
            </span>
          ))}
        </div>
      )}

      {/* Expandable details section */}
      {isExpanded && hasExpandableMetadata && (
        <div className="ml-5 pl-2 mt-2 bg-muted/50 rounded-md p-3 text-xs space-y-2">
          {/* Publisher, Imprint, Released, URL */}
          <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
            {file.publisher && (
              <>
                <span className="text-muted-foreground">Publisher</span>
                <span>{file.publisher.name}</span>
              </>
            )}
            {file.imprint && (
              <>
                <span className="text-muted-foreground">Imprint</span>
                <span>{file.imprint.name}</span>
              </>
            )}
            {file.release_date && (
              <>
                <span className="text-muted-foreground">Released</span>
                <span>{formatDate(file.release_date)}</span>
              </>
            )}
            {file.url && (
              <>
                <span className="text-muted-foreground">URL</span>
                <a
                  className="text-primary hover:underline truncate"
                  href={file.url}
                  rel="noopener noreferrer"
                  target="_blank"
                  title={file.url}
                >
                  {file.url.length > 60 ? file.url.substring(0, 60) + "..." : file.url}
                </a>
              </>
            )}
          </div>

          {/* Identifiers */}
          {file.identifiers && file.identifiers.length > 0 && (
            <div className="pt-2 border-t border-border/50">
              <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
                {file.identifiers.map((id, idx) => (
                  <React.Fragment key={idx}>
                    <span className="text-muted-foreground">
                      {formatIdentifierType(id.type)}
                    </span>
                    <span className="font-mono select-all">{id.value}</span>
                  </React.Fragment>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
```

**Step 3: Add React import for Fragment**

Update the React import at line 2:

```typescript
import React, { useEffect, useRef, useState } from "react";
```

**Step 4: Verify the build passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && yarn lint:types`
Expected: No type errors

**Step 5: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Add FileRow component for file display redesign

Create FileRow component that renders files with:
- Prominent name display with muted filename underneath
- Disclosure chevron for expandable metadata
- Two-column key-value layout for expanded details
- Consistent layout for M4B, EPUB, and CBZ files
EOF
)"
```

---

### Task 3: Update Main Files Section

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Replace the main files rendering**

Replace the main files section (approximately lines 501-683) with the new FileRow component:

Find this section:
```tsx
{/* Files */}
<div>
  <h3 className="font-semibold mb-3">
    Files ({mainFiles.length})
  </h3>
  <div className="space-y-3">
    {mainFiles.map((file) => (
      <div
        className="border-l-4 border-l-primary dark:border-l-violet-300 pl-4 py-2 space-y-2"
        key={file.id}
      >
        ...
      </div>
    ))}
  </div>
</div>
```

Replace with:
```tsx
{/* Files */}
<div>
  <h3 className="font-semibold mb-3">
    Files ({mainFiles.length})
  </h3>
  <div className="space-y-2">
    {mainFiles.map((file) => (
      <FileRow
        key={file.id}
        file={file}
        libraryId={libraryId!}
        libraryDownloadPreference={libraryQuery.data?.download_format_preference}
        isExpanded={expandedFileIds.has(file.id)}
        hasExpandableMetadata={hasExpandableMetadata(file)}
        onToggleExpand={() => toggleFileExpanded(file.id)}
        isDownloading={downloadingFileId === file.id}
        onDownload={() => handleDownload(file.id, file.file_type)}
        onDownloadKepub={() => handleDownloadKepub(file.id)}
        onDownloadOriginal={() => handleDownloadOriginal(file.id)}
        onDownloadWithEndpoint={(endpoint) =>
          handleDownloadWithEndpoint(file.id, endpoint)
        }
        onCancelDownload={handleCancelDownload}
        onEdit={() => setEditingFile(file)}
      />
    ))}
  </div>
</div>
```

**Step 2: Verify the build passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && yarn lint:types`
Expected: No type errors

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Wire up FileRow component for main files

Replace the inline file rendering with FileRow component.
Removes colored left border in favor of cleaner layout.
EOF
)"
```

---

### Task 4: Update Supplements Section

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Replace the supplements rendering**

Find the supplements section (approximately lines 686-748):
```tsx
{/* Supplements */}
{supplements.length > 0 && (
  <>
    <Separator />
    <div>
      <h3 className="font-semibold mb-3">
        Supplements ({supplements.length})
      </h3>
      <div className="space-y-2">
        {supplements.map((file) => (
          <div
            className="border-l-4 border-l-muted-foreground/30 pl-4 py-2"
            key={file.id}
          >
            ...
          </div>
        ))}
      </div>
    </div>
  </>
)}
```

Replace with:
```tsx
{/* Supplements */}
{supplements.length > 0 && (
  <>
    <Separator />
    <div>
      <h3 className="font-semibold mb-3">
        Supplements ({supplements.length})
      </h3>
      <div className="space-y-2">
        {supplements.map((file) => (
          <FileRow
            key={file.id}
            file={file}
            libraryId={libraryId!}
            libraryDownloadPreference={libraryQuery.data?.download_format_preference}
            isExpanded={expandedFileIds.has(file.id)}
            hasExpandableMetadata={hasExpandableMetadata(file)}
            onToggleExpand={() => toggleFileExpanded(file.id)}
            isDownloading={downloadingFileId === file.id}
            onDownload={() => handleDownload(file.id, file.file_type)}
            onDownloadKepub={() => handleDownloadKepub(file.id)}
            onDownloadOriginal={() => handleDownloadOriginal(file.id)}
            onDownloadWithEndpoint={(endpoint) =>
              handleDownloadWithEndpoint(file.id, endpoint)
            }
            onCancelDownload={handleCancelDownload}
            onEdit={() => setEditingFile(file)}
            isSupplement
          />
        ))}
      </div>
    </div>
  </>
)}
```

**Step 2: Verify the build passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && yarn lint:types`
Expected: No type errors

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Wire up FileRow component for supplements

Use FileRow with isSupplement flag for supplement files.
Maintains consistent layout while using muted styling.
EOF
)"
```

---

### Task 5: Clean Up Unused Code

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Remove any dead code**

After refactoring, verify that the old inline file rendering code has been completely removed and no unused imports remain.

**Step 2: Run full lint check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && yarn lint`
Expected: No errors

**Step 3: Visual verification**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && make start`

Navigate to a book detail page and verify:
1. Files show name prominently with filename underneath in muted text
2. Chevron appears only when secondary metadata exists
3. Clicking chevron expands/collapses the details section
4. M4B files show duration, bitrate, and narrators
5. CBZ files show page count
6. Supplements have similar layout but muted styling
7. Download and edit buttons work correctly

**Step 4: Commit if any cleanup was needed**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[UI] Clean up file UI redesign implementation

Remove any dead code from the file display refactoring.
EOF
)"
```

---

### Task 6: Run Full Validation

**Step 1: Run all checks**

Run: `cd /Users/robinjoseph/.worktrees/shisho/refine-file-ui && make check`
Expected: All tests pass, no lint errors

**Step 2: Fix any issues discovered**

If any issues are found, fix them and commit with appropriate message.

---

## Summary of Changes

1. **State management**: Added `expandedFileIds` Set to track expanded files
2. **New component**: Created `FileRow` component for consistent file rendering
3. **Layout changes**:
   - Name prominent, filename muted underneath
   - Removed colored left border
   - Added disclosure chevron for expandable metadata
   - Two-column key-value layout for expanded details
4. **Consistent behavior**: Same layout for main files and supplements (with muted styling for supplements)
