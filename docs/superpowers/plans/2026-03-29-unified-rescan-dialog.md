# Unified Rescan Dialog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two separate rescan menu items with a single "Rescan book/file" item that opens a dialog with three mutually exclusive scan modes, including a new "Reset to file metadata" mode that skips plugins.

**Architecture:** Add a `SkipPlugins` flag to `ScanOptions` that bypasses `runMetadataEnrichers`. Change the API payload from `{ refresh: bool }` to `{ mode: string }`. Replace `ResyncConfirmDialog` with a new `RescanDialog` using radio buttons. Update all menu sites (BookItem, BookDetail book menu, BookDetail file menus) to use the new single menu item.

**Tech Stack:** Go (Echo handlers, scanner worker), React (Radix RadioGroup, shadcn Dialog), TypeScript, TanStack Query

**Spec:** `docs/superpowers/specs/2026-03-29-unified-rescan-dialog-design.md`

---

### Task 1: Add SkipPlugins to ScanOptions and wire through scanner

**Files:**
- Modify: `pkg/worker/scan_unified.go:120-134` (ScanOptions struct)
- Modify: `pkg/worker/scan_unified.go:456` (scanFileByID enricher call)
- Modify: `pkg/worker/scan_unified.go:551-555` (scanBook delegation)
- Modify: `pkg/worker/scan_unified.go:2222` (scanFileCreateNew enricher call)

- [ ] **Step 1: Add SkipPlugins field to ScanOptions**

In `pkg/worker/scan_unified.go`, add `SkipPlugins` to the struct:

```go
type ScanOptions struct {
	// Entry points (mutually exclusive - exactly one must be set)
	FilePath string // Batch scan: discover/create by path
	FileID   int    // Single file resync: file already in DB
	BookID   int    // Book resync: scan all files in book

	// Context (required for FilePath mode)
	LibraryID int

	// Behavior
	ForceRefresh bool // Bypass priority checks, overwrite all metadata
	SkipPlugins  bool // Skip enricher plugins, use only file-embedded metadata

	// Logging (optional, for batch scan job context)
	JobLog *joblogs.JobLogger
}
```

- [ ] **Step 2: Guard enricher call in scanFileByID**

In `pkg/worker/scan_unified.go` around line 455-456, wrap the enricher call:

```go
	// Run metadata enrichers after parsing
	if !opts.SkipPlugins {
		metadata = w.runMetadataEnrichers(ctx, metadata, file, book, file.LibraryID, opts.JobLog)
	}
```

- [ ] **Step 3: Pass SkipPlugins through scanBook delegation**

In `pkg/worker/scan_unified.go` around line 551, add `SkipPlugins` to the ScanOptions passed to `scanFileByID`:

```go
		fileResult, err := w.scanFileByID(ctx, ScanOptions{
			FileID:       file.ID,
			ForceRefresh: opts.ForceRefresh,
			SkipPlugins:  opts.SkipPlugins,
			JobLog:       opts.JobLog,
		}, cache)
```

- [ ] **Step 4: Guard enricher call in scanFileCreateNew (batch path)**

In `pkg/worker/scan_unified.go` around line 2221-2222, wrap the enricher call:

```go
	// Run metadata enrichers after parsing
	if !opts.SkipPlugins {
		metadata = w.runMetadataEnrichers(ctx, metadata, file, book, opts.LibraryID, opts.JobLog)
	}
```

- [ ] **Step 5: Build and verify compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 6: Run Go tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && mise test`
Expected: All tests pass. No behavioral change yet since nothing sets `SkipPlugins=true`.

- [ ] **Step 7: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Backend] Add SkipPlugins flag to ScanOptions"
```

---

### Task 2: Update ResyncPayload and handlers to accept mode

**Files:**
- Modify: `pkg/books/validators.go:58-61` (ResyncPayload struct)
- Modify: `pkg/books/handlers.go:1653-1702` (resyncFile handler)
- Modify: `pkg/books/handlers.go:1704-1752` (resyncBook handler)

- [ ] **Step 1: Update ResyncPayload**

In `pkg/books/validators.go`, replace the struct at lines 58-61:

```go
// ResyncPayload contains the request parameters for resync operations.
type ResyncPayload struct {
	Mode    string `json:"mode"`
	Refresh bool   `json:"refresh"` // Deprecated: kept for backwards compatibility
}

// resolveScanMode converts a ResyncPayload into ForceRefresh and SkipPlugins flags.
// Supports three modes: "scan" (default), "refresh", and "reset".
// Falls back to the deprecated Refresh boolean if Mode is empty.
func (p ResyncPayload) resolveScanMode() (forceRefresh, skipPlugins bool) {
	switch p.Mode {
	case "refresh":
		return true, false
	case "reset":
		return true, true
	case "scan", "":
		// For empty mode, check deprecated Refresh field for backwards compatibility
		return p.Refresh, false
	default:
		return false, false
	}
}
```

- [ ] **Step 2: Update resyncFile handler**

In `pkg/books/handlers.go`, update the `resyncFile` handler. Replace the scanner call at lines 1684-1687:

```go
	// Perform resync
	forceRefresh, skipPlugins := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		FileID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
	})
```

- [ ] **Step 3: Update resyncBook handler**

In `pkg/books/handlers.go`, update the `resyncBook` handler. Replace the scanner call at lines 1735-1738:

```go
	// Perform resync
	forceRefresh, skipPlugins := params.resolveScanMode()
	result, err := h.scanner.Scan(ctx, ScanOptions{
		BookID:       id,
		ForceRefresh: forceRefresh,
		SkipPlugins:  skipPlugins,
	})
```

- [ ] **Step 4: Build and verify**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && go build ./...`
Expected: Clean build.

- [ ] **Step 5: Run Go tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && mise test`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/books/validators.go pkg/books/handlers.go
git commit -m "[Backend] Update resync payload to accept mode parameter"
```

---

### Task 3: Write unit tests for resolveScanMode

**Files:**
- Create: `pkg/books/validators_test.go`

- [ ] **Step 1: Write tests for all mode mappings**

Create `pkg/books/validators_test.go`:

```go
package books

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveScanMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		payload          ResyncPayload
		wantForceRefresh bool
		wantSkipPlugins  bool
	}{
		{
			name:             "scan mode",
			payload:          ResyncPayload{Mode: "scan"},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
		{
			name:             "refresh mode",
			payload:          ResyncPayload{Mode: "refresh"},
			wantForceRefresh: true,
			wantSkipPlugins:  false,
		},
		{
			name:             "reset mode",
			payload:          ResyncPayload{Mode: "reset"},
			wantForceRefresh: true,
			wantSkipPlugins:  true,
		},
		{
			name:             "empty mode without refresh",
			payload:          ResyncPayload{},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
		{
			name:             "empty mode with refresh true (backwards compat)",
			payload:          ResyncPayload{Refresh: true},
			wantForceRefresh: true,
			wantSkipPlugins:  false,
		},
		{
			name:             "unknown mode defaults to scan",
			payload:          ResyncPayload{Mode: "unknown"},
			wantForceRefresh: false,
			wantSkipPlugins:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			forceRefresh, skipPlugins := tt.payload.resolveScanMode()
			assert.Equal(t, tt.wantForceRefresh, forceRefresh, "forceRefresh")
			assert.Equal(t, tt.wantSkipPlugins, skipPlugins, "skipPlugins")
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && go test ./pkg/books/ -run TestResolveScanMode -v`
Expected: All 6 subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/books/validators_test.go
git commit -m "[Test] Add unit tests for resolveScanMode"
```

---

### Task 4: Update frontend ResyncPayload type and hooks

**Files:**
- Modify: `app/hooks/queries/resync.ts`

- [ ] **Step 1: Update the payload type and hooks**

Replace the full content of `app/hooks/queries/resync.ts`:

```typescript
import { QueryKey } from "./books";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { API } from "@/libraries/api";
import { Book, File } from "@/types";

export type RescanMode = "scan" | "refresh" | "reset";

export interface ResyncPayload {
  mode: RescanMode;
}

export interface ResyncFileResult {
  file_deleted?: boolean;
  book_deleted?: boolean;
}

export interface ResyncBookResult {
  book_deleted?: boolean;
}

export const useResyncFile = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      fileId,
      payload,
    }: {
      fileId: number;
      payload: ResyncPayload;
    }): Promise<File | ResyncFileResult> => {
      return API.request<File | ResyncFileResult>(
        "POST",
        `/books/files/${fileId}/resync`,
        payload,
      );
    },
    onSuccess: (result) => {
      // If not deleted, invalidate queries to refresh data
      if (!("file_deleted" in result && result.file_deleted)) {
        const file = result as File;
        queryClient.invalidateQueries({
          queryKey: [QueryKey.RetrieveBook, String(file.book_id)],
        });
      }
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
    },
  });
};

export const useResyncBook = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      bookId,
      payload,
    }: {
      bookId: number;
      payload: ResyncPayload;
    }): Promise<Book | ResyncBookResult> => {
      return API.request<Book | ResyncBookResult>(
        "POST",
        `/books/${bookId}/resync`,
        payload,
      );
    },
    onSuccess: (_result, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveBook, String(bookId)],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
    },
  });
};
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && pnpm lint:types`
Expected: Type errors from callers that still pass `{ refresh: boolean }` — this is expected and will be fixed in subsequent tasks.

- [ ] **Step 3: Commit**

```bash
git add app/hooks/queries/resync.ts
git commit -m "[Frontend] Update ResyncPayload to use mode parameter"
```

---

### Task 5: Create RescanDialog component

**Files:**
- Create: `app/components/library/RescanDialog.tsx`

- [ ] **Step 1: Create the RescanDialog component**

Create `app/components/library/RescanDialog.tsx`:

```tsx
import { Loader2, RefreshCw } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { type RescanMode } from "@/hooks/queries/resync";

interface RescanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: "book" | "file";
  entityName: string;
  onConfirm: (mode: RescanMode) => void;
  isPending: boolean;
}

const modes: { value: RescanMode; label: string; description: string }[] = [
  {
    value: "scan",
    label: "Scan for new metadata",
    description:
      "Pick up new metadata without overwriting manual edits. Use when file metadata has been updated externally.",
  },
  {
    value: "refresh",
    label: "Refresh all metadata",
    description:
      "Re-scan as if this were the first time. Use after installing or updating plugins to re-enrich metadata.",
  },
  {
    value: "reset",
    label: "Reset to file metadata",
    description:
      "Skip plugins and use only metadata embedded in the source file(s). Use when plugin enrichment is matching incorrectly.",
  },
];

export function RescanDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  onConfirm,
  isPending,
}: RescanDialogProps) {
  const [selectedMode, setSelectedMode] = useState<RescanMode>("scan");

  return (
    <Dialog
      onOpenChange={(newOpen) => {
        if (!newOpen) setSelectedMode("scan");
        onOpenChange(newOpen);
      }}
      open={open}
    >
      <DialogContent className="max-w-md">
        <DialogHeader className="pr-8">
          <DialogTitle className="flex items-center gap-2">
            <RefreshCw className="h-5 w-5 shrink-0" />
            <span>Rescan {entityType}</span>
          </DialogTitle>
          <DialogDescription className="break-words">
            Choose how to rescan &ldquo;{entityName}&rdquo;
          </DialogDescription>
        </DialogHeader>

        <RadioGroup
          className="gap-3"
          onValueChange={(value) => setSelectedMode(value as RescanMode)}
          value={selectedMode}
        >
          {modes.map((mode) => (
            <Label
              className="flex items-start gap-3 rounded-lg border p-3 cursor-pointer has-[[data-state=checked]]:border-primary"
              htmlFor={`rescan-${mode.value}`}
              key={mode.value}
            >
              <RadioGroupItem
                className="mt-0.5"
                id={`rescan-${mode.value}`}
                value={mode.value}
              />
              <div className="space-y-0.5">
                <div className="text-sm font-medium leading-none">
                  {mode.label}
                </div>
                <div className="text-xs text-muted-foreground font-normal">
                  {mode.description}
                </div>
              </div>
            </Label>
          ))}
        </RadioGroup>

        <DialogFooter>
          <Button
            disabled={isPending}
            onClick={() => onOpenChange(false)}
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending}
            onClick={() => {
              onOpenChange(false);
              onConfirm(selectedMode);
            }}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Rescan
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Verify TypeScript compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && pnpm lint:types`
Expected: Still type errors from callers of old payload shape — `RescanDialog` itself should compile clean.

- [ ] **Step 3: Commit**

```bash
git add app/components/library/RescanDialog.tsx
git commit -m "[Frontend] Add RescanDialog component with three scan modes"
```

---

### Task 6: Update BookItem to use RescanDialog

**Files:**
- Modify: `app/components/library/BookItem.tsx`

- [ ] **Step 1: Update imports**

In `app/components/library/BookItem.tsx`, replace the `ResyncConfirmDialog` import with `RescanDialog` and add `RescanMode`:

Replace:
```typescript
import { ResyncConfirmDialog } from "@/components/library/ResyncConfirmDialog";
```
With:
```typescript
import { RescanDialog } from "@/components/library/RescanDialog";
```

Also add the `RescanMode` import. Replace:
```typescript
import { useDeleteBook, useResyncBook } from "@/hooks/queries/books";
```
With:
```typescript
import { useDeleteBook, useResyncBook } from "@/hooks/queries/books";
import { type RescanMode } from "@/hooks/queries/resync";
```

- [ ] **Step 2: Rename state variable**

Replace `showRefreshDialog` with `showRescanDialog`. At line 148:

Replace:
```typescript
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);
```
With:
```typescript
  const [showRescanDialog, setShowRescanDialog] = useState(false);
```

- [ ] **Step 3: Replace both handlers with a single rescan handler**

Replace the two handlers at lines 162-188 (`handleScanMetadata` and `handleRefreshMetadata`) with a single handler:

```typescript
  const handleRescan = async (mode: RescanMode) => {
    try {
      await resyncBookMutation.mutateAsync({
        bookId: book.id,
        payload: { mode },
      });
      toast.success("Book rescanned");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to rescan book",
      );
    }
  };
```

- [ ] **Step 4: Update menu items**

Replace the two rescan menu items (lines 265-275) with a single item:

Replace:
```tsx
              <DropdownMenuItem
                disabled={resyncBookMutation.isPending}
                onClick={handleScanMetadata}
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Scan for new metadata
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Refresh all metadata
              </DropdownMenuItem>
```
With:
```tsx
              <DropdownMenuItem
                disabled={resyncBookMutation.isPending}
                onClick={() => setShowRescanDialog(true)}
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Rescan book
              </DropdownMenuItem>
```

- [ ] **Step 5: Replace ResyncConfirmDialog with RescanDialog**

Replace the dialog instance at lines 385-392:

Replace:
```tsx
      <ResyncConfirmDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRefreshMetadata}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
```
With:
```tsx
      <RescanDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRescan}
        onOpenChange={setShowRescanDialog}
        open={showRescanDialog}
      />
```

- [ ] **Step 6: Remove unused imports**

The `RefreshCw` import is still used (for the menu item icon), so keep it. No other imports need removal.

- [ ] **Step 7: Verify TypeScript compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && pnpm lint:types`
Expected: BookItem.tsx compiles clean. Remaining errors only from BookDetail.tsx.

- [ ] **Step 8: Commit**

```bash
git add app/components/library/BookItem.tsx
git commit -m "[Frontend] Update BookItem to use unified RescanDialog"
```

---

### Task 7: Update BookDetail to use RescanDialog

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

This file has multiple sites to update: the FileRow component (two menus — desktop and mobile), the book-level menu, and the book-level dialog. The FileRow also needs its props simplified.

- [ ] **Step 1: Update imports**

In `app/components/pages/BookDetail.tsx`, replace the `ResyncConfirmDialog` import:

Replace:
```typescript
import { ResyncConfirmDialog } from "@/components/library/ResyncConfirmDialog";
```
With:
```typescript
import { RescanDialog } from "@/components/library/RescanDialog";
import { type RescanMode } from "@/hooks/queries/resync";
```

- [ ] **Step 2: Update FileRow props**

Replace `onScanMetadata` and `onRefreshMetadata` with a single `onRescan` prop. In the `FileRowProps` interface (lines 145-174):

Replace:
```typescript
  onScanMetadata: () => void;
  onRefreshMetadata: () => void;
```
With:
```typescript
  onRescan: () => void;
```

- [ ] **Step 3: Update FileRow destructuring**

In the `FileRow` component destructuring (lines 176-205):

Replace:
```typescript
  onScanMetadata,
  onRefreshMetadata,
```
With:
```typescript
  onRescan,
```

- [ ] **Step 4: Update FileRow desktop menu (first menu instance)**

Replace the two rescan items in the desktop menu (around lines 413-423):

Replace:
```tsx
              <DropdownMenuItem
                disabled={isResyncing}
                onClick={onScanMetadata}
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Scan for new metadata
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Refresh all metadata
              </DropdownMenuItem>
```
With:
```tsx
              <DropdownMenuItem
                disabled={isResyncing}
                onClick={onRescan}
              >
                <RefreshCw className="h-4 w-4 mr-2" />
                Rescan file
              </DropdownMenuItem>
```

- [ ] **Step 5: Update FileRow mobile menu (second menu instance)**

Replace the two rescan items in the mobile menu (around lines 577-584):

Replace:
```tsx
              <DropdownMenuItem disabled={isResyncing} onClick={onScanMetadata}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Scan for new metadata
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setShowRefreshDialog(true)}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Refresh all metadata
              </DropdownMenuItem>
```
With:
```tsx
              <DropdownMenuItem disabled={isResyncing} onClick={onRescan}>
                <RefreshCw className="h-4 w-4 mr-2" />
                Rescan file
              </DropdownMenuItem>
```

- [ ] **Step 6: Remove FileRow's showRefreshDialog state and ResyncConfirmDialog**

Remove the `showRefreshDialog` state at line 207:
```typescript
  const [showRefreshDialog, setShowRefreshDialog] = useState(false);
```

Remove the `ResyncConfirmDialog` usage at lines 720-727:
```tsx
      <ResyncConfirmDialog
        entityName={file.name || getFilename(file.filepath)}
        entityType="file"
        isPending={isResyncing}
        onConfirm={onRefreshMetadata}
        onOpenChange={setShowRefreshDialog}
        open={showRefreshDialog}
      />
```

- [ ] **Step 7: Rename BookDetail state and add rescan handlers**

In the `BookDetail` component, replace the four separate handlers and state with unified ones.

Replace state at line 745:
```typescript
  const [showBookRefreshDialog, setShowBookRefreshDialog] = useState(false);
```
With:
```typescript
  const [showBookRescanDialog, setShowBookRescanDialog] = useState(false);
  const [rescanFileId, setRescanFileId] = useState<number | null>(null);
```

Replace the four handlers at lines 924-1004 (`handleScanFileMetadata`, `handleRefreshFileMetadata`, `handleScanBookMetadata`, `handleRefreshBookMetadata`) with two:

```typescript
  const handleRescanFile = async (fileId: number, mode: RescanMode) => {
    setResyncingFileId(fileId);
    try {
      const result = await resyncFileMutation.mutateAsync({
        fileId,
        payload: { mode },
      });
      if ("file_deleted" in result && result.file_deleted) {
        toast.success("File removed (no longer exists on disk)");
      } else {
        toast.success("File rescanned");
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to rescan file",
      );
    } finally {
      setResyncingFileId(null);
    }
  };

  const handleRescanBook = async (mode: RescanMode) => {
    if (!id) return;
    try {
      const result = await resyncBookMutation.mutateAsync({
        bookId: parseInt(id),
        payload: { mode },
      });
      if ("book_deleted" in result && result.book_deleted) {
        toast.success("Book removed (no files remain)");
      } else {
        toast.success("Book rescanned");
      }
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to rescan book",
      );
    }
  };
```

- [ ] **Step 8: Update BookDetail book menu**

Replace the two rescan items in the book-level dropdown (around lines 1187-1199):

Replace:
```tsx
                    <DropdownMenuItem
                      disabled={resyncBookMutation.isPending}
                      onClick={handleScanBookMetadata}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Scan for new metadata
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      onClick={() => setShowBookRefreshDialog(true)}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Refresh all metadata
                    </DropdownMenuItem>
```
With:
```tsx
                    <DropdownMenuItem
                      disabled={resyncBookMutation.isPending}
                      onClick={() => setShowBookRescanDialog(true)}
                    >
                      <RefreshCw className="h-4 w-4 mr-2" />
                      Rescan book
                    </DropdownMenuItem>
```

- [ ] **Step 9: Update FileRow prop wiring for main files**

Replace the `onScanMetadata` and `onRefreshMetadata` props passed to `FileRow` for main files (around lines 1437-1438):

Replace:
```tsx
                    onRefreshMetadata={() => handleRefreshFileMetadata(file.id)}
                    onScanMetadata={() => handleScanFileMetadata(file.id)}
```
With:
```tsx
                    onRescan={() => setRescanFileId(file.id)}
```

- [ ] **Step 10: Update FileRow prop wiring for supplements**

Replace the props for supplement FileRows (around lines 1489-1492):

Replace:
```tsx
                        onRefreshMetadata={() =>
                          handleRefreshFileMetadata(file.id)
                        }
                        onScanMetadata={() => handleScanFileMetadata(file.id)}
```
With:
```tsx
                        onRescan={() => setRescanFileId(file.id)}
```

- [ ] **Step 11: Replace book ResyncConfirmDialog with RescanDialog**

Replace the dialog at lines 1516-1523:

Replace:
```tsx
      <ResyncConfirmDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRefreshBookMetadata}
        onOpenChange={setShowBookRefreshDialog}
        open={showBookRefreshDialog}
      />
```
With:
```tsx
      <RescanDialog
        entityName={book.title}
        entityType="book"
        isPending={resyncBookMutation.isPending}
        onConfirm={handleRescanBook}
        onOpenChange={setShowBookRescanDialog}
        open={showBookRescanDialog}
      />
```

- [ ] **Step 12: Add file RescanDialog**

Add a new `RescanDialog` for files, right after the book one:

```tsx
      <RescanDialog
        entityName={
          rescanFileId
            ? (book.files.find((f) => f.id === rescanFileId)?.name ||
              getFilename(
                book.files.find((f) => f.id === rescanFileId)?.filepath ?? "",
              ))
            : ""
        }
        entityType="file"
        isPending={resyncFileMutation.isPending}
        onConfirm={(mode) => {
          if (rescanFileId) handleRescanFile(rescanFileId, mode);
        }}
        onOpenChange={(open) => {
          if (!open) setRescanFileId(null);
        }}
        open={rescanFileId !== null}
      />
```

- [ ] **Step 13: Verify TypeScript compilation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && pnpm lint:types`
Expected: Clean — no type errors.

- [ ] **Step 14: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Update BookDetail to use unified RescanDialog"
```

---

### Task 8: Delete ResyncConfirmDialog

**Files:**
- Delete: `app/components/library/ResyncConfirmDialog.tsx`

- [ ] **Step 1: Verify no remaining imports**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && grep -r "ResyncConfirmDialog" app/`
Expected: No results (all references removed in Tasks 6 and 7).

- [ ] **Step 2: Delete the file**

```bash
rm app/components/library/ResyncConfirmDialog.tsx
```

- [ ] **Step 3: Run full validation**

Run: `cd /Users/robinjoseph/.worktrees/shisho/clear-identify && mise check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint all green).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "[Frontend] Remove deprecated ResyncConfirmDialog"
```
