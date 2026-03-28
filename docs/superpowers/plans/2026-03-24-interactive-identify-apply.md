# Interactive Identify Apply Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the identify workflow's one-click apply with an interactive review screen for field-by-field metadata selection, and remove the plugin `enrich()` hook.

**Architecture:** The identify dialog gains a second step (review form) between search and apply. A new `POST /plugins/apply` endpoint receives the user's final field selections directly. The `applyEnrichment` function is split into field-filtering (caller-specific) and DB persistence (shared helper). The `enrich()` hook and its endpoint are removed.

**Tech Stack:** Go/Echo backend, React/TypeScript frontend, Tanstack Query, TailwindCSS, shadcn/ui

**Spec:** `docs/superpowers/specs/2026-03-24-interactive-identify-apply-design.md`

---

### Task 1: Update SearchResult type and remove enrich() from Go backend

Unify `SearchResult` with `ParsedMetadata` by adding missing fields, changing `Authors` from `[]string` to `[]ParsedAuthor`, and removing `ProviderData`/`Metadata`. Remove `RunMetadataEnrich` and `EnrichmentResult`.

**Files:**
- Modify: `pkg/plugins/hooks.go:24-28` (delete `EnrichmentResult`), `pkg/plugins/hooks.go:136-157` (update `SearchResult`), `pkg/plugins/hooks.go:207-248` (delete `RunMetadataEnrich`)

- [ ] **Step 1: Update SearchResult type**

In `pkg/plugins/hooks.go`, update the `SearchResult` struct:

```go
type SearchResult struct {
	Title        string                       `json:"title"`
	Authors      []mediafile.ParsedAuthor     `json:"authors"`
	Description  string                       `json:"description"`
	ImageURL     string                       `json:"image_url"`
	ReleaseDate  string                       `json:"release_date"`
	Publisher    string                       `json:"publisher"`
	Subtitle     string                       `json:"subtitle"`
	Series       string                       `json:"series"`
	SeriesNumber *float64                     `json:"series_number,omitempty"`
	Genres       []string                     `json:"genres"`
	Tags         []string                     `json:"tags"`
	Narrators    []string                     `json:"narrators"`
	Identifiers  []mediafile.ParsedIdentifier `json:"identifiers"`
	Imprint      string                       `json:"imprint"`
	URL          string                       `json:"url"`
	CoverURL     string                       `json:"cover_url"`
	// Added by the caller, not the plugin:
	PluginScope    string   `json:"plugin_scope"`
	PluginID       string   `json:"plugin_id"`
	DisabledFields []string `json:"disabled_fields,omitempty"`
}
```

Changes from current: `Authors` is now `[]ParsedAuthor`, added `Imprint`, `URL`, `CoverURL`. Removed `ProviderData`, `Metadata`.

- [ ] **Step 2: Delete EnrichmentResult type**

Remove `EnrichmentResult` struct (lines 24-28) from `pkg/plugins/hooks.go`.

- [ ] **Step 3: Delete RunMetadataEnrich function**

Remove the entire `RunMetadataEnrich` function (lines 207-248) from `pkg/plugins/hooks.go`.

- [ ] **Step 4: Update parseSearchResponse**

Find the `parseSearchResponse` helper in `pkg/plugins/hooks.go` and update it to:
- Parse `authors` as `[]ParsedAuthor` (objects with `name`/`role`) instead of `[]string`
- Parse new fields: `imprint`, `url`, `cover_url` (alias `coverUrl` and `imageUrl`)
- Stop parsing `provider_data` and `metadata`

- [ ] **Step 5: Remove parseEnrichmentResult**

Delete the `parseEnrichmentResult` helper function from `pkg/plugins/hooks.go`.

- [ ] **Step 6: Fix compilation errors**

Run `go build ./...` and fix any references to deleted types/functions. Key locations:
- `pkg/plugins/handler.go` — `enrichMetadata` handler references `RunMetadataEnrich` and `EnrichmentResult`
- `pkg/worker/scan_unified.go` — `runMetadataEnrichers` calls `RunMetadataEnrich`
- Any test files referencing these types

Don't fix handler.go's `enrichMetadata` yet (deleted in Task 2). Focus on making non-deleted code compile.

- [ ] **Step 7: Run tests**

Run: `make test`
Expected: Tests pass (some enricher tests may need updating for new `Authors` type).

- [ ] **Step 8: Commit**

```bash
git add pkg/plugins/hooks.go
git commit -m "[Backend] Update SearchResult type and remove enrich hook"
```

---

### Task 2: Remove enrich endpoint and extract persistence helper

Delete the `enrichMetadata` handler/route, and extract `applyEnrichment`'s DB persistence logic into a shared helper that skips field-filtering.

**Files:**
- Modify: `pkg/plugins/handler.go:140-146` (delete `enrichPayload`), `pkg/plugins/handler.go:1353-1446` (delete `enrichMetadata`), `pkg/plugins/handler.go:1515-1842` (refactor `applyEnrichment`)
- Modify: `pkg/plugins/routes.go:62` (delete enrich route)

- [ ] **Step 1: Delete the enrich route**

In `pkg/plugins/routes.go`, remove line 62:
```go
g.POST("/enrich", h.enrichMetadata)
```

- [ ] **Step 2: Delete enrichPayload and enrichMetadata handler**

In `pkg/plugins/handler.go`:
- Delete the `enrichPayload` struct (lines 140-146)
- Delete the `enrichMetadata` handler function (lines 1353-1446)

- [ ] **Step 3: Extract persistMetadata from applyEnrichment**

Refactor `applyEnrichment` (lines 1515-1842) into two functions:

1. `persistMetadata` — takes `(ctx, book, targetFile, md, pluginScope, pluginID, log)` and contains all the DB persistence logic (lines 1549-1842). No `isAllowed` checks — it applies everything in `md`. Receives `pluginScope` and `pluginID` as strings instead of a `*Runtime`, and constructs `pluginSource` from them.

2. `applyEnrichment` — keeps the field-filtering logic (lines 1515-1547), builds a *filtered* copy of `ParsedMetadata` with only allowed fields populated, then calls `persistMetadata` with that filtered copy.

The signature for `persistMetadata`:

```go
func (h *handler) persistMetadata(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, pluginScope, pluginID string, log logger.Logger) error {
    pluginSource := models.PluginDataSource(pluginScope, pluginID)
    // ... all the DB persistence logic from applyEnrichment (lines 1549-1842)
}
```

The refactored `applyEnrichment` builds a filtered `ParsedMetadata` and delegates:

```go
func (h *handler) applyEnrichment(ctx context.Context, book *models.Book, targetFile *models.File, md *mediafile.ParsedMetadata, rt *Runtime, log logger.Logger) error {
    manifest := rt.Manifest()
    if manifest.Capabilities.MetadataEnricher == nil {
        return nil
    }

    declaredFields := manifest.Capabilities.MetadataEnricher.Fields
    enabledFields, err := h.service.GetEffectiveFieldSettings(ctx, book.LibraryID, rt.Scope(), rt.PluginID(), declaredFields)
    if err != nil {
        log.Warn("failed to get field settings, using all enabled", logger.Data{"error": err.Error()})
        enabledFields = make(map[string]bool, len(declaredFields))
        for _, f := range declaredFields {
            enabledFields[f] = true
        }
    }

    declared := make(map[string]bool, len(declaredFields))
    for _, f := range declaredFields {
        declared[f] = true
    }
    isAllowed := func(field string) bool {
        if !declared[field] {
            return false
        }
        if enabled, ok := enabledFields[field]; ok {
            return enabled
        }
        return true
    }

    filtered := filterParsedMetadata(md, isAllowed)
    return h.persistMetadata(ctx, book, targetFile, filtered, rt.Scope(), rt.PluginID(), log)
}
```

Add a `filterParsedMetadata` helper that copies only allowed fields:

```go
func filterParsedMetadata(md *mediafile.ParsedMetadata, isAllowed func(string) bool) *mediafile.ParsedMetadata {
    out := &mediafile.ParsedMetadata{}
    if isAllowed("title") { out.Title = md.Title }
    if isAllowed("subtitle") { out.Subtitle = md.Subtitle }
    if isAllowed("description") { out.Description = md.Description }
    if isAllowed("authors") { out.Authors = md.Authors }
    if isAllowed("series") || isAllowed("seriesNumber") {
        out.Series = md.Series
        out.SeriesNumber = md.SeriesNumber
    }
    if isAllowed("genres") { out.Genres = md.Genres }
    if isAllowed("tags") { out.Tags = md.Tags }
    if isAllowed("narrators") { out.Narrators = md.Narrators }
    if isAllowed("publisher") { out.Publisher = md.Publisher }
    if isAllowed("imprint") { out.Imprint = md.Imprint }
    if isAllowed("url") { out.URL = md.URL }
    if isAllowed("releaseDate") { out.ReleaseDate = md.ReleaseDate }
    if isAllowed("identifiers") { out.Identifiers = md.Identifiers }
    if isAllowed("cover") {
        out.CoverData = md.CoverData
        out.CoverMimeType = md.CoverMimeType
        out.CoverURL = md.CoverURL
        out.CoverPage = md.CoverPage
    }
    return out
}
```

- [ ] **Step 4: Update persistMetadata to not use isAllowed**

Make sure `persistMetadata` applies every non-empty field unconditionally — remove all `isAllowed()` checks. The `if field != "" {` guards remain (skip empty values), but no permission checks.

- [ ] **Step 5: Run tests and fix**

Run: `make test`
Expected: All existing enrichment tests should still pass since `applyEnrichment` still delegates through the same logic.

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go
git commit -m "[Backend] Remove enrich endpoint and extract persistMetadata helper"
```

---

### Task 3: Add POST /plugins/apply endpoint

Create the new apply endpoint that receives field selections from the frontend review form.

**Files:**
- Modify: `pkg/plugins/handler.go` (add `applyPayload` struct and `applyMetadata` handler)
- Modify: `pkg/plugins/routes.go` (add route)

- [ ] **Step 1: Add applyPayload struct**

In `pkg/plugins/handler.go`, add the payload struct near the other payload types:

```go
type applyPayload struct {
	BookID      int            `json:"book_id" validate:"required"`
	FileID      *int           `json:"file_id"`
	Fields      map[string]any `json:"fields" validate:"required"`
	PluginScope string         `json:"plugin_scope" validate:"required"`
	PluginID    string         `json:"plugin_id" validate:"required"`
}
```

- [ ] **Step 2: Add convertFieldsToMetadata helper**

Add a helper that manually converts the untyped `Fields` map to `*mediafile.ParsedMetadata`:

```go
func convertFieldsToMetadata(fields map[string]any) (*mediafile.ParsedMetadata, error) {
	md := &mediafile.ParsedMetadata{}

	if v, ok := fields["title"].(string); ok {
		md.Title = v
	}
	if v, ok := fields["subtitle"].(string); ok {
		md.Subtitle = v
	}
	if v, ok := fields["description"].(string); ok {
		md.Description = v
	}
	if v, ok := fields["publisher"].(string); ok {
		md.Publisher = v
	}
	if v, ok := fields["imprint"].(string); ok {
		md.Imprint = v
	}
	if v, ok := fields["url"].(string); ok {
		md.URL = v
	}
	if v, ok := fields["series"].(string); ok {
		md.Series = v
	}
	if v, ok := fields["cover_url"].(string); ok {
		md.CoverURL = v
	}

	// Series number
	if v, ok := fields["series_number"].(float64); ok {
		md.SeriesNumber = &v
	}

	// Release date
	if v, ok := fields["release_date"].(string); ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			t, err = time.Parse(time.RFC3339, v)
		}
		if err == nil {
			md.ReleaseDate = &t
		}
	}

	// Authors: []{ name: string, role: string }
	if v, ok := fields["authors"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				name, _ := m["name"].(string)
				role, _ := m["role"].(string)
				if name != "" {
					md.Authors = append(md.Authors, mediafile.ParsedAuthor{Name: name, Role: role})
				}
			}
		}
	}

	// Narrators: []string
	if v, ok := fields["narrators"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Narrators = append(md.Narrators, s)
			}
		}
	}

	// Genres: []string
	if v, ok := fields["genres"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Genres = append(md.Genres, s)
			}
		}
	}

	// Tags: []string
	if v, ok := fields["tags"].([]any); ok {
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				md.Tags = append(md.Tags, s)
			}
		}
	}

	// Identifiers: []{ type: string, value: string }
	if v, ok := fields["identifiers"].([]any); ok {
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				idType, _ := m["type"].(string)
				idValue, _ := m["value"].(string)
				if idType != "" && idValue != "" {
					md.Identifiers = append(md.Identifiers, mediafile.ParsedIdentifier{Type: idType, Value: idValue})
				}
			}
		}
	}

	return md, nil
}
```

- [ ] **Step 3: Add applyMetadata handler**

```go
func (h *handler) applyMetadata(c echo.Context) error {
	if h.enrich == nil {
		return errcodes.InternalServerError("enrichment dependencies not available")
	}

	var payload applyPayload
	if err := c.Bind(&payload); err != nil {
		return errcodes.BadRequest(err.Error())
	}

	ctx := c.Request().Context()
	log := logger.FromContext(ctx)

	// Look up plugin runtime (for httpAccess domain validation on cover download)
	rt := h.manager.GetRuntime(payload.PluginScope, payload.PluginID)
	if rt == nil {
		return errcodes.NotFound("plugin not found or not loaded")
	}

	// Look up book with all relations
	book, err := h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
	if err != nil {
		return errcodes.NotFound("book not found")
	}

	// Library access check
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return errcodes.Unauthorized("User not found in context")
	}
	if !user.HasLibraryAccess(book.LibraryID) {
		return errcodes.Forbidden("You don't have access to this library")
	}

	// Resolve target file
	var targetFile *models.File
	if payload.FileID != nil {
		for i := range book.Files {
			if book.Files[i].ID == *payload.FileID {
				targetFile = book.Files[i]
				break
			}
		}
		if targetFile == nil {
			return errcodes.NotFound("file not found in book")
		}
	} else if len(book.Files) > 0 {
		targetFile = book.Files[0]
	}

	// Convert fields map to ParsedMetadata
	md, err := convertFieldsToMetadata(payload.Fields)
	if err != nil {
		return errcodes.BadRequest("invalid field data: " + err.Error())
	}

	// Download cover if cover_url set
	if md.CoverURL != "" {
		manifest := rt.Manifest()
		var allowedDomains []string
		if manifest.Capabilities.HTTPAccess != nil {
			allowedDomains = manifest.Capabilities.HTTPAccess.Domains
		}
		DownloadCoverFromURL(ctx, md, allowedDomains, log)
	}

	// Persist metadata (no field filtering — user already selected fields)
	if err := h.persistMetadata(ctx, book, targetFile, md, payload.PluginScope, payload.PluginID, log); err != nil {
		return errcodes.InternalServerError("failed to apply metadata: " + err.Error())
	}

	// Reload and return updated book
	updatedBook, err := h.enrich.bookStore.RetrieveBook(ctx, payload.BookID)
	if err != nil {
		return errcodes.InternalServerError("failed to reload book: " + err.Error())
	}

	return c.JSON(200, updatedBook)
}
```

- [ ] **Step 4: Register the route**

In `pkg/plugins/routes.go`, add after the search route:

```go
g.POST("/search", h.searchMetadata)
g.POST("/apply", h.applyMetadata)
```

- [ ] **Step 5: Run tests and fix**

Run: `make test`

- [ ] **Step 6: Commit**

```bash
git add pkg/plugins/handler.go pkg/plugins/routes.go
git commit -m "[Backend] Add POST /plugins/apply endpoint"
```

---

### Task 4: Update scan worker to skip enrich phase

Remove Phase 2 (enrich call) from `runMetadataEnrichers` and use search result metadata directly.

**Files:**
- Modify: `pkg/worker/scan_unified.go:2587-2729` (`runMetadataEnrichers`)

- [ ] **Step 1: Read current scan worker enricher code**

Read `pkg/worker/scan_unified.go` lines 2587-2729 to understand the exact phase structure.

- [ ] **Step 2: Remove Phase 2 (enrich call)**

In `runMetadataEnrichers`, after Phase 1 (search) finds a result:
- Remove the call to `RunMetadataEnrich`
- Instead, convert the `SearchResult` directly to `*mediafile.ParsedMetadata`
- Apply field filtering using `filterMetadataFields` (already exists)
- Handle cover URL download (already exists)
- Continue with the merge logic in Phase 3

The conversion from `SearchResult` to `ParsedMetadata` needs a helper:

```go
func searchResultToMetadata(sr *SearchResult) *mediafile.ParsedMetadata {
	md := &mediafile.ParsedMetadata{
		Title:       sr.Title,
		Subtitle:    sr.Subtitle,
		Description: sr.Description,
		Publisher:   sr.Publisher,
		Imprint:     sr.Imprint,
		URL:         sr.URL,
		Series:      sr.Series,
		SeriesNumber: sr.SeriesNumber,
		Authors:     sr.Authors,
		Narrators:   sr.Narrators,
		Genres:      sr.Genres,
		Tags:        sr.Tags,
		Identifiers: sr.Identifiers,
		CoverURL:    sr.CoverURL,
	}
	// Fall back to ImageURL if CoverURL not set (backward compat with plugins
	// that populate ImageURL but haven't adopted the new CoverURL field yet)
	if md.CoverURL == "" && sr.ImageURL != "" {
		md.CoverURL = sr.ImageURL
	}
	if sr.ReleaseDate != "" {
		t, err := time.Parse("2006-01-02", sr.ReleaseDate)
		if err != nil {
			t, err = time.Parse(time.RFC3339, sr.ReleaseDate)
		}
		if err == nil {
			md.ReleaseDate = &t
		}
	}
	return md
}
```

Place this in `pkg/plugins/hooks.go` near the `SearchResult` type since it's a SearchResult method.

- [ ] **Step 3: Update the enricher loop**

The loop should now:
1. Call `RunMetadataSearch` (unchanged)
2. Take first result
3. Convert to `ParsedMetadata` via `searchResultToMetadata`
4. Filter fields using `filterMetadataFields`
5. Download cover if needed
6. Merge into `enrichedMeta`

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: Scan worker tests should still pass. Some enricher-specific tests may need updating.

- [ ] **Step 5: Commit**

```bash
git add pkg/plugins/hooks.go pkg/worker/scan_unified.go
git commit -m "[Backend] Update scan worker to use search results directly"
```

---

### Task 5: Update plugin SDK types

Update `packages/plugin-types/` to remove `enrich()` and update `SearchResult` authors.

**Files:**
- Modify: `packages/plugin-types/hooks.d.ts`
- Modify: `packages/plugin-types/metadata.d.ts` (verify `ParsedAuthor` is already exported)

- [ ] **Step 1: Update hooks.d.ts**

In `packages/plugin-types/hooks.d.ts`:
- Remove `EnrichContext` interface (lines 77-99)
- Remove `enrich` method from the `metadataEnricher` section of `ShishoPlugin`
- Update `SearchResult.authors` from `string[]` to `Array<{ name: string; role?: string }>`
- Add missing fields to `SearchResult`: `imprint?: string`, `url?: string`, `coverUrl?: string`
- Remove `providerData` and `metadata` from `SearchResult`

- [ ] **Step 2: Run type checks**

Run: `yarn lint:types` from the `packages/plugin-types/` directory (or wherever applicable)

- [ ] **Step 3: Commit**

```bash
git add packages/plugin-types/
git commit -m "[Backend] Update plugin SDK types to remove enrich hook"
```

---

### Task 6: Update frontend types and remove usePluginEnrich

Update the frontend to match the new backend types.

**Files:**
- Modify: `app/hooks/queries/plugins.ts` (update `PluginSearchResult`, delete `usePluginEnrich`)
- Modify: `app/libraries/api.ts` (delete `enrichMetadata` function if it exists)

- [ ] **Step 1: Update PluginSearchResult type**

In `app/hooks/queries/plugins.ts`, update `PluginSearchResult` (lines 503-537):

```typescript
export interface PluginSearchResult {
  title: string;
  authors?: Array<{ name: string; role?: string }>;
  description?: string;
  image_url?: string;
  release_date?: string;
  publisher?: string;
  subtitle?: string;
  series?: string;
  series_number?: number;
  genres?: string[];
  tags?: string[];
  narrators?: string[];
  identifiers?: Array<{ type: string; value: string }>;
  imprint?: string;
  url?: string;
  cover_url?: string;
  plugin_scope: string;
  plugin_id: string;
  disabled_fields?: string[];
}
```

Removed: `provider_data`, `metadata` subfield.
Added: `imprint`, `url`, `cover_url`.
Changed: `authors` from `string[]` to `Array<{ name: string; role?: string }>`.

- [ ] **Step 2: Delete usePluginEnrich**

In `app/hooks/queries/plugins.ts`, delete the `usePluginEnrich` hook (lines 558-590) and its related API function.

Also delete the `enrichMetadata` function from `app/libraries/api.ts` if it exists as a standalone function.

- [ ] **Step 3: Add usePluginApply mutation**

In `app/hooks/queries/plugins.ts`, add:

```typescript
interface PluginApplyPayload {
  book_id: number;
  file_id?: number;
  fields: Record<string, unknown>;
  plugin_scope: string;
  plugin_id: string;
}

export function usePluginApply() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: PluginApplyPayload) =>
      api.request("POST", "/plugins/apply", payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
}
```

- [ ] **Step 4: Fix IdentifyBookDialog references**

In `app/components/library/IdentifyBookDialog.tsx`, update any references to the old `usePluginEnrich` hook — these will become compile errors. For now, just remove the import and the mutation call. The actual review form is built in the next tasks.

Also update any places that read `result.authors` as `string[]` — it's now `Array<{ name: string; role?: string }>`. The search result display code (the result rows) will need to map `result.authors?.map(a => a.name)` instead of using the array directly.

- [ ] **Step 5: Run lint and type checks**

Run: `yarn lint`
Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add app/hooks/queries/plugins.ts app/libraries/api.ts app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Update types, add usePluginApply, remove usePluginEnrich"
```

---

### Task 7: Build IdentifyReviewForm component

Create the new review form component that shows the field-by-field comparison.

**Files:**
- Create: `app/components/library/IdentifyReviewForm.tsx`

- [ ] **Step 1: Create the component file**

Create `app/components/library/IdentifyReviewForm.tsx` with the component shell:

```typescript
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { usePluginApply, type PluginSearchResult } from "@/hooks/queries/plugins";
import type { Book } from "@/types";
import { formatMetadataFieldLabel } from "@/utils/format";

interface IdentifyReviewFormProps {
  result: PluginSearchResult;
  book: Book;
  fileId?: number;
  onBack: () => void;
  onClose: () => void;
}
```

- [ ] **Step 2: Implement smart merge logic**

Add the smart merge initialization that compares book data vs. search result:

```typescript
type FieldStatus = "unchanged" | "changed" | "new";

interface FieldState {
  value: any;
  currentValue: any;
  status: FieldStatus;
  disabled: boolean;
}

function computeInitialFields(book: Book, result: PluginSearchResult, fileId?: number): Record<string, FieldState> {
  // ... compare each field between book and result
  // Apply smart merge: new where empty, keep existing where match, plugin where both differ
  // Mark disabled fields from result.disabled_fields
}
```

For each field, determine the current book value:
- Book-level: `book.title`, `book.subtitle`, `book.description`, `book.authors`, `book.series`, `book.genres`, `book.tags`
- File-level: find the file matching `fileId` in `book.files`, read its `narrators`, `publisher`, `imprint`, `identifiers`, `release_date`, `url`

Then compare against the search result's value to determine status and default value.

- [ ] **Step 3: Implement individual field renderers**

Create renderers for each field type:

- `TextField` — label, current ref bar, text input
- `TextareaField` — label, collapsible current ref, textarea (for description)
- `TagField` — label, current ref with tags, tag input area (for authors, narrators, genres, tags, identifiers)
- `CoverField` — label, side-by-side cover thumbnails with click-to-select
- `SeriesField` — label, current ref, two inputs (name + number) side-by-side

Each field renderer shows:
1. Field label + change badge (Unchanged grey, Changed purple, New green)
2. Current reference bar with "Use current" button (when status is "changed")
3. The editable input pre-filled with the merged value
4. Disabled state + tooltip for disabled fields

- [ ] **Step 4: Implement the form layout**

Render all fields in order:
1. Cover
2. Title
3. Subtitle
4. Authors
5. Narrators
6. Series (name + number)
7. Genres
8. Tags
9. Description
10. Publisher
11. Imprint
12. Release Date
13. URL
14. Identifiers

Footer buttons: "Back to results" (calls `onBack`), "Cancel" (calls `onClose`), "Apply Changes" (submits).

- [ ] **Step 5: Implement submit handler**

On submit, collect form state into the `fields` map and call `usePluginApply`:

```typescript
const applyMutation = usePluginApply();

const handleApply = () => {
  applyMutation.mutate(
    {
      book_id: book.id,
      file_id: fileId,
      fields: collectFieldValues(fieldStates),
      plugin_scope: result.plugin_scope,
      plugin_id: result.plugin_id,
    },
    {
      onSuccess: () => {
        toast.success("Metadata applied successfully");
        onClose();
      },
      onError: (err) => {
        toast.error("Failed to apply metadata");
      },
    }
  );
};
```

- [ ] **Step 6: Run lint**

Run: `yarn lint`

- [ ] **Step 7: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Add IdentifyReviewForm component"
```

---

### Task 8: Integrate review form into IdentifyBookDialog

Wire up the 2-step flow: search -> review, with file selector at the top.

**Files:**
- Modify: `app/components/library/IdentifyBookDialog.tsx`

- [ ] **Step 1: Add step state and file selector**

Add `step` state to the dialog:

```typescript
const [step, setStep] = useState<"search" | "review">("search");
```

Move the file selector dropdown to the top of the dialog (above both steps). It should be visible in both search and review steps. Currently the file selector is shown in the search results area — move it up.

- [ ] **Step 2: Update result click handler**

When a search result is clicked, instead of immediately applying:

```typescript
const handleSelectResult = (result: PluginSearchResult) => {
  setSelectedResult(result);
  setStep("review");
};
```

- [ ] **Step 3: Conditionally render search vs. review**

```tsx
{step === "search" && (
  // Existing search UI: search bar + results list
)}

{step === "review" && selectedResult && (
  <IdentifyReviewForm
    result={selectedResult}
    book={book}
    fileId={selectedFileId}
    onBack={() => setStep("search")}
    onClose={() => onOpenChange(false)}
  />
)}
```

- [ ] **Step 4: Remove old apply logic**

Remove the `handleApply` function that called `usePluginEnrich`. Remove the `enrichMutation` variable. The "Apply" button in search results is no longer needed — clicking the row transitions to review.

- [ ] **Step 5: Reset step on dialog open/close**

When the dialog opens, reset to search step:

```typescript
useEffect(() => {
  if (open) {
    setStep("search");
    setSelectedResult(null);
  }
}, [open]);
```

- [ ] **Step 6: Run lint and type checks**

Run: `yarn lint`

- [ ] **Step 7: Manual test**

Run: `make start`
Test the full flow: open identify dialog → search → click result → review form → edit fields → apply.

- [ ] **Step 8: Commit**

```bash
git add app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Integrate review form into identify dialog"
```

---

### Task 9: Run all checks and fix issues

Final validation pass.

**Files:** Any files that need fixes.

- [ ] **Step 1: Run make tygo**

Run: `make tygo`
This regenerates TypeScript types from Go structs. Verify no issues.

- [ ] **Step 2: Run full check suite**

Run: `make check:quiet`
Expected: All checks pass (Go tests, Go lint, JS lint, JS types).

- [ ] **Step 3: Fix any issues found**

Address any remaining compilation errors, lint warnings, or test failures.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "[Fix] Address check suite issues"
```
