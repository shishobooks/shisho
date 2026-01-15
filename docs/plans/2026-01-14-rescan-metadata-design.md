# Rescan Metadata Design

## Problem

Currently, when a subsequent scan happens, existing files/books are skipped entirely. This means:
- If file metadata changes, it won't be picked up
- If application code is updated to parse new fields, they won't be extracted
- Users must delete and re-add files to get fresh metadata

## Goal

Files should be re-scanned on every sync. New metadata should be picked up based on source priority. If nothing has changed, the scan should be idempotent (no DB writes).

## Design Decisions

| Decision | Choice |
|----------|--------|
| Rescan trigger | Always re-parse every file |
| Same priority handling | Prefer non-empty, then new |
| Relationship updates | Full replace at same-or-higher priority |
| Cover behavior | Keep current (don't re-extract if exists on disk) |

## Priority Logic

### Scalar Fields

```go
func shouldUpdateField(newValue, existingValue string, newSource, existingSource string) bool {
    newPriority := models.DataSourcePriority[newSource]
    existingPriority := models.DataSourcePriority[existingSource]

    // Higher priority (lower number) always wins
    if newPriority < existingPriority {
        return newValue != "" // Only if new value is non-empty
    }

    // Same priority: prefer non-empty, then new
    if newPriority == existingPriority {
        if newValue == "" {
            return false // Keep existing if new is empty
        }
        return newValue != existingValue // Update only if different
    }

    // Lower priority (higher number) never overwrites
    return false
}
```

### Relationships (authors, series, genres, tags)

```go
func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string) bool {
    newPriority := models.DataSourcePriority[newSource]
    existingPriority := models.DataSourcePriority[existingSource]

    if newPriority < existingPriority {
        return len(newItems) > 0
    }

    if newPriority == existingPriority {
        if len(newItems) == 0 {
            return false
        }
        return !pointerutil.EqualSlices(newItems, existingItems)
    }

    return false
}
```

## Scan Flow Changes

### Current Flow (problematic)

1. Walk library paths
2. For each file, check if filepath exists in DB
3. **If exists → skip processing entirely** (except cover recovery)
4. If new → create book + file

### New Flow

1. **Extract metadata** (always, regardless of file existence)
   - Parse file (EPUB/CBZ/M4B)
   - Read sidecars (book + file)
   - Parse filename patterns

2. **Check if file exists in DB**
   - If new → create book + file with extracted metadata (unchanged)
   - If exists → continue to comparison step

3. **Compare and update** (new logic for existing files)
   - For each scalar field: use `shouldUpdateField()` to decide
   - For each relationship: use `shouldUpdateRelationship()` to decide
   - Collect all fields that need updating
   - If any changes → single DB update with changed fields only
   - If no changes → skip DB write entirely (idempotent)

4. **Cover handling** (unchanged)
   - Skip extraction if cover file exists on disk
   - Extract only if no cover present

5. **Supplements** (unchanged)
   - Check by filepath, create only if new

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/worker/scan.go` | Remove early return for existing files; add comparison logic before updates |
| `pkg/worker/scan_helpers.go` (new) | Add `shouldUpdateField()` and `shouldUpdateRelationship()` helpers |

## Edge Cases

1. **Empty source field in DB** - Treat as lowest priority (filepath level) for backwards compatibility with existing data

2. **File deleted between scans** - Unchanged; orphan cleanup already handles this

3. **Concurrent scans** - Unchanged; existing job locking prevents conflicts

4. **File swap (same name, different content)** - New metadata picked up since same priority takes new non-empty value

5. **Manual edits** - Preserved; manual has priority 0, file sources have priority 3+

## Testing Strategy

### Unit Tests

- `shouldUpdateField()` with various priority combinations
- `shouldUpdateRelationship()` with list comparisons

### Integration Test Scenarios

- New file → creates book with correct sources
- Existing file, no changes → no DB writes (idempotency)
- Existing file, same priority, new value → updates
- Existing file, lower priority source → no update
- Existing file, higher priority source → updates
- File swap (same name, different content) → picks up new metadata
- Manual edit preserved through rescan

### Relationship Tests

- Authors added/changed at same priority
- Manual author edit preserved
- Empty relationship list doesn't overwrite existing

## Future Work (Separate Tasks)

- **Per-book "Refresh Metadata" button** - Reset all source fields and rescan
- **Per-scan "Force Refresh" flag** - Ignore priorities for bulk rebuilds
