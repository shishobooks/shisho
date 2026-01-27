# Plugin Field Configuration Design

## Overview

This design adds per-field configuration to the plugin system, allowing users to control which metadata fields each enricher plugin can set. Plugins declare which fields they intend to set in their manifest, and users can enable/disable individual fields at both global and per-library levels.

## Goals

- Plugin authors declare which metadata fields their enricher will set
- Users can enable/disable individual fields per plugin
- Per-library overrides allow different configurations for different libraries
- Maintains existing "first non-empty wins" merge logic and source priority

## Manifest Schema Changes

The `MetadataEnricherCap` gains a required `fields` array:

```json
{
  "capabilities": {
    "metadataEnricher": {
      "description": "Enriches books with Goodreads metadata",
      "fileTypes": ["epub", "cbz"],
      "fields": ["title", "authors", "description", "genres", "cover", "identifiers"]
    }
  }
}
```

### Valid Field Names

Fixed enum validated at parse time:

| Field | Description |
|-------|-------------|
| `title` | Book title |
| `subtitle` | Book subtitle |
| `authors` | Author list with roles |
| `narrators` | Narrator names |
| `series` | Series name AND series number (grouped) |
| `seriesNumber` | Alias for `series` |
| `genres` | Genre list |
| `tags` | Tag list |
| `description` | Book description |
| `publisher` | Publisher name |
| `imprint` | Publisher imprint |
| `url` | External URL |
| `releaseDate` | Release/publication date |
| `cover` | Cover image (coverData, coverMimeType, coverPage grouped) |
| `identifiers` | External identifiers (ISBN, etc.) |

### Logical Field Groupings

Some logical fields map to multiple underlying struct fields:

- `cover` → controls `coverData`, `coverMimeType`, and `coverPage`
- `series` → controls both `series` (name) and `seriesNumber`

### Validation Behavior

- If `metadataEnricher` is declared but `fields` is missing or empty → plugin loads, but the enricher hook is **disabled**. Warning stored in `load_error` field.
- If `fields` contains unknown names → load fails with validation error.
- Other hooks (fileParser, outputGenerator, etc.) continue to work normally.
- File parsers do NOT require field declarations - only enrichers.

## Database Schema

### Global Field Settings

```sql
CREATE TABLE plugin_field_settings (
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    field TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (scope, plugin_id, field),
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);
```

### Per-Library Overrides

```sql
CREATE TABLE library_plugin_field_settings (
    library_id INTEGER NOT NULL,
    scope TEXT NOT NULL,
    plugin_id TEXT NOT NULL,
    field TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    PRIMARY KEY (library_id, scope, plugin_id, field),
    FOREIGN KEY (library_id) REFERENCES libraries(id) ON DELETE CASCADE,
    FOREIGN KEY (scope, plugin_id) REFERENCES plugins(scope, id) ON DELETE CASCADE
);
```

### Storage Behavior

- When a plugin is installed, no rows exist in `plugin_field_settings` → all declared fields are enabled by default
- Users toggle fields off → row inserted with `enabled=false`
- Per-library table only contains explicit overrides; absence means "use global default"
- Foreign key cascades ensure cleanup on plugin uninstall or library deletion

## Go Models

### New Models

```go
// pkg/models/plugin.go

type PluginFieldSetting struct {
    bun.BaseModel `bun:"table:plugin_field_settings,alias:pfs"`

    Scope    string `bun:",pk" json:"scope"`
    PluginID string `bun:",pk" json:"plugin_id"`
    Field    string `bun:",pk" json:"field"`
    Enabled  bool   `bun:",notnull" json:"enabled"`
}

type LibraryPluginFieldSetting struct {
    bun.BaseModel `bun:"table:library_plugin_field_settings,alias:lpfs"`

    LibraryID int    `bun:",pk" json:"library_id"`
    Scope     string `bun:",pk" json:"scope"`
    PluginID  string `bun:",pk" json:"plugin_id"`
    Field     string `bun:",pk" json:"field"`
    Enabled   bool   `bun:",notnull" json:"enabled"`
}
```

### Manifest Changes

```go
// pkg/plugins/manifest.go

// ValidMetadataFields lists all valid field names for enricher declarations
var ValidMetadataFields = []string{
    "title", "subtitle", "authors", "narrators",
    "series", "seriesNumber", "genres", "tags",
    "description", "publisher", "imprint", "url",
    "releaseDate", "cover", "identifiers",
}

type MetadataEnricherCap struct {
    Description string   `json:"description"`
    FileTypes   []string `json:"fileTypes"`
    Fields      []string `json:"fields"` // Required
}
```

### Service Layer Additions

```go
// pkg/plugins/service.go

// GetFieldSettings returns global field settings for a plugin.
// Fields not in the table are considered enabled by default.
func (s *Service) GetFieldSettings(ctx context.Context, scope, pluginID string) (map[string]bool, error)

// SetFieldSetting updates a single field's enabled state.
func (s *Service) SetFieldSetting(ctx context.Context, scope, pluginID, field string, enabled bool) error

// GetLibraryFieldSettings returns per-library field overrides.
func (s *Service) GetLibraryFieldSettings(ctx context.Context, libraryID int, scope, pluginID string) (map[string]bool, error)

// SetLibraryFieldSetting updates a per-library field override.
func (s *Service) SetLibraryFieldSetting(ctx context.Context, libraryID int, scope, pluginID, field string, enabled bool) error

// GetEffectiveFieldSettings merges global and per-library settings.
// Per-library settings fully override global (can enable or disable).
func (s *Service) GetEffectiveFieldSettings(ctx context.Context, libraryID int, scope, pluginID string) (map[string]bool, error)

// ResetLibraryFieldSettings removes all per-library overrides for a plugin.
func (s *Service) ResetLibraryFieldSettings(ctx context.Context, libraryID int, scope, pluginID string) error
```

## Runtime Filtering

Filtering happens in `pkg/worker/scan_unified.go` during enrichment, after the plugin returns its result but before merging.

### Updated Flow

```go
for _, rt := range runtimes {
    // ... existing file type check ...

    // Get effective field settings for this library + plugin
    enabledFields := w.pluginService.GetEffectiveFieldSettings(ctx, libraryID, rt.Scope(), rt.PluginID())

    result, err := w.pluginManager.RunMetadataEnricher(ctx, rt, enrichCtx)
    // ... error handling ...

    if result.Modified && result.Metadata != nil {
        // Filter to only enabled fields, log warnings for undeclared
        declaredFields := rt.Manifest().Capabilities.MetadataEnricher.Fields
        result.Metadata = filterMetadataFields(result.Metadata, declaredFields, enabledFields, rt.PluginID())
    }

    // ... existing merge logic (mergeEnrichedMetadata) ...
}
```

### Filter Function

```go
// filterMetadataFields zeros out fields that are undeclared or disabled.
// - Undeclared fields (returned but not in manifest) → zero + log warning
// - Disabled fields (declared but user disabled) → zero silently
func filterMetadataFields(
    md *mediafile.ParsedMetadata,
    declaredFields []string,
    enabledFields map[string]bool,
    pluginID string,
) *mediafile.ParsedMetadata
```

The existing "first non-empty wins" merge logic remains unchanged - it receives pre-filtered metadata.

## API Endpoints

### Global Field Settings

```
GET  /plugins/installed/:scope/:id/fields
     → { fields: { title: true, authors: true, cover: false, ... } }

PUT  /plugins/installed/:scope/:id/fields
     ← { title: true, authors: false, ... }
     → 204 No Content
```

### Per-Library Field Settings

```
GET  /libraries/:libraryId/plugins/:scope/:id/fields
     → { fields: { title: true, ... }, customized: true }

PUT  /libraries/:libraryId/plugins/:scope/:id/fields
     ← { title: false, authors: true, ... }
     → 204 No Content

DELETE /libraries/:libraryId/plugins/:scope/:id/fields
     → 204 No Content (resets to global defaults)
```

### Extended Config Endpoint

```
GET  /plugins/installed/:scope/:id/config
     → {
         schema: { ... },
         values: { ... },
         declaredFields: ["title", "authors", "cover"],
         fieldSettings: { title: true, authors: true, cover: true }
       }
```

## Frontend Changes

### PluginConfigDialog Extension

Add a "Metadata Fields" section below existing config fields (only shown for enrichers with declared fields):

```tsx
{data.declaredFields && data.declaredFields.length > 0 && (
  <div className="space-y-3">
    <Label>Metadata Fields</Label>
    <p className="text-xs text-muted-foreground">
      Choose which fields this plugin can set during enrichment.
    </p>
    <div className="space-y-2">
      {data.declaredFields.map((field) => (
        <div key={field} className="flex items-center justify-between">
          <span className="text-sm">{formatFieldLabel(field)}</span>
          <Switch
            checked={fieldSettings[field] ?? true}
            onCheckedChange={(checked) => handleFieldToggle(field, checked)}
          />
        </div>
      ))}
    </div>
  </div>
)}
```

### Field Label Formatting

Humanize field names for display:
- `title` → "Title"
- `seriesNumber` → "Series Number"
- `releaseDate` → "Release Date"
- `cover` → "Cover Image"

### Query Hook Updates

- Extend `usePluginConfig` response type with `declaredFields` and `fieldSettings`
- Add `useSavePluginFieldSettings` mutation (or extend `useSavePluginConfig`)

## Validation and Edge Cases

### Load-Time Validation

```go
if rt.metadataEnricher != nil {
    cap := manifest.Capabilities.MetadataEnricher
    if cap == nil || len(cap.Fields) == 0 {
        // Disable the enricher hook, set warning
        rt.metadataEnricher = nil
        rt.loadWarning = "metadataEnricher requires fields declaration"
    } else {
        // Validate field names
        for _, f := range cap.Fields {
            if !isValidMetadataField(f) {
                return fmt.Errorf("invalid metadata field %q in metadataEnricher.fields", f)
            }
        }
    }
}
```

### Edge Cases

| Case | Behavior |
|------|----------|
| Plugin uninstalled | Cascade delete cleans up field settings |
| Library deleted | Cascade delete cleans up library overrides |
| Plugin updated with different fields | Old settings for removed fields are orphaned (harmless), new fields default to enabled |
| Enricher returns empty metadata | No filtering needed, `modified: false` path |
| All fields disabled | Plugin runs but contributes nothing |
| Undeclared field returned | Stripped from result, warning logged |

## TypeScript SDK Updates

### manifest.d.ts

```typescript
interface MetadataEnricherCapability {
  description?: string;
  fileTypes: string[];
  fields: MetadataField[];
}

type MetadataField =
  | 'title' | 'subtitle' | 'authors' | 'narrators'
  | 'series' | 'seriesNumber' | 'genres' | 'tags'
  | 'description' | 'publisher' | 'imprint' | 'url'
  | 'releaseDate' | 'cover' | 'identifiers';
```

## Decision Summary

| Aspect | Decision |
|--------|----------|
| Scope | Global + per-library with full override |
| Declaration | Required for enrichers, simple array |
| Load behavior (no fields) | Disable enricher hook, warn |
| Runtime (undeclared returned) | Strip + log warning |
| File parsers | No requirement |
| Default state | All enabled |
| Field names | Fixed enum (15 fields) |
| Storage | New dedicated tables |
| UI | Extend existing config dialog |
| Manifest version | Stay at v1 |
