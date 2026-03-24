# Interactive Identify Apply

## Summary

Replace the current identify workflow's one-click apply with an interactive review screen where users compare existing book metadata against new plugin data field-by-field, choosing which values to keep. Also remove the plugin `enrich()` hook entirely — `search()` results must return complete metadata.

## User Flow

The identify dialog becomes a 2-step flow within the same modal:

### Step 1 — Search

- File selector dropdown at top of the dialog (only shown for multi-file books)
  - Persists across both steps
  - Selected file influences which result the user picks (e.g. audible vs goodreads)
  - File-level metadata (narrators, cover, identifiers) will be applied to this file
- Search input + results list (same as today)
- Clicking a result transitions to Step 2

### Step 2 — Review Changes

Single-form layout pre-filled with smart-merged values. Each field shows:

1. **Label** + **change badge** (right-aligned)
2. **Current reference bar** — compact bar showing the book's current value
3. **Editable input** — pre-filled with the smart-merged value

#### Field Types

| Field | Input Type | Notes |
|-------|-----------|-------|
| Title | Text input | |
| Subtitle | Text input | |
| Authors | Tag input (multi-select) | |
| Narrators | Tag input (multi-select) | File-level |
| Series | Text input + number input | Displayed side-by-side |
| Genres | Tag input (multi-select) | |
| Tags | Tag input (multi-select) | |
| Description | Textarea | Current ref collapsible with "Show more" toggle |
| Publisher | Text input | File-level |
| Imprint | Text input | File-level |
| Release Date | Text/date input | |
| URL | Text input | |
| Cover | Side-by-side thumbnails | Click to select current or new. File-level |
| Identifiers | Tag input (key:value pairs) | File-level |

#### Smart Merge Defaults

| Scenario | Default Value | Badge | "Use current" |
|----------|--------------|-------|---------------|
| Book empty, result populated | New value | New (green) | No (nothing to revert to) |
| Both populated, values match | Keep as-is | Unchanged (grey) | No |
| Both populated, values differ | Plugin value | Changed (purple) | Yes |
| Book populated, result empty | Existing value | Unchanged (grey) | No |

#### Disabled Fields

When a field is in the search result's `disabled_fields` array (computed from plugin field settings):
- Input is non-interactive (disabled state)
- Tooltip: "Field disabled for this plugin"
- The field's current value is preserved — no change applied

#### Cover Selection

- Two clickable thumbnails side-by-side: "Keep current" and "Use new"
- Selected option highlighted with accent border + checkmark
- If cover is a disabled field, only the current cover is shown with a disabled tooltip

#### Footer Actions

- **Back to results** — returns to Step 1, preserving search query and results
- **Cancel** — closes the dialog entirely
- **Apply Changes** — submits the form

## Backend Changes

### Remove `enrich()` Hook

- Delete the `enrichMetadata` handler and `POST /plugins/enrich` route from `pkg/plugins/handler.go`
- Remove `RunMetadataEnrich` from `pkg/plugins/hooks.go`
- Remove `EnrichmentResult` type
- Remove `providerData` from `SearchResult` type — no longer needed
- Update plugin manifest validation to not require/expect `enrich` function

### Search Result Contract Change

`search()` must return complete metadata in each result. The `SearchResult` type already has all metadata fields — plugins just need to populate them fully instead of deferring to `enrich()`.

### New Apply Endpoint

`POST /plugins/apply`

**Request payload:**

```go
type applyPayload struct {
    BookID  int                        `json:"book_id" validate:"required"`
    FileID  *int                       `json:"file_id"`
    Fields  map[string]any             `json:"fields" validate:"required"`
    // Fields contains the final merged values keyed by field name:
    // - "title": string
    // - "subtitle": string
    // - "authors": []string
    // - "narrators": []string
    // - "series": string
    // - "series_number": float64
    // - "genres": []string
    // - "tags": []string
    // - "description": string
    // - "publisher": string
    // - "imprint": string
    // - "release_date": string
    // - "url": string
    // - "cover_url": string (server downloads it)
    // - "identifiers": []{ type: string, value: string }
    PluginScope string                 `json:"plugin_scope" validate:"required"`
    PluginID    string                 `json:"plugin_id" validate:"required"`
}
```

**Handler logic:**
1. Look up book with all relations
2. Resolve target file (from `FileID` or first file)
3. Convert `Fields` map into a `ParsedMetadata` struct
4. If `cover_url` present, download it (validate domain against plugin's httpAccess allowlist)
5. Apply metadata to book/file using existing `applyEnrichment` logic
6. Return updated book

### Update Scan Worker

In `pkg/worker/scan_unified.go`, `runMetadataEnrichers`:
- Remove Phase 2 (enrich call)
- Take the first search result's metadata directly
- Apply enabled fields using existing `filterMetadataFields` logic
- Rest of the merge/priority logic stays the same

## Frontend Changes

### IdentifyBookDialog

- Add `step` state: `"search" | "review"`
- Move file selector dropdown to top of dialog (visible in both steps)
- Remove the old apply mutation (`usePluginEnrich`)
- Clicking a search result sets `selectedResult` and transitions to `step = "review"`

### New Component: IdentifyReviewForm

**Props:** `selectedResult: PluginSearchResult`, `book: Book`, `selectedFileId?: number`

**State:** Form state initialized from smart merge of `book` + `selectedResult`

**Rendering:**
- Iterates over all metadata fields
- For each field: renders label, change badge, current reference bar, and editable input
- Disabled fields rendered as non-interactive with tooltip
- Cover field uses side-by-side thumbnail picker

**Submit:**
- Collects form state into the `fields` map
- Calls `POST /plugins/apply`
- Invalidates book queries on success
- Closes dialog

### Remove usePluginEnrich

- Delete the `usePluginEnrich` hook from `app/hooks/queries/plugins.ts`
- Delete the `enrichMetadata` API function from `app/libraries/api.ts`

### No New Dependencies

Tag inputs, text inputs, textarea, and cover display all use existing UI components and patterns.
