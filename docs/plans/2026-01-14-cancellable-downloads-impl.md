# Cancellable Downloads Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add cancel buttons to downloads so users can abort long-running file generation (especially KePub conversions) without waiting.

**Architecture:** Frontend adds AbortController to fetch requests and shows X buttons during downloads. Clicking cancel aborts the browser's fetch request and clears the loading UI.

**Limitation:** Due to Go's HTTP/1.1 server not detecting client disconnects during CPU-bound operations, the backend will continue generating the file even after cancellation. The generated file gets cached, so subsequent download attempts will be instant. This is an acceptable tradeoff - the work isn't wasted, just the UI responds immediately to user cancellation.

**Tech Stack:** React/TypeScript (frontend), Go with Echo (backend), AbortController (browser API)

---

## Task 1: Frontend - Add Cancel Button UI

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Add X icon import and AbortController ref**

Add `X` to the lucide-react imports and add a `useRef` for the AbortController:

```typescript
import { ArrowLeft, Download, Edit, Loader2, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
```

After the `coverError` state declaration (around line 138), add:

```typescript
const downloadAbortController = useRef<AbortController | null>(null);
```

**Step 2: Run `yarn lint:types` to verify no type errors**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint:types`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Add AbortController ref for download cancellation

Add useRef to track AbortController for in-progress downloads. This will
be used to abort fetch requests when user clicks cancel.
EOF
)"
```

---

## Task 2: Frontend - Wire Up AbortController to Downloads

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Modify handleDownloadWithEndpoint to use AbortController**

Replace the existing `handleDownloadWithEndpoint` function (lines 144-184) with:

```typescript
const handleDownloadWithEndpoint = async (
  fileId: number,
  endpoint: string,
) => {
  setDownloadError(null);
  setDownloadingFileId(fileId);

  // Create abort controller for this download
  const abortController = new AbortController();
  downloadAbortController.current = abortController;

  try {
    // Use HEAD request to trigger generation and check for errors
    // This avoids loading the entire file into browser memory
    const headResponse = await fetch(endpoint, {
      method: "HEAD",
      signal: abortController.signal,
    });

    if (!headResponse.ok) {
      // HEAD failed - make a GET request to get the error message (small JSON response)
      const errorResponse = await fetch(endpoint, {
        signal: abortController.signal,
      });
      const contentType = errorResponse.headers.get("content-type");
      if (contentType && contentType.includes("application/json")) {
        const error = await errorResponse.json();
        setDownloadError({
          fileId,
          message: error.message || "Failed to generate file",
        });
      } else {
        setDownloadError({
          fileId,
          message: "Failed to download file",
        });
      }
      return;
    }

    // HEAD succeeded - file is ready, trigger streaming download
    window.location.href = endpoint;
    toast.success("Download started");
  } catch (error) {
    // Don't show error dialog for user-initiated cancellation
    if (error instanceof DOMException && error.name === "AbortError") {
      return;
    }
    console.error("Download error:", error);
    toast.error("Failed to download file");
  } finally {
    downloadAbortController.current = null;
    setDownloadingFileId(null);
  }
};
```

**Step 2: Add cancel handler after handleDownloadOriginal (around line 216)**

```typescript
const handleCancelDownload = () => {
  downloadAbortController.current?.abort();
  downloadAbortController.current = null;
  setDownloadingFileId(null);
};
```

**Step 3: Run `yarn lint:types` to verify no type errors**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint:types`
Expected: No errors

**Step 4: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Wire AbortController to download fetch requests

- Pass AbortController signal to HEAD and error-fetch requests
- Add handleCancelDownload function to abort in-progress downloads
- Silently handle AbortError (user-initiated cancellation)
EOF
)"
```

---

## Task 3: Frontend - Add Cancel Button to UI

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Update the download button rendering to show cancel button**

Find the section that renders the download button (around lines 519-534). Replace the `<Button>` that shows `Loader2` with a conditional that shows an X cancel button when downloading:

Replace this block:
```typescript
<Button
  disabled={downloadingFileId === file.id}
  onClick={() =>
    handleDownload(file.id, file.file_type)
  }
  size="sm"
  title="Download"
  variant="ghost"
>
  {downloadingFileId === file.id ? (
    <Loader2 className="h-3 w-3 animate-spin" />
  ) : (
    <Download className="h-3 w-3" />
  )}
</Button>
```

With:
```typescript
{downloadingFileId === file.id ? (
  <div className="flex items-center gap-1">
    <Loader2 className="h-3 w-3 animate-spin" />
    <Button
      onClick={handleCancelDownload}
      size="sm"
      title="Cancel download"
      variant="ghost"
      className="h-6 w-6 p-0"
    >
      <X className="h-3 w-3" />
    </Button>
  </div>
) : (
  <Button
    onClick={() =>
      handleDownload(file.id, file.file_type)
    }
    size="sm"
    title="Download"
    variant="ghost"
  >
    <Download className="h-3 w-3" />
  </Button>
)}
```

**Step 2: Run `yarn lint` to check for lint errors**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint`
Expected: No errors (or only pre-existing warnings)

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Add cancel button UI during downloads

Show X button next to spinner when download is in progress. Clicking
cancels the fetch request and clears the downloading state.
EOF
)"
```

---

## Task 4: Frontend - Add Cancel to DownloadFormatPopover

**Files:**
- Modify: `app/components/library/DownloadFormatPopover.tsx`

**Step 1: Add onCancel prop to DownloadFormatPopover**

Update the interface and component to accept an optional `onCancel` callback:

```typescript
import { Download, Loader2, X } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

interface DownloadFormatPopoverProps {
  onDownloadOriginal: () => void;
  onDownloadKepub: () => void;
  onCancel?: () => void;
  isLoading?: boolean;
  disabled?: boolean;
}

const DownloadFormatPopover = ({
  onDownloadOriginal,
  onDownloadKepub,
  onCancel,
  isLoading = false,
  disabled = false,
}: DownloadFormatPopoverProps) => {
  const [open, setOpen] = useState(false);

  const handleOriginal = () => {
    setOpen(false);
    onDownloadOriginal();
  };

  const handleKepub = () => {
    setOpen(false);
    onDownloadKepub();
  };

  // When loading, show spinner + cancel button instead of popover trigger
  if (isLoading) {
    return (
      <div className="flex items-center gap-1">
        <Loader2 className="h-3 w-3 animate-spin" />
        {onCancel && (
          <Button
            onClick={onCancel}
            size="sm"
            title="Cancel download"
            variant="ghost"
            className="h-6 w-6 p-0"
          >
            <X className="h-3 w-3" />
          </Button>
        )}
      </div>
    );
  }

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          disabled={disabled}
          size="sm"
          title="Download"
          variant="ghost"
        >
          <Download className="h-3 w-3" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-48 p-2">
        <div className="flex flex-col gap-1">
          <p className="text-xs font-medium text-muted-foreground px-2 py-1">
            Download format
          </p>
          <Button
            className="justify-start"
            onClick={handleOriginal}
            size="sm"
            variant="ghost"
          >
            Original
          </Button>
          <Button
            className="justify-start"
            onClick={handleKepub}
            size="sm"
            variant="ghost"
          >
            KePub (Kobo)
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
};

export default DownloadFormatPopover;
```

**Step 2: Run `yarn lint:types` to verify no type errors**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint:types`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/library/DownloadFormatPopover.tsx
git commit -m "$(cat <<'EOF'
[Feature] Add cancel support to DownloadFormatPopover

- Add optional onCancel prop
- Show spinner + X cancel button when isLoading is true
- Render inline instead of popover during loading state
EOF
)"
```

---

## Task 5: Frontend - Wire Cancel to DownloadFormatPopover in BookDetail

**Files:**
- Modify: `app/components/pages/BookDetail.tsx`

**Step 1: Update DownloadFormatPopover usage to include onCancel**

Find the DownloadFormatPopover usage (around lines 505-517) and add the `onCancel` prop:

```typescript
<DownloadFormatPopover
  disabled={downloadingFileId === file.id}
  isLoading={downloadingFileId === file.id}
  onCancel={handleCancelDownload}
  onDownloadKepub={() =>
    handleDownloadKepub(file.id)
  }
  onDownloadOriginal={() =>
    handleDownloadWithEndpoint(
      file.id,
      `/api/books/files/${file.id}/download`,
    )
  }
/>
```

**Step 2: Run `yarn lint` to check for errors**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint`
Expected: No errors

**Step 3: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Wire cancel handler to DownloadFormatPopover

Pass handleCancelDownload to DownloadFormatPopover so users can cancel
KePub conversions and other downloads from the format selection UI.
EOF
)"
```

---

## Task 6: Backend - Understanding Context Cancellation Limitations

**Files:**
- Read: `pkg/books/handlers.go`
- Read: `pkg/downloadcache/cache.go`

**Step 1: Context is passed through correctly**

The code properly passes context through the call chain:

1. `handlers.go`: `ctx := c.Request().Context()` - gets the request context
2. `handlers.go`: `h.downloadCache.GetOrGenerate(ctx, ...)` - passes ctx
3. `cache.go`: `generator.Generate(ctx, ...)` - passes context to generator

**Step 2: Generators check for cancellation**

- `epub.go`: checks `ctx.Done()` in the file processing loop
- `cbz.go`: checks `ctx.Done()` in the file processing loop
- `m4b.go`: checks `ctx.Err()` before expensive operations
- `kepub/cbz.go`: checks `ctx.Done()` in worker goroutines

**Step 3: Known limitation - HTTP/1.1 doesn't cancel context on disconnect**

While the code structure is correct, Go's HTTP/1.1 server does NOT automatically cancel the request context when a client disconnects during CPU-bound operations. The context only gets cancelled when:
- The handler returns
- The server is shut down
- For HTTP/2: when the client sends RST_STREAM

Since file generation is CPU-bound (no network I/O), the server doesn't detect client disconnect until generation completes and it tries to send the response.

**Accepted behavior:** Frontend cancellation stops the UI immediately. Backend continues generating and caches the result (so next download is instant). This is an acceptable tradeoff for simplicity.

---

## Task 7: Manual Testing

**Step 1: Start the development server**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && make start`
Expected: Server starts successfully

**Step 2: Test download cancellation**

1. Navigate to a book detail page with a CBZ or EPUB file
2. Click the download button (or format selector for KePub)
3. While spinner is showing, click the X cancel button
4. Verify: Spinner disappears, no error shown, no partial download

**Step 3: Test that normal downloads still work**

1. Click download and let it complete
2. Verify: File downloads successfully with toast "Download started"

**Step 4: Test DownloadFormatPopover cancel**

1. On a library with "ask" download preference, click the download button
2. Select "KePub (Kobo)" option
3. While loading, click the X cancel button
4. Verify: Loading state clears, popover stays closed

---

## Task 8: Final Verification and Commit

**Step 1: Run full lint check**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && yarn lint`
Expected: No errors

**Step 2: Run Go checks**

Run: `cd /Users/robinjoseph/.worktrees/shisho/cancel-download && make check`
Expected: All checks pass

**Step 3: Update the design doc to note implementation is complete**

Add a note at the top of `docs/plans/2026-01-14-cancellable-downloads-design.md`:

```markdown
> **Status:** Implemented - see commit history on `task/cancel-download` branch
```

**Step 4: Commit the design doc update**

```bash
git add docs/plans/2026-01-14-cancellable-downloads-design.md
git commit -m "$(cat <<'EOF'
[Docs] Mark cancellable downloads design as implemented
EOF
)"
```
