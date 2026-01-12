# Metadata Fields Implementation Plan

> **Status:** ✅ COMPLETED (2026-01-11)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add description to books, and URL/publisher/imprint/release_date to files with full parser/generator support.

## Implementation Notes

**Additional steps discovered during implementation (not in original plan):**

1. **API Relations** - When adding relations to File model, update `pkg/books/service.go` to load them via `.Relation("Files.Publisher")` and `.Relation("Files.Imprint")` in all book query methods.

2. **UI Display** - Update `app/components/pages/BookDetail.tsx` to display new fields.

3. **Entity Services** - Created `pkg/publishers/service.go` and `pkg/imprints/service.go` following the pattern from `pkg/genres/service.go` with `FindOrCreate` methods.

4. **Worker Integration** - Added publisher and imprint services to `pkg/worker/worker.go` and initialized in `New()`.

5. **Sidecar FromModel** - Updated `pkg/sidecar/sidecar.go` `BookSidecarFromModel()` and `FileSidecarFromModel()` to include new fields.

These additional steps have been documented in `CLAUDE.md` under the expanded "Metadata Sync Checklist" section.

---

**Architecture:** Modify existing migration (pre-production), add new Publisher/Imprint entities with one-to-many relationships to files, extend all three file parsers (EPUB/CBZ/M4B) and generators.

**Tech Stack:** Go, Bun ORM, SQLite, TypeScript (auto-generated types via tygo)

---

## Task 1: Database Migration Changes

**Files:**
- Modify: `pkg/migrations/20250321211048_create_initial_tables.go`

**Step 1: Add publishers table**

Add after the tags table creation (around line 343):

```go
// Publishers (normalized, case-insensitive per library)
_, err = db.Exec(`
	CREATE TABLE publishers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		library_id INTEGER REFERENCES libraries (id) ON DELETE CASCADE NOT NULL,
		name TEXT NOT NULL
	)
`)
if err != nil {
	return errors.WithStack(err)
}
_, err = db.Exec(`CREATE UNIQUE INDEX ux_publishers_name_library_id ON publishers (name COLLATE NOCASE, library_id)`)
if err != nil {
	return errors.WithStack(err)
}
_, err = db.Exec(`CREATE INDEX ix_publishers_library_id ON publishers (library_id)`)
if err != nil {
	return errors.WithStack(err)
}

// Imprints (normalized, case-insensitive per library)
_, err = db.Exec(`
	CREATE TABLE imprints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		library_id INTEGER REFERENCES libraries (id) ON DELETE CASCADE NOT NULL,
		name TEXT NOT NULL
	)
`)
if err != nil {
	return errors.WithStack(err)
}
_, err = db.Exec(`CREATE UNIQUE INDEX ux_imprints_name_library_id ON imprints (name COLLATE NOCASE, library_id)`)
if err != nil {
	return errors.WithStack(err)
}
_, err = db.Exec(`CREATE INDEX ix_imprints_library_id ON imprints (library_id)`)
if err != nil {
	return errors.WithStack(err)
}
```

**Step 2: Modify books table**

Change the books table creation to add description fields:

```go
_, err = db.Exec(`
	CREATE TABLE books (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		library_id INTEGER REFERENCES libraries (id) NOT NULL,
		filepath TEXT NOT NULL,
		title TEXT NOT NULL,
		title_source TEXT NOT NULL,
		sort_title TEXT NOT NULL,
		sort_title_source TEXT NOT NULL,
		subtitle TEXT,
		subtitle_source TEXT,
		description TEXT,
		description_source TEXT,
		author_source TEXT NOT NULL,
		genre_source TEXT,
		tag_source TEXT
	)
`)
```

**Step 3: Modify files table**

Change the files table creation to add new fields:

```go
_, err = db.Exec(`
	CREATE TABLE files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		library_id TEXT REFERENCES libraries (id) NOT NULL,
		book_id TEXT REFERENCES books (id) NOT NULL,
		filepath TEXT NOT NULL,
		file_type TEXT NOT NULL,
		filesize_bytes INTEGER NOT NULL,
		cover_image_path TEXT,
		cover_mime_type TEXT,
		cover_source TEXT,
		cover_page INTEGER,
		page_count INTEGER,
		audiobook_duration_seconds DOUBLE,
		audiobook_bitrate_bps INTEGER,
		narrator_source TEXT,
		url TEXT,
		url_source TEXT,
		release_date DATE,
		release_date_source TEXT,
		publisher_id INTEGER REFERENCES publishers (id),
		publisher_source TEXT,
		imprint_id INTEGER REFERENCES imprints (id),
		imprint_source TEXT
	)
`)
```

**Step 4: Add down migration for new tables**

In the down function, add drops for publishers and imprints (before dropping files):

```go
_, err = db.Exec("DROP TABLE IF EXISTS imprints")
if err != nil {
	return errors.WithStack(err)
}
_, err = db.Exec("DROP TABLE IF EXISTS publishers")
if err != nil {
	return errors.WithStack(err)
}
```

**Step 5: Run migration to verify syntax**

Run: `make db:rollback && make db:migrate`
Expected: Migration runs without errors

**Step 6: Commit**

```bash
git add pkg/migrations/
git commit -m "[DB] Add description to books, URL/publisher/imprint/release_date to files"
```

---

## Task 2: Add Publisher and Imprint Models

**Files:**
- Create: `pkg/models/publisher.go`
- Create: `pkg/models/imprint.go`

**Step 1: Create Publisher model**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Publisher struct {
	bun.BaseModel `bun:"table:publishers,alias:pub" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Name      string    `bun:",nullzero" json:"name"`
	FileCount int       `bun:",scanonly" json:"file_count"`
}
```

**Step 2: Create Imprint model**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Imprint struct {
	bun.BaseModel `bun:"table:imprints,alias:imp" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Name      string    `bun:",nullzero" json:"name"`
	FileCount int       `bun:",scanonly" json:"file_count"`
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/models/publisher.go pkg/models/imprint.go
git commit -m "[Models] Add Publisher and Imprint models"
```

---

## Task 3: Update Book Model

**Files:**
- Modify: `pkg/models/book.go`

**Step 1: Add Description fields to Book struct**

Add after SubtitleSource:

```go
Description       *string       `json:"description"`
DescriptionSource *string       `json:"description_source" tstype:"DataSource"`
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add pkg/models/book.go
git commit -m "[Models] Add description fields to Book"
```

---

## Task 4: Update File Model

**Files:**
- Modify: `pkg/models/file.go`

**Step 1: Add new fields to File struct**

Add after NarratorSource:

```go
URL               *string    `json:"url"`
URLSource         *string    `json:"url_source" tstype:"DataSource"`
ReleaseDate       *time.Time `json:"release_date"`
ReleaseDateSource *string    `json:"release_date_source" tstype:"DataSource"`
PublisherID       *int       `json:"publisher_id"`
PublisherSource   *string    `json:"publisher_source" tstype:"DataSource"`
Publisher         *Publisher `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty" tstype:"Publisher"`
ImprintID         *int       `json:"imprint_id"`
ImprintSource     *string    `json:"imprint_source" tstype:"DataSource"`
Imprint           *Imprint   `bun:"rel:belongs-to,join:imprint_id=id" json:"imprint,omitempty" tstype:"Imprint"`
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add pkg/models/file.go
git commit -m "[Models] Add URL/publisher/imprint/release_date fields to File"
```

---

## Task 5: Update ParsedMetadata

**Files:**
- Modify: `pkg/mediafile/mediafile.go`

**Step 1: Add new fields to ParsedMetadata struct**

Add after existing fields:

```go
Description string
Publisher   string
Imprint     string
URL         string
ReleaseDate *time.Time
```

**Step 2: Add time import if not present**

Ensure `"time"` is in the imports.

**Step 3: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/mediafile/mediafile.go
git commit -m "[Mediafile] Add description/publisher/imprint/URL/release_date to ParsedMetadata"
```

---

## Task 6: Update EPUB Parser

**Files:**
- Modify: `pkg/epub/opf.go`

**Step 1: Add fields to OPF struct**

Add after Tags:

```go
Description string
Publisher   string
Imprint     string
URL         string
ReleaseDate *time.Time
```

**Step 2: Update ParseOPF to extract new fields**

After parsing tags (around line 272), add:

```go
// Extract description
description := pkg.Metadata.Description

// Extract publisher
publisher := pkg.Metadata.Publisher

// Extract release date from dc:date
var releaseDate *time.Time
if pkg.Metadata.Date != "" {
	// Try various date formats
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, pkg.Metadata.Date); err == nil {
			releaseDate = &t
			break
		}
	}
}

// Extract imprint from meta tags
var imprint string
for _, m := range pkg.Metadata.Meta {
	if m.Property == "ibooks:imprint" || m.Name == "imprint" {
		imprint = m.Text
		if imprint == "" {
			imprint = m.Content
		}
		break
	}
}

// Extract URL from dc:relation or dc:source (if URL-like)
var url string
// Check dc:relation first (not currently in Package struct - need to add)
// For now, check meta tags for URL-like content
```

**Step 3: Add Relation field to Package Metadata struct**

In the Metadata struct inside Package, add:

```go
Relation []string `xml:"relation"`
Source   []string `xml:"source"`
```

**Step 4: Update URL extraction**

```go
// Extract URL from dc:relation or dc:source
var url string
for _, rel := range pkg.Metadata.Relation {
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		url = rel
		break
	}
}
if url == "" {
	for _, src := range pkg.Metadata.Source {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			url = src
			break
		}
	}
}
```

**Step 5: Update OPF return struct**

```go
return &OPF{
	Title:         title,
	Authors:       authors,
	Series:        series,
	SeriesNumber:  seriesNumber,
	Genres:        genres,
	Tags:          tags,
	Description:   description,
	Publisher:     publisher,
	Imprint:       imprint,
	URL:           url,
	ReleaseDate:   releaseDate,
	CoverFilepath: coverFilepath,
	CoverMimeType: coverMimeType,
}, nil
```

**Step 6: Update Parse return**

In the Parse function, update the return statement:

```go
return &mediafile.ParsedMetadata{
	Title:         opf.Title,
	Authors:       opf.Authors,
	Series:        opf.Series,
	SeriesNumber:  opf.SeriesNumber,
	Genres:        opf.Genres,
	Tags:          opf.Tags,
	Description:   opf.Description,
	Publisher:     opf.Publisher,
	Imprint:       opf.Imprint,
	URL:           opf.URL,
	ReleaseDate:   opf.ReleaseDate,
	CoverMimeType: opf.CoverMimeType,
	CoverData:     opf.CoverData,
	DataSource:    models.DataSourceEPUBMetadata,
}, nil
```

**Step 7: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 8: Commit**

```bash
git add pkg/epub/opf.go
git commit -m "[EPUB] Extract description/publisher/imprint/URL/release_date"
```

---

## Task 7: Update CBZ Parser

**Files:**
- Modify: `pkg/cbz/cbz.go`

**Step 1: Add Summary and Web to ComicInfo struct**

The struct already has Publisher and Imprint. Add:

```go
Summary string `xml:"Summary"`
Web     string `xml:"Web"`
```

**Step 2: Update Parse to extract and return new fields**

After extracting genres and tags, add:

```go
// Extract description from Summary
var description string
if comicInfo != nil && comicInfo.Summary != "" {
	description = comicInfo.Summary
}

// Extract URL from Web
var url string
if comicInfo != nil && comicInfo.Web != "" {
	url = comicInfo.Web
}

// Extract publisher
var publisher string
if comicInfo != nil && comicInfo.Publisher != "" {
	publisher = comicInfo.Publisher
}

// Extract imprint
var imprint string
if comicInfo != nil && comicInfo.Imprint != "" {
	imprint = comicInfo.Imprint
}

// Extract release date from Year/Month/Day
var releaseDate *time.Time
if comicInfo != nil && comicInfo.Year != "" {
	year, err := strconv.Atoi(comicInfo.Year)
	if err == nil {
		month := 1
		day := 1
		if comicInfo.Month != "" {
			if m, err := strconv.Atoi(comicInfo.Month); err == nil && m >= 1 && m <= 12 {
				month = m
			}
		}
		if comicInfo.Day != "" {
			if d, err := strconv.Atoi(comicInfo.Day); err == nil && d >= 1 && d <= 31 {
				day = d
			}
		}
		t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		releaseDate = &t
	}
}
```

**Step 3: Update the return statement**

```go
return &mediafile.ParsedMetadata{
	Title:         title,
	Authors:       authors,
	Series:        series,
	SeriesNumber:  seriesNumber,
	Genres:        genres,
	Tags:          tags,
	Description:   description,
	Publisher:     publisher,
	Imprint:       imprint,
	URL:           url,
	ReleaseDate:   releaseDate,
	CoverMimeType: coverMimeType,
	CoverData:     coverData,
	CoverPage:     coverPage,
	PageCount:     pageCount,
	DataSource:    models.DataSourceCBZMetadata,
}, nil
```

**Step 4: Add time import**

Add `"time"` to imports.

**Step 5: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add pkg/cbz/cbz.go
git commit -m "[CBZ] Extract description/publisher/imprint/URL/release_date"
```

---

## Task 8: Update M4B Parser - Add Atoms

**Files:**
- Modify: `pkg/mp4/atoms.go`

**Step 1: Add Publisher atom constant**

Add after AtomEncoder:

```go
AtomPublisher = [4]byte{0xA9, 'p', 'u', 'b'} // ©pub - Publisher
```

**Step 2: Add release date atom constant**

```go
AtomReleaseDate = [4]byte{'r', 'l', 'd', 't'} // rldt - Release date (Audible)
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/mp4/atoms.go
git commit -m "[MP4] Add Publisher and ReleaseDate atom constants"
```

---

## Task 9: Update M4B Parser - Reader

**Files:**
- Modify: `pkg/mp4/reader.go`

**Step 1: Add publisher and releaseDate to rawMetadata struct**

```go
publisher   string // from ©pub
releaseDate string // from rldt (Audible release date)
```

**Step 2: Update isMetadataAtom function**

Add:

```go
atomTypeEquals(boxType, AtomPublisher) ||
atomTypeEquals(boxType, AtomReleaseDate) ||
```

**Step 3: Update processMetadataAtom function**

Add cases:

```go
case atomTypeEquals(boxType, AtomPublisher):
	meta.publisher = parseTextData(child.data)

case atomTypeEquals(boxType, AtomReleaseDate):
	meta.releaseDate = parseTextData(child.data)
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 5: Commit**

```bash
git add pkg/mp4/reader.go
git commit -m "[MP4] Parse ©pub and rldt atoms"
```

---

## Task 10: Update M4B Parser - Metadata Conversion

**Files:**
- Modify: `pkg/mp4/metadata.go`

**Step 1: Add Publisher and ReleaseDate to Metadata struct**

Add after Description:

```go
Publisher   string     // from ©pub
Imprint     string     // from com.shisho:imprint freeform
URL         string     // from com.shisho:url freeform
ReleaseDate *time.Time // parsed from rldt or ©day
```

**Step 2: Update convertRawMetadata**

Add after setting Description:

```go
// Set publisher
meta.Publisher = raw.publisher

// Extract imprint from freeform
if imp, ok := raw.freeform["com.shisho:imprint"]; ok {
	meta.Imprint = imp
}

// Extract URL from freeform
if url, ok := raw.freeform["com.shisho:url"]; ok {
	meta.URL = url
}

// Parse release date - prefer rldt, fall back to ©day
var releaseDate *time.Time
dateStr := raw.releaseDate
if dateStr == "" {
	dateStr = raw.year
}
if dateStr != "" {
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			releaseDate = &t
			break
		}
	}
}
meta.ReleaseDate = releaseDate
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add pkg/mp4/metadata.go
git commit -m "[MP4] Add Publisher/Imprint/URL/ReleaseDate to Metadata"
```

---

## Task 11: Update M4B Parser - Main Parse Function

**Files:**
- Modify: `pkg/mp4/mp4.go`

**Step 1: Update Parse return to include new fields**

```go
return &mediafile.ParsedMetadata{
	Title:         meta.Title,
	Subtitle:      meta.Subtitle,
	Authors:       meta.Authors,
	Narrators:     meta.Narrators,
	Series:        meta.Series,
	SeriesNumber:  meta.SeriesNumber,
	Genres:        meta.Genres,
	Tags:          meta.Tags,
	Description:   meta.Description,
	Publisher:     meta.Publisher,
	Imprint:       meta.Imprint,
	URL:           meta.URL,
	ReleaseDate:   meta.ReleaseDate,
	CoverMimeType: meta.CoverMimeType,
	CoverData:     meta.CoverData,
	DataSource:    models.DataSourceM4BMetadata,
	Duration:      meta.Duration,
	BitrateBps:    meta.Bitrate,
}, nil
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: Build succeeds

**Step 3: Commit**

```bash
git add pkg/mp4/mp4.go
git commit -m "[MP4] Return new metadata fields from Parse"
```

---

## Task 12: Run Tests and Generate Types

**Step 1: Run all tests**

Run: `make test`
Expected: All tests pass

**Step 2: Generate TypeScript types**

Run: `make tygo`
Expected: Types regenerated

**Step 3: Run full checks**

Run: `make check`
Expected: All checks pass

**Step 4: Commit generated types**

```bash
git add app/types/generated/
git commit -m "[Types] Regenerate TypeScript types for new metadata fields"
```

---

## Remaining Tasks (outlined)

The following tasks follow similar patterns and should be implemented after the core model/parser work:

### Task 13: Update Sidecar Types
- Modify: `pkg/sidecar/types.go`
- Add description to BookSidecar
- Add URL/publisher/imprint/release_date to file sidecar

### Task 14: Update Download Fingerprint
- Modify: `pkg/downloadcache/fingerprint.go`
- Add new fields to Fingerprint struct and ComputeFingerprint()

### Task 15: Update Scanner
- Modify: `pkg/worker/scan.go`
- Handle new fields during file scanning
- Create/lookup Publisher and Imprint entities
- Set source fields appropriately

### Task 16: Update EPUB Generator
- Modify: `pkg/filegen/epub.go`
- Write description, publisher, release_date
- Write imprint to both meta formats
- Write URL to dc:relation and dc:source

### Task 17: Update CBZ Generator
- Modify: `pkg/filegen/cbz.go`
- Write Summary, Publisher, Imprint, Web, Year/Month/Day

### Task 18: Update M4B Generator
- Modify: `pkg/filegen/m4b.go`
- Write ©pub atom
- Write ©day and rldt atoms
- Write com.shisho:imprint and com.shisho:url freeform atoms

### Task 19: Update KePub Generator
- Modify: `pkg/kepub/cbz.go`
- Ensure new fields are passed through during CBZ-to-KePub conversion

### Task 20: Final Testing
- Run full test suite
- Test with sample files
- Verify round-trip (parse → generate → parse)
