# File Hints and Confidence Scores Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add file context hints and confidence scores to the plugin search system, with auto-apply threshold during scan.

**Architecture:** Extend SearchContext with a read-only `file` object, add `confidence` field to ParsedMetadata/search results, implement per-plugin and global confidence thresholds that gate auto-apply during scan enrichment.

**Tech Stack:** Go (Echo, Bun, goja), React (TypeScript), TailwindCSS, SQLite

**Spec:** `docs/superpowers/specs/2026-03-28-file-hints-confidence-design.md`

---

### Task 1: Add Confidence Field to ParsedMetadata and Search Parsing

**Files:**
- Modify: `pkg/mediafile/mediafile.go` (add Confidence field)
- Modify: `pkg/plugins/hooks.go` (parse confidence in parseSearchResponse)
- Modify: `packages/plugin-sdk/metadata.d.ts` (add confidence to TS type)

- [ ] **Step 1: Add Confidence to Go ParsedMetadata**

In `pkg/mediafile/mediafile.go`, add after the `Chapters` field (line 71):

```go
	// Confidence is an optional score (0-1) indicating how confident the plugin
	// is that this result matches the search query. Used by the scan pipeline
	// to decide whether to auto-apply enrichment results.
	Confidence *float64 `json:"confidence,omitempty"`
```

- [ ] **Step 2: Parse confidence in parseSearchResponse**

In `pkg/plugins/hooks.go`, after the `seriesNumber` parsing block (around line 348), add:

```go
		// confidence -> *float64 (0-1 score)
		confidenceVal := itemObj.Get("confidence")
		if confidenceVal != nil && !goja.IsUndefined(confidenceVal) && !goja.IsNull(confidenceVal) {
			c := confidenceVal.ToFloat()
			md.Confidence = &c
		}
```

- [ ] **Step 3: Add confidence to TypeScript ParsedMetadata**

In `packages/plugin-sdk/metadata.d.ts`, add before the closing brace of `ParsedMetadata` (after `chapters`):

```typescript
  /**
   * Confidence score (0-1) indicating how well this result matches the search query.
   * Used by the scan pipeline to decide whether to auto-apply enrichment.
   * If omitted, the result is always applied (backwards compatible).
   */
  confidence?: number;
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -10`

Expected: All tests pass (confidence is optional, doesn't break existing tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/mediafile/mediafile.go pkg/plugins/hooks.go packages/plugin-sdk/metadata.d.ts
git commit -m "[Feature] Add confidence score field to ParsedMetadata and search parsing

Plugins can return an optional confidence (0-1) on each search result.
Parsed in parseSearchResponse, serialized to frontend via JSON."
```

---

### Task 2: Add File Hints to SearchContext

**Files:**
- Modify: `pkg/plugins/handler.go` (add file hints to interactive search context, re-add Files relation)
- Modify: `pkg/worker/scan_unified.go` (add file hints to scan search context)
- Modify: `packages/plugin-sdk/hooks.d.ts` (add file to SearchContext type)

- [ ] **Step 1: Update SearchContext TypeScript type**

In `packages/plugin-sdk/hooks.d.ts`, update the `SearchContext` interface to add the file hints:

```typescript
/** Context passed to metadataEnricher.search(). */
export interface SearchContext {
  /** Search query — title or free-text. Always present. */
  query: string;
  /** Author name to narrow results. Optional. */
  author?: string;
  /** Structured identifiers for direct lookup (ISBN, ASIN, etc.). Optional. */
  identifiers?: Array<{ type: string; value: string }>;
  /** Read-only file metadata for matching hints. Not user-editable. */
  file?: {
    /** File type extension (e.g., "epub", "cbz", "m4b", "pdf"). */
    fileType?: string;
    /** Audiobook duration in seconds. */
    duration?: number;
    /** Number of pages (CBZ/PDF). */
    pageCount?: number;
    /** File size in bytes. */
    filesizeBytes?: number;
  };
}
```

- [ ] **Step 2: Add file hints to interactive search handler**

In `pkg/plugins/handler.go`, the `searchMetadata` handler needs to populate file hints. First, re-add the Files relation to the fallback DB query (around line 1287). Change:

```go
		var b models.Book
		err = h.db.NewSelect().Model(&b).
			Where("b.id = ?", payload.BookID).
			Scan(ctx)
```

To:

```go
		var b models.Book
		err = h.db.NewSelect().Model(&b).
			Where("b.id = ?", payload.BookID).
			Relation("Files").
			Scan(ctx)
```

Then after the search context building (after the identifiers block, before `var allResults`), add file hints:

```go
	// Add file hints from the book's first file (non-modifiable context)
	if len(book.Files) > 0 {
		f := book.Files[0]
		fileCtx := map[string]interface{}{
			"fileType": f.FileType,
		}
		if f.AudiobookDurationSeconds != nil {
			fileCtx["duration"] = *f.AudiobookDurationSeconds
		}
		if f.PageCount != nil {
			fileCtx["pageCount"] = *f.PageCount
		}
		fileCtx["filesizeBytes"] = f.FilesizeBytes
		searchCtx["file"] = fileCtx
	}
```

- [ ] **Step 3: Add file hints to scan pipeline search context**

In `pkg/worker/scan_unified.go`, inside the per-runtime loop in `runMetadataEnrichers`, after the identifiers block and before the `RunMetadataSearch` call, add:

```go
		// Add file hints (non-modifiable context)
		if file != nil {
			fileCtx := map[string]interface{}{
				"fileType": file.FileType,
			}
			if file.AudiobookDurationSeconds != nil {
				fileCtx["duration"] = *file.AudiobookDurationSeconds
			}
			if file.PageCount != nil {
				fileCtx["pageCount"] = *file.PageCount
			}
			fileCtx["filesizeBytes"] = file.FilesizeBytes
			searchCtx["file"] = fileCtx
		}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -10`
Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/worker/ -v -count=1 2>&1 | tail -10`

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/plugins/handler.go pkg/worker/scan_unified.go packages/plugin-sdk/hooks.d.ts
git commit -m "[Feature] Add file hints to search context

Plugins receive file.fileType, file.duration, file.pageCount, and
file.filesizeBytes as read-only hints for search matching. Same
shape for both interactive identify and scan pipeline."
```

---

### Task 3: Add Global Confidence Threshold Config

**Files:**
- Modify: `pkg/config/config.go` (add Enrichment config section)
- Modify: `shisho.example.yaml` (document new setting)
- Modify: `website/docs/configuration.md` (document new setting)

- [ ] **Step 1: Add Enrichment config to Config struct**

In `pkg/config/config.go`, add a new field to the Config struct. Find the section with other application settings and add:

```go
	// Enrichment settings
	EnrichmentConfidenceThreshold float64 `koanf:"enrichment_confidence_threshold" json:"enrichment_confidence_threshold"`
```

Add the default value in the `defaults` map (find where defaults are set):

```go
	"enrichment_confidence_threshold": 0.85,
```

- [ ] **Step 2: Add to shisho.example.yaml**

Add after the plugin settings section (around line 120):

```yaml
# Enrichment settings
#
# Confidence threshold for automatic metadata enrichment during scans.
# When a plugin returns a confidence score with search results, results
# below this threshold are skipped. Range: 0.0 - 1.0 (0.85 = 85%).
# Per-plugin thresholds can override this in plugin settings.
# Env: ENRICHMENT_CONFIDENCE_THRESHOLD
enrichment_confidence_threshold: 0.85
```

- [ ] **Step 3: Add to configuration docs**

In `website/docs/configuration.md`, add a row to the configuration table in the appropriate section:

```markdown
| `enrichment_confidence_threshold` | `ENRICHMENT_CONFIDENCE_THRESHOLD` | `0.85` | Confidence threshold (0-1) for automatic metadata enrichment during scans. When a plugin returns a confidence score, results below this threshold are skipped. Per-plugin thresholds override this value. |
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/config/ -v -count=1 2>&1 | tail -10`

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/config/config.go shisho.example.yaml website/docs/configuration.md
git commit -m "[Feature] Add global confidence threshold config

Default 0.85 (85%). Plugins returning search results with confidence
below this threshold are skipped during automatic scan enrichment."
```

---

### Task 4: Add Per-Plugin Confidence Threshold (DB + Model + API)

**Files:**
- Create: new migration file in `pkg/migrations/`
- Modify: `pkg/models/plugin.go` (add ConfidenceThreshold field)
- Modify: `pkg/plugins/handler.go` (expose threshold in config response, accept threshold updates)
- Modify: `pkg/plugins/service.go` (add threshold update method)

- [ ] **Step 1: Create database migration**

Create `pkg/migrations/20260328100000_add_plugin_confidence_threshold.go`:

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE plugins ADD COLUMN confidence_threshold REAL`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`ALTER TABLE plugins DROP COLUMN confidence_threshold`)
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	Migrations.MustRegister(up, down)
}
```

- [ ] **Step 2: Add ConfidenceThreshold to Plugin model**

In `pkg/models/plugin.go`, add to the Plugin struct (after `UpdateAvailableVersion`):

```go
	ConfidenceThreshold *float64 `json:"confidence_threshold"`
```

- [ ] **Step 3: Add threshold to config response**

In `pkg/plugins/handler.go`, find the `pluginConfigResponse` struct and add:

```go
type pluginConfigResponse struct {
	Schema              ConfigSchema           `json:"schema"`
	Values              map[string]interface{} `json:"values"`
	DeclaredFields      []string               `json:"declaredFields"`
	FieldSettings       map[string]bool        `json:"fieldSettings"`
	ConfidenceThreshold *float64               `json:"confidence_threshold"`
}
```

In the `getConfig` handler, set the new field from the plugin model:

```go
	plugin, err := h.service.GetPlugin(ctx, scope, pluginID)
	// ... (find where plugin is loaded)
	resp.ConfidenceThreshold = plugin.ConfidenceThreshold
```

- [ ] **Step 4: Accept threshold in update endpoint**

In `pkg/plugins/handler.go`, add `ConfidenceThreshold` to the `updatePayload` struct:

```go
type updatePayload struct {
	Enabled             *bool             `json:"enabled"`
	AutoUpdate          *bool             `json:"auto_update"`
	Config              map[string]string `json:"config"`
	ConfidenceThreshold *float64          `json:"confidence_threshold"`
}
```

In the update handler, save the threshold when provided:

```go
	if payload.ConfidenceThreshold != nil {
		if err := h.service.UpdateConfidenceThreshold(ctx, scope, pluginID, payload.ConfidenceThreshold); err != nil {
			return errors.WithStack(err)
		}
	}
```

- [ ] **Step 5: Add threshold update method to service**

In `pkg/plugins/service.go`, add:

```go
// UpdateConfidenceThreshold sets the per-plugin confidence threshold.
func (s *Service) UpdateConfidenceThreshold(ctx context.Context, scope, pluginID string, threshold *float64) error {
	_, err := s.db.NewUpdate().Model((*models.Plugin)(nil)).
		Set("confidence_threshold = ?", threshold).
		Where("scope = ?", scope).
		Where("id = ?", pluginID).
		Exec(ctx)
	return errors.WithStack(err)
}
```

- [ ] **Step 6: Run migration and tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise db:migrate 2>&1`
Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/plugins/ -v -count=1 2>&1 | tail -10`

Expected: Migration succeeds, all tests pass.

- [ ] **Step 7: Commit**

```bash
git add pkg/migrations/20260328100000_add_plugin_confidence_threshold.go pkg/models/plugin.go pkg/plugins/handler.go pkg/plugins/service.go
git commit -m "[Feature] Add per-plugin confidence threshold

New column on plugins table. Exposed in plugin config API response
and settable via plugin update endpoint. Nullable — falls back to
global config when not set."
```

---

### Task 5: Implement Confidence Threshold Check in Scan Pipeline

**Files:**
- Modify: `pkg/worker/scan_unified.go` (add threshold check after first result)

- [ ] **Step 1: Add threshold check in runMetadataEnrichers**

In `pkg/worker/scan_unified.go`, after the first result is selected (around line 2666), add the confidence threshold check. The current code:

```go
		// Take the first result directly as ParsedMetadata (no conversion needed)
		firstResult := searchResp.Results[0]
		searchMeta := &firstResult
```

Replace with:

```go
		// Take the first result directly as ParsedMetadata (no conversion needed)
		firstResult := searchResp.Results[0]
		searchMeta := &firstResult

		// Check confidence threshold (if result provides a score)
		if searchMeta.Confidence != nil {
			threshold := w.getConfidenceThreshold(rt)
			if *searchMeta.Confidence < threshold {
				logWarn("enricher result below confidence threshold, skipping", logger.Data{
					"plugin":     rt.PluginID(),
					"confidence": fmt.Sprintf("%.0f%%", *searchMeta.Confidence*100),
					"threshold":  fmt.Sprintf("%.0f%%", threshold*100),
					"book":       book.Title,
				})
				continue
			}
			log.Info("enricher auto-applying result", logger.Data{
				"plugin":     rt.PluginID(),
				"confidence": fmt.Sprintf("%.0f%%", *searchMeta.Confidence*100),
				"book":       book.Title,
			})
		}
```

- [ ] **Step 2: Add getConfidenceThreshold helper**

Add this method to the Worker (or as a helper in the same file):

```go
// getConfidenceThreshold returns the effective confidence threshold for a plugin.
// Priority: per-plugin threshold > global config > default (0.85).
func (w *Worker) getConfidenceThreshold(rt *plugins.Runtime) float64 {
	// Check per-plugin threshold
	if w.pluginService != nil {
		plugin, err := w.pluginService.GetPlugin(context.Background(), rt.Scope(), rt.PluginID())
		if err == nil && plugin != nil && plugin.ConfidenceThreshold != nil {
			return *plugin.ConfidenceThreshold
		}
	}

	// Fall back to global config
	if w.config != nil && w.config.EnrichmentConfidenceThreshold > 0 {
		return w.config.EnrichmentConfidenceThreshold
	}

	// Default
	return 0.85
}
```

Note: Check if `w.config` and `w.pluginService` are available on the Worker struct. If not, they may need to be accessed differently — read the Worker struct definition to see what's available.

- [ ] **Step 3: Add `fmt` import if not already present**

The logging uses `fmt.Sprintf` — ensure `fmt` is in the imports.

- [ ] **Step 4: Run tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && go test ./pkg/worker/ -v -count=1 2>&1 | tail -20`

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/worker/scan_unified.go
git commit -m "[Feature] Add confidence threshold check to scan pipeline

Results with confidence below threshold are skipped with a warning.
Results above threshold are applied with an info log. Results without
confidence are always applied (backwards compatible). Threshold
resolves: per-plugin → global config → 0.85 default."
```

---

### Task 6: Show Confidence Badge in Identify Dialog

**Files:**
- Modify: `app/hooks/queries/plugins.ts` (add confidence to PluginSearchResult)
- Modify: `app/components/library/IdentifyBookDialog.tsx` (show confidence badge)

- [ ] **Step 1: Add confidence to PluginSearchResult type**

In `app/hooks/queries/plugins.ts`, add to the `PluginSearchResult` interface:

```typescript
  confidence?: number;
```

- [ ] **Step 2: Show confidence badge on search results**

In `app/components/library/IdentifyBookDialog.tsx`, find the search result card rendering. Each result card shows the plugin name in a badge. Add a confidence badge next to it when the result has a confidence score.

Find where the plugin name badge is rendered in the result card (look for the plugin scope/id display). Add adjacent to it:

```tsx
{result.confidence != null && (
  <Badge
    className={cn(
      "text-xs",
      result.confidence >= 0.9
        ? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
        : result.confidence >= 0.7
          ? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400"
          : "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
    )}
    variant="secondary"
  >
    {Math.round(result.confidence * 100)}%
  </Badge>
)}
```

Color coding: green for ≥90%, yellow for ≥70%, red for below 70%.

- [ ] **Step 3: Run lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise lint:js 2>&1 | tail -10`

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add app/hooks/queries/plugins.ts app/components/library/IdentifyBookDialog.tsx
git commit -m "[Frontend] Show confidence badge on identify search results

Color-coded percentage badge: green ≥90%, yellow ≥70%, red below 70%.
Only shown when the plugin provides a confidence score."
```

---

### Task 7: Add Confidence Threshold to Plugin Config Dialog

**Files:**
- Modify: `app/components/plugins/PluginConfigDialog.tsx` (add threshold slider/input)

- [ ] **Step 1: Add threshold input to the config dialog**

In `app/components/plugins/PluginConfigDialog.tsx`, after the field settings section (where enricher field toggles are rendered), add a confidence threshold input for enricher plugins:

```tsx
{/* Confidence threshold */}
{configData?.declaredFields && configData.declaredFields.length > 0 && (
  <div className="space-y-2">
    <Label>Auto-identify confidence threshold</Label>
    <p className="text-xs text-muted-foreground">
      During automatic scans, results with confidence below this threshold
      will be skipped. Leave empty to use the global default.
    </p>
    <div className="flex items-center gap-2">
      <Input
        className="w-24"
        max={100}
        min={0}
        onChange={(e) => {
          const val = e.target.value;
          setConfidenceThreshold(val === "" ? null : Number(val));
        }}
        placeholder="85"
        type="number"
        value={confidenceThreshold ?? ""}
      />
      <span className="text-sm text-muted-foreground">%</span>
    </div>
  </div>
)}
```

This requires:
- Adding `confidenceThreshold` state initialized from `configData?.confidence_threshold`
- Including the threshold in the save mutation (convert from percentage to 0-1 float for the API)
- Including the threshold in the `hasChanges` check

The specific implementation depends on the existing dialog structure — the implementer should read the full `PluginConfigDialog.tsx` and follow the existing patterns for state management and save handling.

- [ ] **Step 2: Run lint**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise lint:js 2>&1 | tail -10`

- [ ] **Step 3: Commit**

```bash
git add app/components/plugins/PluginConfigDialog.tsx
git commit -m "[Frontend] Add confidence threshold input to plugin config dialog

Number input (0-100%) shown for enricher plugins. Empty means
use global default. Saved as 0-1 float via plugin update API."
```

---

### Task 8: Update Documentation

**Files:**
- Modify: `website/docs/plugins/development.md` (document file hints and confidence)
- Modify: `pkg/plugins/CLAUDE.md` (update)

- [ ] **Step 1: Update plugin development docs**

In `website/docs/plugins/development.md`, update the metadata enricher section:

1. Show file hints in the search context documentation:
```javascript
// context.file — read-only file metadata
// context.file.fileType   — "epub", "cbz", "m4b", "pdf"
// context.file.duration   — seconds (audiobooks)
// context.file.pageCount  — CBZ/PDF page count
// context.file.filesizeBytes — file size
```

2. Document confidence scores:
```javascript
return {
  results: [{
    title: "The Great Book",
    confidence: 0.92,  // optional, 0-1
    // ... other fields
  }]
};
```

3. Explain auto-apply behavior: confidence >= threshold → auto-applied during scan, confidence < threshold → skipped. No confidence → always applied (backwards compatible).

- [ ] **Step 2: Update pkg/plugins/CLAUDE.md**

Add file hints and confidence to the metadataEnricher documentation. Update the scan pipeline section to mention the confidence threshold check.

- [ ] **Step 3: Commit**

```bash
git add website/docs/plugins/development.md pkg/plugins/CLAUDE.md
git commit -m "[Docs] Document file hints, confidence scores, and auto-apply threshold"
```

---

### Task 9: Run Full Validation

- [ ] **Step 1: Run mise check:quiet**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise check:quiet`

Expected: All checks pass.

- [ ] **Step 2: Run mise tygo**

Run: `cd /Users/robinjoseph/.worktrees/shisho/plugin-refinement && mise tygo`

Verify generated types include the new `confidence` field from `ParsedMetadata` (with `json:"confidence,omitempty"` it should appear in generated TS).
