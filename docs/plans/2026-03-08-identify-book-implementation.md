# Identify Book Dialog — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an "Identify book" dialog to the book detail page that searches enricher plugins and applies a selected result's metadata to the book.

**Architecture:** New `IdentifyBookDialog` component triggered from the book detail dropdown menu. Uses existing `usePluginSearch` and `usePluginEnrich` mutation hooks. Dialog contains a search input (pre-filled with book title), a scrollable result card list, and an apply button.

**Tech Stack:** React, TypeScript, TailwindCSS, Tanstack Query, Radix Dialog, Lucide icons

---

### Task 1: Create the IdentifyBookDialog component

**Files:**
- Create: `app/components/library/IdentifyBookDialog.tsx`

**Step 1: Create the dialog component**

Create `app/components/library/IdentifyBookDialog.tsx` with the full implementation:

```tsx
import { Loader2, Search } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  type PluginSearchResult,
  usePluginEnrich,
  usePluginSearch,
} from "@/hooks/queries/plugins";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";

interface IdentifyBookDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  book: Book;
}

export function IdentifyBookDialog({
  open,
  onOpenChange,
  book,
}: IdentifyBookDialogProps) {
  const [query, setQuery] = useState("");
  const [selectedResult, setSelectedResult] =
    useState<PluginSearchResult | null>(null);
  const searchMutation = usePluginSearch();
  const enrichMutation = usePluginEnrich();
  const inputRef = useRef<HTMLInputElement>(null);
  const hasSearchedRef = useRef(false);

  // Pre-fill query and auto-search when dialog opens
  useEffect(() => {
    if (open) {
      setQuery(book.title);
      setSelectedResult(null);
      hasSearchedRef.current = false;
    }
  }, [open, book.title]);

  // Auto-search after query is set from dialog open
  useEffect(() => {
    if (open && query && !hasSearchedRef.current) {
      hasSearchedRef.current = true;
      searchMutation.mutate({ query, bookId: book.id });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, query]);

  const handleSearch = () => {
    if (!query.trim()) return;
    setSelectedResult(null);
    searchMutation.mutate({ query: query.trim(), bookId: book.id });
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      handleSearch();
    }
  };

  const handleApply = () => {
    if (!selectedResult) return;
    enrichMutation.mutate(
      {
        pluginScope: selectedResult.plugin_scope,
        pluginId: selectedResult.plugin_id,
        bookId: book.id,
        providerData: selectedResult.provider_data,
      },
      {
        onSuccess: () => {
          toast.success("Book metadata updated successfully");
          onOpenChange(false);
        },
        onError: (error) => {
          toast.error(error.message || "Failed to apply metadata");
        },
      },
    );
  };

  const results = searchMutation.data?.results ?? [];

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Identify Book</DialogTitle>
          <DialogDescription>
            Search for this book across metadata providers and apply the correct
            match.
          </DialogDescription>
        </DialogHeader>

        {/* Search bar */}
        <div className="flex gap-2">
          <Input
            className="flex-1"
            onKeyDown={handleKeyDown}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by title, author, ISBN..."
            ref={inputRef}
            value={query}
          />
          <Button
            disabled={searchMutation.isPending || !query.trim()}
            onClick={handleSearch}
            variant="outline"
          >
            {searchMutation.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Search className="h-4 w-4" />
            )}
          </Button>
        </div>

        {/* Results */}
        <div className="min-h-[200px] max-h-[60vh] overflow-y-auto">
          {searchMutation.isPending && (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {searchMutation.isSuccess && results.length === 0 && (
            <div className="text-center py-12 text-muted-foreground">
              No results found. Try a different search query.
            </div>
          )}

          {searchMutation.isSuccess && results.length > 0 && (
            <div className="space-y-2">
              {results.map((result, index) => (
                <button
                  className={cn(
                    "w-full text-left rounded-lg border p-3 cursor-pointer transition-colors",
                    "hover:bg-muted/50",
                    selectedResult === result
                      ? "ring-2 ring-primary border-primary"
                      : "border-border",
                  )}
                  key={`${result.plugin_scope}-${result.plugin_id}-${index}`}
                  onClick={() => setSelectedResult(result)}
                  type="button"
                >
                  <div className="flex gap-3">
                    {/* Cover thumbnail */}
                    {result.image_url ? (
                      <img
                        alt=""
                        className="w-16 h-24 object-cover rounded shrink-0 bg-muted"
                        src={result.image_url}
                      />
                    ) : (
                      <div className="w-16 h-24 rounded shrink-0 bg-muted flex items-center justify-center text-muted-foreground text-xs">
                        No cover
                      </div>
                    )}

                    {/* Details */}
                    <div className="flex-1 min-w-0 space-y-1">
                      <div className="flex items-start justify-between gap-2">
                        <p className="font-medium leading-tight">
                          {result.title}
                        </p>
                        <Badge
                          className="shrink-0 text-xs"
                          variant="outline"
                        >
                          {result.plugin_id}
                        </Badge>
                      </div>

                      {result.authors && result.authors.length > 0 && (
                        <p className="text-sm text-muted-foreground">
                          {result.authors.join(", ")}
                        </p>
                      )}

                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        {result.release_date && (
                          <span>{result.release_date}</span>
                        )}
                        {result.publisher && (
                          <>
                            {result.release_date && (
                              <span className="text-muted-foreground/50">
                                ·
                              </span>
                            )}
                            <span>{result.publisher}</span>
                          </>
                        )}
                      </div>

                      {result.identifiers && result.identifiers.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-1">
                          {result.identifiers.map((id) => (
                            <Badge
                              className="text-xs"
                              key={`${id.type}-${id.value}`}
                              variant="secondary"
                            >
                              {id.type}: {id.value}
                            </Badge>
                          ))}
                        </div>
                      )}

                      {result.description && (
                        <p className="text-xs text-muted-foreground line-clamp-2 mt-1">
                          {result.description}
                        </p>
                      )}
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}

          {searchMutation.isError && (
            <div className="text-center py-12 text-destructive">
              Search failed. Please try again.
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={!selectedResult || enrichMutation.isPending}
            onClick={handleApply}
          >
            {enrichMutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/identify && yarn lint:types`
Expected: PASS (no type errors)

**Step 3: Commit**

```
git add app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Add IdentifyBookDialog component"
```

---

### Task 2: Wire the dialog into BookDetail

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add import, state, and menu item**

In `BookDetail.tsx`:

1. Add import at top (with other dialog imports):
```tsx
import { IdentifyBookDialog } from "@/components/library/IdentifyBookDialog";
```

2. Add `Search` to the lucide-react import (it's already importing from lucide-react at line 1).

3. Add state variable after the existing dialog states (around line 764):
```tsx
const [showIdentifyDialog, setShowIdentifyDialog] = useState(false);
```

4. Add dropdown menu item in the overflow menu. Insert after the "Refresh all metadata" item (after line 1191) and before the merge separator (line 1192):
```tsx
<DropdownMenuItem
  onClick={() => setShowIdentifyDialog(true)}
>
  <Search className="h-4 w-4 mr-2" />
  Identify book
</DropdownMenuItem>
```

5. Add the dialog component in the dialog rendering section (after the `BookEditDialog` at line 1494):
```tsx
<IdentifyBookDialog
  book={book}
  onOpenChange={setShowIdentifyDialog}
  open={showIdentifyDialog}
/>
```

**Step 2: Verify it compiles**

Run: `cd /Users/robinjoseph/.worktrees/shisho/identify && yarn lint:types`
Expected: PASS

**Step 3: Run full lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/identify && yarn lint`
Expected: PASS

**Step 4: Commit**

```
git add app/components/pages/BookDetail.tsx
git commit -m "[Frontend] Wire IdentifyBookDialog into book detail page"
```

---

### Task 3: Manual testing

**Step 1: Start the dev server**

Run: `make start` (if not already running)

**Step 2: Test the flow**

1. Navigate to any book detail page
2. Click the ⋮ overflow menu
3. Click "Identify book"
4. Verify dialog opens with title pre-filled and search auto-fires
5. If enricher plugins are installed, verify results appear as cards
6. If no enricher plugins, verify "No results found" message
7. Select a result and verify highlight ring appears
8. Click Apply and verify book metadata updates

**Step 3: Run make check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/identify && make check`
Expected: All checks pass
