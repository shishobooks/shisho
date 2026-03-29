# File Hints and Confidence Scores for Search

**Date:** 2026-03-28
**Status:** Approved
**Scope:** `pkg/plugins/`, `pkg/worker/`, `pkg/mediafile/`, `pkg/config/`, `pkg/models/`, `app/`, `packages/plugin-sdk/`, `website/docs/`

## Problem

Plugins have no access to non-modifiable file metadata (duration, page count, file type, file size) during search, which limits matching accuracy. For example, an audiobook enricher can't use duration to verify a match from Audible. Additionally, plugins can't express how confident they are in each search result, which means the scan pipeline always takes the first result regardless of match quality.

## Design

### 1. File Hints in SearchContext

Add an optional `file` object to the search context with read-only file metadata:

```typescript
interface SearchContext {
  query: string;
  author?: string;
  identifiers?: Array<{ type: string; value: string }>;
  file?: {
    fileType?: string;       // "epub", "cbz", "m4b", "pdf"
    duration?: number;        // seconds (audiobooks only)
    pageCount?: number;       // CBZ/PDF only
    filesizeBytes?: number;   // file size in bytes
  };
}
```

- Populated automatically by both the interactive handler and scan pipeline
- Not shown in the identify dialog UI (not user-editable)
- The interactive handler needs to load the book with Files relation to populate this from the first file (or selected file)

### 2. Confidence Score on Search Results

Plugins return an optional `confidence` (0-1 float) on each result:

```javascript
return {
  results: [{
    title: "The Great Book",
    confidence: 0.92,
    // ... other metadata fields
  }]
};
```

**Go:** Add `Confidence *float64` to `ParsedMetadata` with `json:"confidence,omitempty"`. Parse from JS result in `parseSearchResponse`. Inherited by `EnrichSearchResult` via embedding — reaches the frontend.

**Frontend:** `PluginSearchResult` gets `confidence?: number`. Identify dialog shows confidence as a percentage badge on each result (e.g., "92%"). Subtle treatment — small badge, muted color, doesn't dominate.

### 3. Auto-Apply with Confidence Threshold During Scan

In `runMetadataEnrichers`, after getting the first search result:

1. If result has no confidence score → apply as before (backwards compat)
2. If result has confidence score → check against threshold
3. If confidence >= threshold → apply and log at info level (plugin name, confidence, book title)
4. If confidence < threshold → skip this enricher and log at warn level

**Threshold resolution order:**
1. Per-plugin `confidence_threshold` on the `plugins` table (nullable float)
2. Global `enrichment.confidence_threshold` in config.yaml
3. Default: 0.85

### 4. Configuration

**Global config** (`config.go`):
```yaml
enrichment:
  confidence_threshold: 0.85
```

**Per-plugin:** New nullable `confidence_threshold` column on the `plugins` table. Managed via the existing plugin config API — add to the config response and accept in config updates.

### 5. Per-Plugin Threshold API

Extend the existing `GET /plugins/installed/:scope/:id/config` response to include `confidence_threshold`. Extend `POST /plugins/installed/:scope/:id/config` (or add a dedicated endpoint) to accept `confidence_threshold` updates. The plugin config dialog in the frontend can show this as a slider or number input.

## Files Affected

### Go (modify)
- `pkg/mediafile/mediafile.go` — add `Confidence *float64` to ParsedMetadata
- `pkg/plugins/hooks.go` — parse `confidence` in parseSearchResponse
- `pkg/plugins/handler.go` — add file hints to search context, load Files relation, expose threshold in config API
- `pkg/worker/scan_unified.go` — add file hints to search context, add confidence threshold check with logging
- `pkg/config/config.go` — add `Enrichment.ConfidenceThreshold` section
- `pkg/models/plugin.go` — add `ConfidenceThreshold *float64` to Plugin model

### Database
- New migration — add `confidence_threshold` column to `plugins` table

### Frontend (modify)
- `app/hooks/queries/plugins.ts` — add `confidence` to PluginSearchResult
- `app/components/library/IdentifyBookDialog.tsx` — show confidence badge on results

### TypeScript SDK (modify)
- `packages/plugin-sdk/hooks.d.ts` — add `file` to SearchContext
- `packages/plugin-sdk/metadata.d.ts` — add `confidence` to ParsedMetadata

### Config/Docs
- `shisho.example.yaml` — add `enrichment.confidence_threshold`
- `website/docs/configuration.md` — document new config field
- `website/docs/plugins/development.md` — document file hints and confidence
- `pkg/plugins/CLAUDE.md` — update
