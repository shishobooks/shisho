# Rescan Metadata Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Re-parse existing files on every sync and update metadata based on source priority, while remaining idempotent (no DB writes if nothing changed).

**Architecture:** Remove the early return for existing files in `scanFile()`. Extract metadata as normal, then compare with existing DB values using priority-based logic. Only update fields where the new source has equal-or-higher priority and the value differs.

**Tech Stack:** Go, Bun ORM, SQLite

---

## Background

The current scan flow (`pkg/worker/scan.go:346-353`) returns early when a file already exists:

```go
if existingFile != nil {
    jobLog.Info("file already exists", logger.Data{"file_id": existingFile.ID})
    // Check if cover is missing and recover it if needed
    if err := w.recoverMissingCover(ctx, existingFile); err != nil {
        jobLog.Warn("failed to recover missing cover", logger.Data{"file_id": existingFile.ID, "error": err.Error()})
    }
    return nil  // <-- EARLY RETURN skips all metadata extraction
}
```

This means:
- If file metadata changes, it won't be picked up
- If application code is updated to parse new fields, they won't be extracted
- Users must delete and re-add files to get fresh metadata

**Priority System (lower number = higher priority):**
- 0: `manual` (user edits via UI)
- 1: `sidecar` (YAML sidecar files)
- 2: `existing_cover` (preserved covers)
- 3: `epub_metadata`, 4: `cbz_metadata`, 5: `m4b_metadata`
- 6: `filepath` (parsed from filename/directory)

---

## Task 1: Add Helper Functions

**Files:**
- Create: `pkg/worker/scan_helpers.go`
- Test: `pkg/worker/scan_helpers_test.go`

### Step 1: Write the failing test for `shouldUpdateScalar`

```go
// pkg/worker/scan_helpers_test.go
package worker

import (
    "testing"

    "github.com/shishobooks/shisho/pkg/models"
    "github.com/stretchr/testify/assert"
)

func TestShouldUpdateScalar(t *testing.T) {
    tests := []struct {
        name           string
        newValue       string
        existingValue  string
        newSource      string
        existingSource string
        want           bool
    }{
        {
            name:           "higher priority source with value updates",
            newValue:       "New Title",
            existingValue:  "Old Title",
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceEPUBMetadata,
            want:           true,
        },
        {
            name:           "higher priority source with empty value does not update",
            newValue:       "",
            existingValue:  "Old Title",
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "same priority with different value updates",
            newValue:       "New Title",
            existingValue:  "Old Title",
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           true,
        },
        {
            name:           "same priority with same value does not update",
            newValue:       "Same Title",
            existingValue:  "Same Title",
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "same priority with empty new value does not update",
            newValue:       "",
            existingValue:  "Old Title",
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "lower priority source never updates",
            newValue:       "New Title",
            existingValue:  "Old Title",
            newSource:      models.DataSourceFilepath,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "manual source is never overwritten",
            newValue:       "New Title",
            existingValue:  "Manual Title",
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceManual,
            want:           false,
        },
        {
            name:           "empty existing source treated as filepath priority",
            newValue:       "New Title",
            existingValue:  "Old Title",
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: "",
            want:           true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := shouldUpdateScalar(tt.newValue, tt.existingValue, tt.newSource, tt.existingSource)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Step 2: Run test to verify it fails

Run: `go test -v ./pkg/worker/... -run TestShouldUpdateScalar`
Expected: FAIL with "undefined: shouldUpdateScalar"

### Step 3: Write minimal implementation

```go
// pkg/worker/scan_helpers.go
package worker

import "github.com/shishobooks/shisho/pkg/models"

// shouldUpdateScalar determines if a scalar field should be updated based on priority rules.
// Returns true if the new value should replace the existing value.
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string) bool {
    // Empty existing source is treated as filepath priority (backwards compatibility)
    if existingSource == "" {
        existingSource = models.DataSourceFilepath
    }

    newPriority := models.DataSourcePriority[newSource]
    existingPriority := models.DataSourcePriority[existingSource]

    // Higher priority (lower number) wins if new value is non-empty
    if newPriority < existingPriority {
        return newValue != ""
    }

    // Same priority: prefer non-empty new value if different
    if newPriority == existingPriority {
        if newValue == "" {
            return false
        }
        return newValue != existingValue
    }

    // Lower priority never overwrites
    return false
}
```

### Step 4: Run test to verify it passes

Run: `go test -v ./pkg/worker/... -run TestShouldUpdateScalar`
Expected: PASS

### Step 5: Write the failing test for `shouldUpdateRelationship`

Add to `pkg/worker/scan_helpers_test.go`:

```go
func TestShouldUpdateRelationship(t *testing.T) {
    tests := []struct {
        name           string
        newItems       []string
        existingItems  []string
        newSource      string
        existingSource string
        want           bool
    }{
        {
            name:           "higher priority source with items updates",
            newItems:       []string{"Author A"},
            existingItems:  []string{"Author B"},
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceEPUBMetadata,
            want:           true,
        },
        {
            name:           "higher priority source with empty items does not update",
            newItems:       []string{},
            existingItems:  []string{"Author B"},
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "same priority with different items updates",
            newItems:       []string{"Author A", "Author B"},
            existingItems:  []string{"Author C"},
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           true,
        },
        {
            name:           "same priority with same items does not update",
            newItems:       []string{"Author A", "Author B"},
            existingItems:  []string{"Author A", "Author B"},
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "same priority with same items different order updates",
            newItems:       []string{"Author B", "Author A"},
            existingItems:  []string{"Author A", "Author B"},
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           true,
        },
        {
            name:           "same priority with empty new items does not update",
            newItems:       []string{},
            existingItems:  []string{"Author A"},
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "lower priority source never updates",
            newItems:       []string{"Author A"},
            existingItems:  []string{"Author B"},
            newSource:      models.DataSourceFilepath,
            existingSource: models.DataSourceEPUBMetadata,
            want:           false,
        },
        {
            name:           "manual source is never overwritten",
            newItems:       []string{"Author A"},
            existingItems:  []string{"Manual Author"},
            newSource:      models.DataSourceSidecar,
            existingSource: models.DataSourceManual,
            want:           false,
        },
        {
            name:           "empty existing source treated as filepath priority",
            newItems:       []string{"Author A"},
            existingItems:  []string{"Author B"},
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: "",
            want:           true,
        },
        {
            name:           "nil existing items with new items updates",
            newItems:       []string{"Author A"},
            existingItems:  nil,
            newSource:      models.DataSourceEPUBMetadata,
            existingSource: models.DataSourceFilepath,
            want:           true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := shouldUpdateRelationship(tt.newItems, tt.existingItems, tt.newSource, tt.existingSource)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Step 6: Run test to verify it fails

Run: `go test -v ./pkg/worker/... -run TestShouldUpdateRelationship`
Expected: FAIL with "undefined: shouldUpdateRelationship"

### Step 7: Write implementation for `shouldUpdateRelationship`

Add to `pkg/worker/scan_helpers.go`:

```go
// shouldUpdateRelationship determines if a relationship (authors, series, etc.) should be updated.
// Returns true if the new items should replace the existing items.
func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string) bool {
    // Empty existing source is treated as filepath priority (backwards compatibility)
    if existingSource == "" {
        existingSource = models.DataSourceFilepath
    }

    newPriority := models.DataSourcePriority[newSource]
    existingPriority := models.DataSourcePriority[existingSource]

    // Higher priority (lower number) wins if new items is non-empty
    if newPriority < existingPriority {
        return len(newItems) > 0
    }

    // Same priority: prefer non-empty new items if different
    if newPriority == existingPriority {
        if len(newItems) == 0 {
            return false
        }
        return !equalStringSlices(newItems, existingItems)
    }

    // Lower priority never overwrites
    return false
}

// equalStringSlices compares two string slices for equality (order matters).
func equalStringSlices(a, b []string) bool {
    if len(a) != len(b) {
        return false
    }
    for i := range a {
        if a[i] != b[i] {
            return false
        }
    }
    return true
}
```

### Step 8: Run test to verify it passes

Run: `go test -v ./pkg/worker/... -run TestShouldUpdateRelationship`
Expected: PASS

### Step 9: Run all helper tests together

Run: `go test -v ./pkg/worker/... -run "TestShouldUpdate"`
Expected: All PASS

### Step 10: Commit

```bash
git add pkg/worker/scan_helpers.go pkg/worker/scan_helpers_test.go
git commit -m "$(cat <<'EOF'
[Feature] Add priority-based update helper functions for rescan

Add shouldUpdateScalar() and shouldUpdateRelationship() to determine
when metadata fields should be updated based on source priority.
EOF
)"
```

---

## Task 2: Remove Early Return for Existing Files

**Files:**
- Modify: `pkg/worker/scan.go:346-353`

### Step 1: Understand current structure

The current code at lines 346-353 returns early when a file exists. We need to:
1. Store `existingFile` for later use
2. Continue with metadata extraction
3. Later, use the helper functions to compare and update

### Step 2: Remove the early return

Change `pkg/worker/scan.go` from:

```go
if existingFile != nil {
    jobLog.Info("file already exists", logger.Data{"file_id": existingFile.ID})
    // Check if cover is missing and recover it if needed
    if err := w.recoverMissingCover(ctx, existingFile); err != nil {
        jobLog.Warn("failed to recover missing cover", logger.Data{"file_id": existingFile.ID, "error": err.Error()})
    }
    return nil
}
```

To:

```go
if existingFile != nil {
    jobLog.Info("file already exists, will check for metadata updates", logger.Data{"file_id": existingFile.ID})
    // Check if cover is missing and recover it if needed
    if err := w.recoverMissingCover(ctx, existingFile); err != nil {
        jobLog.Warn("failed to recover missing cover", logger.Data{"file_id": existingFile.ID, "error": err.Error()})
    }
    // Continue to metadata extraction - we'll compare and update later
}
```

### Step 3: Run existing tests to verify no regression

Run: `make test`
Expected: All tests pass

### Step 4: Commit

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Refactor] Remove early return for existing files in scan

Allow metadata extraction to proceed for files that already exist in
the database. This is the first step toward re-scanning metadata.
EOF
)"
```

---

## Task 3: Add Update Logic for Existing Files with Existing Books

**Files:**
- Modify: `pkg/worker/scan.go` (around line 694-880, the `if existingBook != nil` block)

### Step 1: Understand current update logic

The existing code at lines 694-880 already handles updates when a **new file** is added to an **existing book**. It uses priority comparisons like:

```go
if strings.TrimSpace(title) != "" && models.DataSourcePriority[titleSource] < models.DataSourcePriority[existingBook.TitleSource] && existingBook.Title != title {
```

This only updates when the new source has **strictly higher** priority (`<`). For rescan, we need to also update when priorities are **equal** and values differ.

### Step 2: Refactor to use helper functions

Replace the existing priority checks with calls to `shouldUpdateScalar` and `shouldUpdateRelationship`.

**For title (example pattern):**

Change from:
```go
// Update title only if the new title is non-empty and from a higher priority source
if strings.TrimSpace(title) != "" && models.DataSourcePriority[titleSource] < models.DataSourcePriority[existingBook.TitleSource] && existingBook.Title != title {
    jobLog.Info("updating title", logger.Data{"new_title": title, "old_title": existingBook.Title})
    existingBook.Title = title
    existingBook.TitleSource = titleSource
    updateOptions.Columns = append(updateOptions.Columns, "title", "title_source")
    metadataChanged = true
}
```

To:
```go
// Update title based on priority rules
if shouldUpdateScalar(title, existingBook.Title, titleSource, existingBook.TitleSource) {
    jobLog.Info("updating title", logger.Data{"new_title": title, "old_title": existingBook.Title})
    existingBook.Title = title
    existingBook.TitleSource = titleSource
    updateOptions.Columns = append(updateOptions.Columns, "title", "title_source")
    metadataChanged = true
}
```

**For subtitle:**

Change from:
```go
existingSubtitleSource := models.DataSourceFilepath
if existingBook.SubtitleSource != nil {
    existingSubtitleSource = *existingBook.SubtitleSource
}
if subtitle != nil && *subtitle != "" && models.DataSourcePriority[subtitleSource] < models.DataSourcePriority[existingSubtitleSource] {
```

To:
```go
existingSubtitleSource := ""
if existingBook.SubtitleSource != nil {
    existingSubtitleSource = *existingBook.SubtitleSource
}
existingSubtitle := ""
if existingBook.Subtitle != nil {
    existingSubtitle = *existingBook.Subtitle
}
newSubtitle := ""
if subtitle != nil {
    newSubtitle = *subtitle
}
if shouldUpdateScalar(newSubtitle, existingSubtitle, subtitleSource, existingSubtitleSource) {
```

**For description:**

Change from:
```go
existingDescriptionSource := models.DataSourceFilepath
if existingBook.DescriptionSource != nil {
    existingDescriptionSource = *existingBook.DescriptionSource
}
if description != nil && *description != "" && models.DataSourcePriority[descriptionSource] < models.DataSourcePriority[existingDescriptionSource] {
```

To:
```go
existingDescriptionSource := ""
if existingBook.DescriptionSource != nil {
    existingDescriptionSource = *existingBook.DescriptionSource
}
existingDescription := ""
if existingBook.Description != nil {
    existingDescription = *existingBook.Description
}
newDescription := ""
if description != nil {
    newDescription = *description
}
if shouldUpdateScalar(newDescription, existingDescription, descriptionSource, existingDescriptionSource) {
```

**For authors:**

Change from:
```go
if len(authors) > 0 && models.DataSourcePriority[authorSource] < models.DataSourcePriority[existingBook.AuthorSource] {
```

To:
```go
// Extract existing author names for comparison
existingAuthorNames := make([]string, len(existingBook.Authors))
for i, a := range existingBook.Authors {
    if a.Person != nil {
        existingAuthorNames[i] = a.Person.Name
    }
}
newAuthorNames := make([]string, len(authors))
for i, a := range authors {
    newAuthorNames[i] = a.Name
}
if shouldUpdateRelationship(newAuthorNames, existingAuthorNames, authorSource, existingBook.AuthorSource) {
```

**For series:**

Change from:
```go
var existingSeriesSource string
if len(existingBook.BookSeries) > 0 && existingBook.BookSeries[0].Series != nil {
    existingSeriesSource = existingBook.BookSeries[0].Series.NameSource
}
if len(seriesList) > 0 && (len(existingBook.BookSeries) == 0 || models.DataSourcePriority[seriesSource] < models.DataSourcePriority[existingSeriesSource]) {
```

To:
```go
var existingSeriesSource string
if len(existingBook.BookSeries) > 0 && existingBook.BookSeries[0].Series != nil {
    existingSeriesSource = existingBook.BookSeries[0].Series.NameSource
}
existingSeriesNames := make([]string, len(existingBook.BookSeries))
for i, bs := range existingBook.BookSeries {
    if bs.Series != nil {
        existingSeriesNames[i] = bs.Series.Name
    }
}
newSeriesNames := make([]string, len(seriesList))
for i, s := range seriesList {
    newSeriesNames[i] = s.name
}
if shouldUpdateRelationship(newSeriesNames, existingSeriesNames, seriesSource, existingSeriesSource) {
```

**For genres:**

Change from:
```go
var existingGenreSource string
if existingBook.GenreSource != nil {
    existingGenreSource = *existingBook.GenreSource
}
if len(genreNames) > 0 && (len(existingBook.BookGenres) == 0 || models.DataSourcePriority[genreSource] < models.DataSourcePriority[existingGenreSource]) {
```

To:
```go
existingGenreSource := ""
if existingBook.GenreSource != nil {
    existingGenreSource = *existingBook.GenreSource
}
existingGenreNames := make([]string, len(existingBook.BookGenres))
for i, bg := range existingBook.BookGenres {
    if bg.Genre != nil {
        existingGenreNames[i] = bg.Genre.Name
    }
}
if shouldUpdateRelationship(genreNames, existingGenreNames, genreSource, existingGenreSource) {
```

**For tags:**

Change from:
```go
var existingTagSource string
if existingBook.TagSource != nil {
    existingTagSource = *existingBook.TagSource
}
if len(tagNames) > 0 && (len(existingBook.BookTags) == 0 || models.DataSourcePriority[tagSource] < models.DataSourcePriority[existingTagSource]) {
```

To:
```go
existingTagSource := ""
if existingBook.TagSource != nil {
    existingTagSource = *existingBook.TagSource
}
existingTagNames := make([]string, len(existingBook.BookTags))
for i, bt := range existingBook.BookTags {
    if bt.Tag != nil {
        existingTagNames[i] = bt.Tag.Name
    }
}
if shouldUpdateRelationship(tagNames, existingTagNames, tagSource, existingTagSource) {
```

### Step 3: Run tests

Run: `make test`
Expected: All tests pass

### Step 4: Commit

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Feature] Use priority helpers for book metadata updates

Refactor existing book update logic to use shouldUpdateScalar() and
shouldUpdateRelationship() helpers. This enables same-priority updates
where values differ, supporting the rescan metadata feature.
EOF
)"
```

---

## Task 4: Add Update Logic for Existing Files (File-Level Metadata)

**Files:**
- Modify: `pkg/worker/scan.go` (add new section after existing file check)

### Step 1: Understand current flow

Currently, when `existingFile != nil`, we only recover missing covers. We need to add logic to:
1. Compare file-level metadata (narrators, identifiers, publisher, imprint, URL, release date)
2. Update if priority rules allow

### Step 2: Add file-level update logic

After the early return removal (around line 353), add a new block that handles file metadata updates when `existingFile != nil`. This should be placed before the book creation/update logic.

Add after the cover recovery block (before line 355):

```go
// If file already exists, check for metadata updates at file level
if existingFile != nil {
    fileUpdated := false
    fileUpdateColumns := make([]string, 0)

    // Update narrators
    existingNarratorSource := ""
    if existingFile.NarratorSource != nil {
        existingNarratorSource = *existingFile.NarratorSource
    }
    existingNarratorNames := make([]string, len(existingFile.Narrators))
    for i, n := range existingFile.Narrators {
        if n.Person != nil {
            existingNarratorNames[i] = n.Person.Name
        }
    }
    if shouldUpdateRelationship(narratorNames, existingNarratorNames, narratorSource, existingNarratorSource) {
        jobLog.Info("updating narrators", logger.Data{"new_count": len(narratorNames), "old_count": len(existingNarratorNames)})
        // Delete existing narrators
        if err := w.bookService.DeleteFileNarrators(ctx, existingFile.ID); err != nil {
            return errors.WithStack(err)
        }
        // Create new narrators
        for i, name := range narratorNames {
            person, err := w.personService.FindOrCreatePerson(ctx, name, libraryID)
            if err != nil {
                jobLog.Error("failed to find/create narrator person", nil, logger.Data{"narrator": name, "error": err.Error()})
                continue
            }
            narrator := &models.Narrator{
                FileID:    existingFile.ID,
                PersonID:  person.ID,
                SortOrder: i + 1,
            }
            if err := w.bookService.CreateNarrator(ctx, narrator); err != nil {
                jobLog.Error("failed to create narrator", nil, logger.Data{"file_id": existingFile.ID, "person_id": person.ID, "error": err.Error()})
            }
        }
        existingFile.NarratorSource = &narratorSource
        fileUpdateColumns = append(fileUpdateColumns, "narrator_source")
        fileUpdated = true
    }

    // Update identifiers
    existingIdentifierSource := ""
    if existingFile.IdentifierSource != nil {
        existingIdentifierSource = *existingFile.IdentifierSource
    }
    existingIdentifierValues := make([]string, len(existingFile.Identifiers))
    for i, id := range existingFile.Identifiers {
        existingIdentifierValues[i] = id.Type + ":" + id.Value
    }
    newIdentifierValues := make([]string, len(identifiers))
    for i, id := range identifiers {
        newIdentifierValues[i] = id.Type + ":" + id.Value
    }
    if shouldUpdateRelationship(newIdentifierValues, existingIdentifierValues, identifierSource, existingIdentifierSource) {
        jobLog.Info("updating identifiers", logger.Data{"new_count": len(identifiers), "old_count": len(existingFile.Identifiers)})
        // Delete existing identifiers
        if err := w.bookService.DeleteFileIdentifiers(ctx, existingFile.ID); err != nil {
            return errors.WithStack(err)
        }
        // Create new identifiers
        for _, id := range identifiers {
            fileIdentifier := &models.FileIdentifier{
                FileID: existingFile.ID,
                Type:   id.Type,
                Value:  id.Value,
                Source: identifierSource,
            }
            if err := w.bookService.CreateFileIdentifier(ctx, fileIdentifier); err != nil {
                jobLog.Error("failed to create identifier", nil, logger.Data{"file_id": existingFile.ID, "type": id.Type, "error": err.Error()})
            }
        }
        existingFile.IdentifierSource = &identifierSource
        fileUpdateColumns = append(fileUpdateColumns, "identifier_source")
        fileUpdated = true
    }

    // Update URL
    existingURLSource := ""
    if existingFile.URLSource != nil {
        existingURLSource = *existingFile.URLSource
    }
    existingURL := ""
    if existingFile.URL != nil {
        existingURL = *existingFile.URL
    }
    newURL := ""
    if fileURL != nil {
        newURL = *fileURL
    }
    if shouldUpdateScalar(newURL, existingURL, fileURLSource, existingURLSource) {
        jobLog.Info("updating URL", logger.Data{"new_url": newURL})
        existingFile.URL = fileURL
        existingFile.URLSource = &fileURLSource
        fileUpdateColumns = append(fileUpdateColumns, "url", "url_source")
        fileUpdated = true
    }

    // Update publisher
    existingPublisherSource := ""
    if existingFile.PublisherSource != nil {
        existingPublisherSource = *existingFile.PublisherSource
    }
    existingPublisherName := ""
    if existingFile.Publisher != nil {
        existingPublisherName = existingFile.Publisher.Name
    }
    newPublisherName := ""
    if publisherName != nil {
        newPublisherName = *publisherName
    }
    if shouldUpdateScalar(newPublisherName, existingPublisherName, publisherSource, existingPublisherSource) {
        jobLog.Info("updating publisher", logger.Data{"new_publisher": newPublisherName})
        if newPublisherName != "" {
            publisher, err := w.publisherService.FindOrCreatePublisher(ctx, newPublisherName, libraryID)
            if err != nil {
                jobLog.Error("failed to find/create publisher", nil, logger.Data{"publisher": newPublisherName, "error": err.Error()})
            } else {
                existingFile.PublisherID = &publisher.ID
                existingFile.PublisherSource = &publisherSource
                fileUpdateColumns = append(fileUpdateColumns, "publisher_id", "publisher_source")
                fileUpdated = true
            }
        } else {
            existingFile.PublisherID = nil
            existingFile.PublisherSource = &publisherSource
            fileUpdateColumns = append(fileUpdateColumns, "publisher_id", "publisher_source")
            fileUpdated = true
        }
    }

    // Update imprint
    existingImprintSource := ""
    if existingFile.ImprintSource != nil {
        existingImprintSource = *existingFile.ImprintSource
    }
    existingImprintName := ""
    if existingFile.Imprint != nil {
        existingImprintName = existingFile.Imprint.Name
    }
    newImprintName := ""
    if imprintName != nil {
        newImprintName = *imprintName
    }
    if shouldUpdateScalar(newImprintName, existingImprintName, imprintSource, existingImprintSource) {
        jobLog.Info("updating imprint", logger.Data{"new_imprint": newImprintName})
        if newImprintName != "" {
            imprint, err := w.imprintService.FindOrCreateImprint(ctx, newImprintName, libraryID)
            if err != nil {
                jobLog.Error("failed to find/create imprint", nil, logger.Data{"imprint": newImprintName, "error": err.Error()})
            } else {
                existingFile.ImprintID = &imprint.ID
                existingFile.ImprintSource = &imprintSource
                fileUpdateColumns = append(fileUpdateColumns, "imprint_id", "imprint_source")
                fileUpdated = true
            }
        } else {
            existingFile.ImprintID = nil
            existingFile.ImprintSource = &imprintSource
            fileUpdateColumns = append(fileUpdateColumns, "imprint_id", "imprint_source")
            fileUpdated = true
        }
    }

    // Update release date
    existingReleaseDateSource := ""
    if existingFile.ReleaseDateSource != nil {
        existingReleaseDateSource = *existingFile.ReleaseDateSource
    }
    existingReleaseDate := ""
    if existingFile.ReleaseDate != nil {
        existingReleaseDate = existingFile.ReleaseDate.Format(time.RFC3339)
    }
    newReleaseDate := ""
    if releaseDate != nil {
        newReleaseDate = releaseDate.Format(time.RFC3339)
    }
    if shouldUpdateScalar(newReleaseDate, existingReleaseDate, releaseDateSource, existingReleaseDateSource) {
        jobLog.Info("updating release date", logger.Data{"new_release_date": newReleaseDate})
        existingFile.ReleaseDate = releaseDate
        existingFile.ReleaseDateSource = &releaseDateSource
        fileUpdateColumns = append(fileUpdateColumns, "release_date", "release_date_source")
        fileUpdated = true
    }

    // Update file size if changed (no source tracking, just update if different)
    if existingFile.FilesizeBytes != size {
        jobLog.Info("updating file size", logger.Data{"new_size": size, "old_size": existingFile.FilesizeBytes})
        existingFile.FilesizeBytes = size
        fileUpdateColumns = append(fileUpdateColumns, "filesize_bytes")
        fileUpdated = true
    }

    // Apply file updates if any changes
    if fileUpdated {
        if err := w.bookService.UpdateFile(ctx, existingFile, books.UpdateFileOptions{Columns: fileUpdateColumns}); err != nil {
            return errors.WithStack(err)
        }
    }

    // Now proceed to book-level updates
    // Get the existing book for this file
    existingBook, err := w.bookService.RetrieveBook(ctx, books.RetrieveBookOptions{
        ID: &existingFile.BookID,
    })
    if err != nil {
        return errors.WithStack(err)
    }

    // Continue with book-level metadata comparison and updates
    // (The existing book update logic starting at line 694 will handle this)
}
```

**Note:** This is a large code block. The actual implementation will need to integrate with the existing control flow carefully. The key insight is that we need to:
1. Check for file existence
2. Extract all metadata (already happens)
3. Compare and update file-level metadata
4. Then proceed to book-level metadata updates

### Step 3: Verify required service methods exist

Check that these methods exist on `bookService`:
- `DeleteFileNarrators(ctx, fileID)`
- `DeleteFileIdentifiers(ctx, fileID)`
- `UpdateFile(ctx, file, options)`

If they don't exist, they'll need to be added in a prerequisite task.

### Step 4: Run tests

Run: `make test`
Expected: All tests pass

### Step 5: Commit

```bash
git add pkg/worker/scan.go
git commit -m "$(cat <<'EOF'
[Feature] Add file-level metadata updates for existing files

When rescanning existing files, compare and update file-level metadata
(narrators, identifiers, publisher, imprint, URL, release date) based
on source priority rules.
EOF
)"
```

---

## Task 5: Verify Service Methods Exist (Prerequisite Check)

**Files:**
- Read: `pkg/books/service.go`

### Step 1: Check for required methods

Before implementing Task 4, verify these methods exist:

```go
DeleteFileNarrators(ctx context.Context, fileID int) error
DeleteFileIdentifiers(ctx context.Context, fileID int) error
UpdateFile(ctx context.Context, file *models.File, options UpdateFileOptions) error
```

### Step 2: If missing, create them

If `DeleteFileNarrators` is missing, add to `pkg/books/service.go`:

```go
// DeleteFileNarrators deletes all narrators for a file.
func (s *Service) DeleteFileNarrators(ctx context.Context, fileID int) error {
    _, err := s.db.NewDelete().
        Model((*models.Narrator)(nil)).
        Where("file_id = ?", fileID).
        Exec(ctx)
    return errors.WithStack(err)
}
```

If `DeleteFileIdentifiers` is missing, add:

```go
// DeleteFileIdentifiers deletes all identifiers for a file.
func (s *Service) DeleteFileIdentifiers(ctx context.Context, fileID int) error {
    _, err := s.db.NewDelete().
        Model((*models.FileIdentifier)(nil)).
        Where("file_id = ?", fileID).
        Exec(ctx)
    return errors.WithStack(err)
}
```

If `UpdateFile` is missing or doesn't support column selection, ensure it does:

```go
// UpdateFileOptions contains options for updating a file.
type UpdateFileOptions struct {
    Columns []string
}

// UpdateFile updates a file with the specified columns.
func (s *Service) UpdateFile(ctx context.Context, file *models.File, options UpdateFileOptions) error {
    query := s.db.NewUpdate().Model(file).WherePK()
    if len(options.Columns) > 0 {
        query = query.Column(options.Columns...)
    }
    _, err := query.Exec(ctx)
    return errors.WithStack(err)
}
```

### Step 3: Commit if changes made

```bash
git add pkg/books/service.go
git commit -m "$(cat <<'EOF'
[Feature] Add file-level delete and update methods for rescan

Add DeleteFileNarrators, DeleteFileIdentifiers, and UpdateFile methods
to support updating file-level metadata during rescan.
EOF
)"
```

---

## Task 6: Integration Test - Idempotent Rescan

**Files:**
- Create: `pkg/worker/scan_rescan_test.go`

### Step 1: Write integration test for idempotency

```go
// pkg/worker/scan_rescan_test.go
package worker

import (
    "context"
    "testing"
    "time"

    "github.com/shishobooks/shisho/pkg/models"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestScanFile_ExistingFile_NoChanges_Idempotent(t *testing.T) {
    // This test verifies that rescanning an existing file with unchanged metadata
    // does not result in any database writes (idempotency).

    // Setup: Create a test library and file
    // ... (test setup code using test helpers)

    // First scan: Create the file
    // ...

    // Record the UpdatedAt timestamp
    // ...

    // Wait a moment to ensure timestamps would differ if updated
    time.Sleep(10 * time.Millisecond)

    // Second scan: Rescan the same file
    // ...

    // Verify: UpdatedAt should not have changed
    // ...
}

func TestScanFile_ExistingFile_MetadataChanged_Updates(t *testing.T) {
    // This test verifies that when file metadata changes (e.g., EPUB updated),
    // the rescan picks up the new metadata.

    // Setup: Create a test library and file
    // ...

    // First scan: Create the file with initial metadata
    // ...

    // Modify the test file to have different metadata
    // (or use a sidecar file to inject new metadata)
    // ...

    // Second scan: Rescan the file
    // ...

    // Verify: Metadata should be updated
    // ...
}

func TestScanFile_ExistingFile_ManualEdit_Preserved(t *testing.T) {
    // This test verifies that manual edits (priority 0) are preserved
    // through rescan even when file metadata changes.

    // Setup: Create a test library and file
    // ...

    // First scan: Create the file
    // ...

    // Manual edit: Update title via API (sets source to "manual")
    // ...

    // Second scan: Rescan with different title in file metadata
    // ...

    // Verify: Manual title should be preserved
    // ...
}
```

### Step 2: Run test to verify it fails initially

Run: `go test -v ./pkg/worker/... -run TestScanFile_ExistingFile`
Expected: Tests should guide implementation

### Step 3: Implement tests fully after Tasks 2-4

The actual test implementation depends on having the feature complete. Write detailed tests after the core implementation.

### Step 4: Run tests and iterate

Run: `make test`
Expected: All tests pass

### Step 5: Commit

```bash
git add pkg/worker/scan_rescan_test.go
git commit -m "$(cat <<'EOF'
[Test] Add integration tests for rescan metadata feature

Test idempotency (no DB writes when unchanged), metadata updates when
file content changes, and preservation of manual edits through rescan.
EOF
)"
```

---

## Task 7: Handle Edge Case - Empty Source Field in DB

**Files:**
- Modify: `pkg/worker/scan_helpers.go`
- Test: `pkg/worker/scan_helpers_test.go`

### Step 1: Already handled in helper functions

The helper functions already handle empty source fields by treating them as `filepath` priority:

```go
// Empty existing source is treated as filepath priority (backwards compatibility)
if existingSource == "" {
    existingSource = models.DataSourceFilepath
}
```

### Step 2: Add explicit test case

Verify the test case exists in `TestShouldUpdateScalar`:

```go
{
    name:           "empty existing source treated as filepath priority",
    newValue:       "New Title",
    existingValue:  "Old Title",
    newSource:      models.DataSourceEPUBMetadata,
    existingSource: "",
    want:           true,
},
```

### Step 3: Run tests

Run: `go test -v ./pkg/worker/... -run TestShouldUpdateScalar`
Expected: PASS

---

## Task 8: Final Verification and Cleanup

### Step 1: Run full test suite

Run: `make check`
Expected: All tests pass, no lint errors

### Step 2: Manual testing

1. Start the dev server: `make start`
2. Add a library with test files
3. Trigger initial scan
4. Verify files are indexed
5. Modify a file's metadata (e.g., edit EPUB OPF)
6. Trigger rescan
7. Verify metadata is updated
8. Verify manual edits are preserved

### Step 3: Final commit

```bash
git add -A
git commit -m "$(cat <<'EOF'
[Feature] Complete rescan metadata implementation

Files are now re-scanned on every sync. Metadata is updated based on
source priority: manual > sidecar > file_metadata > filepath. If
nothing changes, the scan is idempotent (no DB writes).
EOF
)"
```

---

## Summary of Changes

| File | Change |
|------|--------|
| `pkg/worker/scan_helpers.go` | New - priority comparison helpers |
| `pkg/worker/scan_helpers_test.go` | New - unit tests for helpers |
| `pkg/worker/scan.go` | Modified - remove early return, use helpers |
| `pkg/worker/scan_rescan_test.go` | New - integration tests |
| `pkg/books/service.go` | Modified - add missing methods if needed |

## Testing Checklist

- [ ] `shouldUpdateScalar` handles all priority combinations
- [ ] `shouldUpdateRelationship` handles all priority combinations
- [ ] Empty source fields treated as filepath priority
- [ ] Existing file with no metadata changes = no DB writes
- [ ] Existing file with same-priority different value = updates
- [ ] Existing file with lower-priority source = no update
- [ ] Existing file with higher-priority source = updates
- [ ] Manual edits (priority 0) preserved through rescan
- [ ] File-level metadata (narrators, identifiers, etc.) updated correctly
- [ ] Book-level metadata (authors, series, genres, tags) updated correctly
