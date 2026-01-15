# Remove Sidecar Data Source Design

## Problem

Currently, `sidecar` is a possible data source with priority 1 (second only to manual edits). This causes a problem when:

1. A folder is moved to another library, or the database is wiped and regenerated
2. All metadata is read from sidecars and tagged with `DataSourceSidecar`
3. If the underlying files (EPUB, CBZ, M4B) are later updated, those changes are never picked up because sidecar has higher priority than file metadata

The purpose of sidecars is to store metadata changes non-destructively (instead of writing back to the file). The sidecar's priority should match the file type it represents, not be higher.

## Solution

Remove `DataSourceSidecar` as a concept. Sidecars become a storage mechanism, not a data source. The data source reflects what the sidecar represents.

## New Priority System

| Priority | Data Source | Used For |
|----------|-------------|----------|
| 0 | `manual` | User edits via API |
| 1 | `file_metadata` | Book sidecars |
| 1 | `epub_metadata` | EPUB files and their sidecars |
| 1 | `cbz_metadata` | CBZ files and their sidecars |
| 1 | `m4b_metadata` | M4B files and their sidecars |
| 1 | `existing_cover` | Manually placed cover files |
| 2 | `filepath` | Extracted from filename patterns |

All file-derived sources share priority 1. This means:
- If a sidecar says title is "Foo" and the EPUB is updated to say "Bar", the system uses "Bar" (same priority, file scanned more recently)
- Manual edits still take precedence over everything
- Filepath extraction remains the fallback

## Sidecar Reading Changes

**Book sidecars** use `DataSourceFileMetadata` since they don't correspond to a single file type (a book could have EPUB + M4B, or just images).

**File sidecars** use the file's actual type:
- `.epub` → `epub_metadata`
- `.cbz` → `cbz_metadata`
- `.m4b` → `m4b_metadata`
- Other → `file_metadata` (fallback)

Helper function:

```go
func getDataSourceForFile(path string) string {
    ext := strings.ToLower(filepath.Ext(path))
    switch ext {
    case ".epub":
        return models.DataSourceEPUBMetadata
    case ".cbz":
        return models.DataSourceCBZMetadata
    case ".m4b":
        return models.DataSourceM4BMetadata
    default:
        return models.DataSourceFileMetadata
    }
}
```

## Implementation Changes

### File: `pkg/models/data-source.go`

1. Remove `DataSourceSidecar` constant
2. Add `DataSourceFileMetadata` constant
3. Update priority map - all file-derived sources get priority 1:

```go
const (
    DataSourceManual          = "manual"
    DataSourceFileMetadata    = "file_metadata"  // NEW
    DataSourceExistingCover   = "existing_cover"
    DataSourceEPUBMetadata    = "epub_metadata"
    DataSourceCBZMetadata     = "cbz_metadata"
    DataSourceM4BMetadata     = "m4b_metadata"
    DataSourceFilepath        = "filepath"
)

const (
    DataSourceManualPriority        = 0
    DataSourceFileMetadataPriority  = 1  // All file-derived share this
    DataSourceFilepathPriority      = 2
)

var DataSourcePriority = map[string]int{
    DataSourceManual:        DataSourceManualPriority,
    DataSourceFileMetadata:  DataSourceFileMetadataPriority,
    DataSourceExistingCover: DataSourceFileMetadataPriority,
    DataSourceEPUBMetadata:  DataSourceFileMetadataPriority,
    DataSourceCBZMetadata:   DataSourceFileMetadataPriority,
    DataSourceM4BMetadata:   DataSourceFileMetadataPriority,
    DataSourceFilepath:      DataSourceFilepathPriority,
}
```

### File: `pkg/worker/scan.go`

1. Add `getDataSourceForFile(path string) string` helper
2. Update book sidecar reading to use `DataSourceFileMetadata`
3. Update file sidecar reading to use `getDataSourceForFile()`
4. Remove all references to `DataSourceSidecar`

### File: `pkg/worker/scan_helpers.go`

No changes needed - `shouldUpdateScalar` and `shouldUpdateRelationship` already work with the priority map.

## Edge Cases

**Unknown file extensions:** Files without recognized extensions use `DataSourceFileMetadata` as fallback.

**Books with no files:** Book sidecars for books without files use `DataSourceFileMetadata`.

## Migration

No backwards compatibility needed. Assumes fresh database.

## Testing

1. Create a book with an EPUB containing metadata
2. Manually edit the title via API → source becomes `manual`
3. Rescan → sidecar is read, but manual still wins (priority 0 vs 1)
4. Update the EPUB file's internal metadata
5. Rescan → new EPUB metadata applies since `epub_metadata` priority equals previous source
