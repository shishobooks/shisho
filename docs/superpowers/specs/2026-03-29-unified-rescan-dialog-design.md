# Unified Rescan Dialog

**Date:** 2026-03-29
**Status:** Approved

## Problem

When auto-identify during scan misidentifies a book via enricher plugins, there is no way to clear that metadata and revert to just the baked-in file metadata. The existing "Refresh all metadata" resync re-runs enricher plugins, so it re-applies the same incorrect identification.

Additionally, the book and file context menus each have two separate rescan items ("Scan for new metadata" and "Refresh all metadata") that would be clearer as a single entry point with explicit mode selection.

## Solution

Replace the two rescan menu items with a single "Rescan book" / "Rescan file" item that opens a dialog presenting three mutually exclusive scan modes.

## Scan Modes

| Mode | Name | ForceRefresh | SkipPlugins | Description |
|------|------|-------------|-------------|-------------|
| `scan` | Scan for new metadata | false | false | Pick up new metadata without overwriting manual edits. Use when file metadata has been updated externally. |
| `refresh` | Refresh all metadata | true | false | Re-scan as if this were the first time. Use after installing or updating plugins to re-enrich metadata. |
| `reset` | Reset to file metadata | true | true | Skip plugins and use only metadata embedded in the source file(s). Use when plugin enrichment is matching incorrectly. |

Default selection: **Scan for new metadata** (safest option).

All three modes re-extract covers from files. Mode `refresh` allows plugins to potentially upgrade covers afterward; mode `reset` stops at the file-embedded cover.

## API Changes

### Payload

Both `POST /books/:id/resync` and `POST /books/files/:id/resync` accept:

```json
{ "mode": "scan" | "refresh" | "reset" }
```

Replaces the current `{ "refresh": boolean }` payload.

**Backwards compatibility:** If `mode` is empty, fall back to checking the `refresh` boolean for any existing callers.

### Response

No changes — responses remain the same (updated entity or deletion status).

## Backend Changes

### ScanOptions

Add `SkipPlugins bool` field to `ScanOptions` in `pkg/worker/scan_unified.go`:

```go
type ScanOptions struct {
    FilePath     string
    FileID       int
    BookID       int
    LibraryID    int
    ForceRefresh bool
    SkipPlugins  bool   // New: skip enricher plugins during scan
    JobLog       *joblogs.JobLogger
}
```

### ResyncPayload

Update `ResyncPayload` in `pkg/books/validators.go`:

```go
type ResyncPayload struct {
    Mode    string `json:"mode"`
    Refresh bool   `json:"refresh"` // Deprecated: kept for backwards compatibility
}
```

### Resync Handlers

Update `resyncFile` and `resyncBook` handlers in `pkg/books/handlers.go` to map mode to options:

```go
forceRefresh, skipPlugins := resolveScanMode(params.Mode, params.Refresh)
result, err := h.scanner.Scan(ctx, ScanOptions{
    FileID:       id,
    ForceRefresh: forceRefresh,
    SkipPlugins:  skipPlugins,
})
```

Mode resolution logic:
- `"scan"` → `ForceRefresh=false, SkipPlugins=false`
- `"refresh"` → `ForceRefresh=true, SkipPlugins=false`
- `"reset"` → `ForceRefresh=true, SkipPlugins=true`
- empty → fall back to `Refresh` boolean (backwards compat)

### Scan Worker

In `pkg/worker/scan_unified.go`, skip `runMetadataEnrichers` when `SkipPlugins` is true. The `SkipPlugins` flag must be threaded through to all code paths that call `runMetadataEnrichers`:

- `scanFileByID` (FileID mode, single file resync)
- `scanInternal` (FilePath mode, batch scan — though batch scans won't use this flag)
- `scanBook` (BookID mode, which delegates to `scanFileByID` per file)

## Frontend Changes

### New RescanDialog Component

`app/components/library/RescanDialog.tsx` — replaces `ResyncConfirmDialog.tsx`.

Props:
```typescript
interface RescanDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: "book" | "file";
  entityName: string;
  onConfirm: (mode: RescanMode) => void;
  isPending: boolean;
}

type RescanMode = "scan" | "refresh" | "reset";
```

Dialog contains:
- Title: "Rescan book" or "Rescan file"
- Subtitle: entity name
- Three radio options with names and descriptions (mode 1 selected by default)
- Cancel and Rescan buttons

### ResyncPayload Type

Update `app/hooks/queries/resync.ts`:

```typescript
export type RescanMode = "scan" | "refresh" | "reset";

export interface ResyncPayload {
  mode: RescanMode;
}
```

### Menu Changes

**Book menu** (`BookItem.tsx`):
- Remove: "Scan for new metadata", "Refresh all metadata"
- Add: "Rescan book" (opens RescanDialog)
- Keep: "Identify book", separator, "Delete"

**File menu** (`BookDetail.tsx`):
- Remove: "Scan for new metadata", "Refresh all metadata"
- Add: "Rescan file" (opens RescanDialog)
- Keep: Edit, Set as primary, Move to another book, Delete file

### Deleted Files

- `app/components/library/ResyncConfirmDialog.tsx` — no longer needed

## Menu Structure (After)

### Book Menu
```
Rescan book
Identify book
─────────────
Delete
```

### File Menu
```
Edit
─────────────
Rescan file
Set as primary    (conditional)
Move to another book
─────────────
Delete file
```

## What's NOT Changing

- `useResyncBook` and `useResyncFile` hooks — same structure, just payload shape changes
- `IdentifyBookDialog` — untouched
- Batch library scans — unaffected (they don't use resync endpoints)
- Sidecar handling — unchanged
