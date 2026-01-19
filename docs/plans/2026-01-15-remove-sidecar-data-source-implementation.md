# Remove Sidecar Data Source Implementation Plan

> **⚠️ REVERTED:** This implementation was completed and later reverted on 2026-01-19.
> See the design document for details on why this was reverted.

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove `DataSourceSidecar` as a concept so sidecars use the appropriate file type's data source instead of having their own higher priority.

**Architecture:** Replace `DataSourceSidecar` with `DataSourceFileMetadata` as a generic fallback. All file-derived sources (EPUB, CBZ, M4B, file_metadata) share priority 1. Sidecars inherit their data source from the file type they represent.

**Tech Stack:** Go, SQLite (Bun ORM)

---

## Task 1: Update Data Source Constants and Priorities

**Files:**
- Modify: `pkg/models/data-source.go`

**Step 1: Read the current data-source.go file**

Run: `make tygo` (ensure types are up to date first)

**Step 2: Update the constants to remove sidecar and add file_metadata**

Replace the entire content of `pkg/models/data-source.go` with:

```go
package models

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceFileMetadata | typeof DataSourceExistingCover | typeof DataSourceEPUBMetadata | typeof DataSourceCBZMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual        = "manual"
	DataSourceFileMetadata  = "file_metadata"
	DataSourceExistingCover = "existing_cover"
	DataSourceEPUBMetadata  = "epub_metadata"
	DataSourceCBZMetadata   = "cbz_metadata"
	DataSourceM4BMetadata   = "m4b_metadata"
	DataSourceFilepath      = "filepath"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority       = 0
	DataSourceFileMetadataPriority = 1 // All file-derived sources share this
	DataSourceFilepathPriority     = 2
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

**Step 3: Run make tygo to regenerate TypeScript types**

Run: `make tygo`
Expected: Types regenerated (or "Nothing to be done" which is normal)

**Step 4: Commit the data source changes**

```bash
git add pkg/models/data-source.go
git commit -m "$(cat <<'EOF'
[Refactor] Remove DataSourceSidecar, add DataSourceFileMetadata

All file-derived sources now share priority 1 instead of sidecar
having its own priority. This allows file metadata updates to
override sidecar data when files are rescanned.
EOF
)"
```

---

## Task 2: Add Helper Function for File Data Source

**Files:**
- Modify: `pkg/worker/scan.go`

**Step 1: Add the getDataSourceForFile helper function**

Add this function near the top of scan.go (after the imports, before the first method):

```go
// getDataSourceForFile returns the appropriate data source based on file extension.
// File sidecars use the file's actual type to ensure they have the same priority
// as the file's metadata.
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

**Step 2: Verify the function compiles**

Run: `go build ./pkg/worker/...`
Expected: No errors

**Step 3: Commit the helper function**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Refactor] Add getDataSourceForFile helper for sidecar source mapping
EOF
)"
```

---

## Task 3: Update Book Sidecar Reading to Use DataSourceFileMetadata

**Files:**
- Modify: `pkg/worker/scan.go:665-712`

**Step 1: Replace all DataSourceSidecar references in book sidecar section**

Find the section starting with `// Apply book sidecar data` (around line 665) and replace every `models.DataSourceSidecar` with `models.DataSourceFileMetadata`:

```go
	// Apply book sidecar data (same priority as file metadata)
	if bookSidecarData != nil {
		jobLog.Info("applying book sidecar data", nil)
		if bookSidecarData.Title != "" && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[titleSource] {
			title = bookSidecarData.Title
			titleSource = models.DataSourceFileMetadata
		}
		if len(bookSidecarData.Authors) > 0 && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[authorSource] {
			authorSource = models.DataSourceFileMetadata
			authors = make([]mediafile.ParsedAuthor, 0)
			for _, a := range bookSidecarData.Authors {
				role := ""
				if a.Role != nil {
					role = *a.Role
				}
				authors = append(authors, mediafile.ParsedAuthor{Name: a.Name, Role: role})
			}
		}
		if len(bookSidecarData.Series) > 0 && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[seriesSource] {
			seriesSource = models.DataSourceFileMetadata
			seriesList = make([]seriesData, 0, len(bookSidecarData.Series))
			for _, s := range bookSidecarData.Series {
				if s.Name != "" {
					seriesList = append(seriesList, seriesData{
						name:      s.Name,
						number:    s.Number,
						sortOrder: s.SortOrder,
					})
				}
			}
		}
		if len(bookSidecarData.Genres) > 0 && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[genreSource] {
			genreSource = models.DataSourceFileMetadata
			genreNames = bookSidecarData.Genres
		}
		if len(bookSidecarData.Tags) > 0 && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[tagSource] {
			tagSource = models.DataSourceFileMetadata
			tagNames = bookSidecarData.Tags
		}
		if bookSidecarData.Subtitle != nil && *bookSidecarData.Subtitle != "" && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[subtitleSource] {
			subtitle = bookSidecarData.Subtitle
			subtitleSource = models.DataSourceFileMetadata
		}
		if bookSidecarData.Description != nil && *bookSidecarData.Description != "" && models.DataSourcePriority[models.DataSourceFileMetadata] < models.DataSourcePriority[descriptionSource] {
			description = bookSidecarData.Description
			descriptionSource = models.DataSourceFileMetadata
		}
	}
```

**Step 2: Update the comment above the sidecar reading section**

Find the comment around line 654:
```go
	// Read sidecar files if they exist (sidecar has priority 1, higher than file metadata)
```

Replace with:
```go
	// Read sidecar files if they exist (same priority as file metadata)
```

**Step 3: Verify it compiles**

Run: `go build ./pkg/worker/...`
Expected: No errors

**Step 4: Commit the book sidecar changes**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Refactor] Update book sidecar reading to use DataSourceFileMetadata
EOF
)"
```

---

## Task 4: Update File Sidecar Reading to Use Dynamic Data Source

**Files:**
- Modify: `pkg/worker/scan.go:1218-1287`

**Step 1: Replace DataSourceSidecar with dynamic source in file sidecar section**

Find the section starting with `// Apply file sidecar data for narrators` (around line 1218).

For each sidecar field, replace `models.DataSourceSidecar` with `getDataSourceForFile(path)`. The updated section should look like:

```go
	// Determine the appropriate data source for file sidecar data
	fileSidecarSource := getDataSourceForFile(path)

	// Apply file sidecar data for narrators (same priority as file metadata)
	if fileSidecarData != nil && len(fileSidecarData.Narrators) > 0 {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[narratorSource] {
			jobLog.Info("applying file sidecar data for narrators", logger.Data{"narrator_count": len(fileSidecarData.Narrators)})
			narratorSource = fileSidecarSource
			narratorNames = make([]string, 0)
			for _, n := range fileSidecarData.Narrators {
				narratorNames = append(narratorNames, n.Name)
			}
		}
	}

	// Apply file sidecar data for identifiers (same priority as file metadata)
	if fileSidecarData != nil && len(fileSidecarData.Identifiers) > 0 {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[identifierSource] {
			jobLog.Info("applying file sidecar data for identifiers", logger.Data{"identifier_count": len(fileSidecarData.Identifiers)})
			identifierSource = fileSidecarSource
			identifiers = make([]mediafile.ParsedIdentifier, 0)
			for _, id := range fileSidecarData.Identifiers {
				identifiers = append(identifiers, mediafile.ParsedIdentifier{
					Type:  id.Type,
					Value: id.Value,
				})
			}
		}
	}

	// Apply file sidecar data for URL (same priority as file metadata)
	if fileSidecarData != nil && fileSidecarData.URL != nil && *fileSidecarData.URL != "" {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[fileURLSource] {
			fileURL = fileSidecarData.URL
			fileURLSource = fileSidecarSource
		}
	}

	// Apply file sidecar data for publisher (same priority as file metadata)
	if fileSidecarData != nil && fileSidecarData.Publisher != nil && *fileSidecarData.Publisher != "" {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[publisherSource] {
			publisherName = fileSidecarData.Publisher
			publisherSource = fileSidecarSource
		}
	}

	// Apply file sidecar data for imprint (same priority as file metadata)
	if fileSidecarData != nil && fileSidecarData.Imprint != nil && *fileSidecarData.Imprint != "" {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[imprintSource] {
			imprintName = fileSidecarData.Imprint
			imprintSource = fileSidecarSource
		}
	}

	// Apply file sidecar data for release date (same priority as file metadata)
	if fileSidecarData != nil && fileSidecarData.ReleaseDate != nil && *fileSidecarData.ReleaseDate != "" {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[releaseDateSource] {
			// Parse the ISO 8601 date string from sidecar
			if t, err := time.Parse("2006-01-02", *fileSidecarData.ReleaseDate); err == nil {
				releaseDate = &t
				releaseDateSource = fileSidecarSource
			}
		}
	}

	// Apply file sidecar data for name (same priority as file metadata)
	if fileSidecarData != nil && fileSidecarData.Name != nil && *fileSidecarData.Name != "" {
		if models.DataSourcePriority[fileSidecarSource] < models.DataSourcePriority[fileNameSource] {
			jobLog.Info("applying file sidecar data for name", logger.Data{"name": *fileSidecarData.Name})
			fileName = fileSidecarData.Name
			fileNameSource = fileSidecarSource
		}
	}
```

**Step 2: Verify it compiles**

Run: `go build ./pkg/worker/...`
Expected: No errors

**Step 3: Commit the file sidecar changes**

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Refactor] Update file sidecar reading to use dynamic data source

File sidecars now use getDataSourceForFile() to determine their
source based on file extension (epub_metadata, cbz_metadata, etc.)
EOF
)"
```

---

## Task 5: Update Test File to Use DataSourceFileMetadata

**Files:**
- Modify: `pkg/worker/scan_helpers_test.go`

**Step 1: Replace all DataSourceSidecar references with DataSourceFileMetadata**

In `pkg/worker/scan_helpers_test.go`, replace every `models.DataSourceSidecar` with `models.DataSourceFileMetadata`:

Lines to update:
- Line 24: `newSource: models.DataSourceFileMetadata,`
- Line 32: `newSource: models.DataSourceFileMetadata,`
- Line 40: `newSource: models.DataSourceFileMetadata,`
- Line 80: `newSource: models.DataSourceFileMetadata,`
- Line 115: `newSource: models.DataSourceFileMetadata,`
- Line 123: `newSource: models.DataSourceFileMetadata,`
- Line 131: `newSource: models.DataSourceFileMetadata,`
- Line 179: `newSource: models.DataSourceFileMetadata,`

**Step 2: Run the tests to verify they pass**

Run: `go test ./pkg/worker/... -run "TestShouldUpdate" -v`
Expected: All tests pass

**Step 3: Commit the test updates**

```bash
git add pkg/worker/scan_helpers_test.go
git commit -m "$(cat <<'EOF'
[Test] Update scan helper tests to use DataSourceFileMetadata
EOF
)"
```

---

## Task 6: Run Full Test Suite and Verify

**Files:**
- None (verification only)

**Step 1: Run the full test suite**

Run: `make test`
Expected: All tests pass

**Step 2: Run the linter**

Run: `make lint`
Expected: No linting errors

**Step 3: Run the full check**

Run: `make check`
Expected: All checks pass

---

## Task 7: Final Cleanup and Squash Commit

**Files:**
- None (git operations only)

**Step 1: View the commit history**

Run: `git log --oneline -10`
Expected: See the commits made in this implementation

**Step 2: Verify there are no remaining references to DataSourceSidecar**

Run: `grep -r "DataSourceSidecar" pkg/`
Expected: No results

**Step 3: Note completion**

The implementation is complete. When ready to merge to master, use the `squash-merge-worktree` skill to create a single squash commit.

---

## Summary of Changes

1. **pkg/models/data-source.go**: Removed `DataSourceSidecar` constant and priority. Added `DataSourceFileMetadata`. All file-derived sources now share priority 1.

2. **pkg/worker/scan.go**:
   - Added `getDataSourceForFile()` helper function
   - Updated book sidecar reading to use `DataSourceFileMetadata`
   - Updated file sidecar reading to use dynamic source from `getDataSourceForFile()`

3. **pkg/worker/scan_helpers_test.go**: Updated test cases to use `DataSourceFileMetadata` instead of `DataSourceSidecar`.
