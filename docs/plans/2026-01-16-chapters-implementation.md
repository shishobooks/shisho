# Chapters Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add chapter parsing, storage, and API endpoints for CBZ, M4B, and EPUB files.

**Architecture:** Chapters are file-level metadata stored in a new `chapters` table with self-referential `parent_id` for EPUB nesting. Each file type has different position semantics: CBZ uses page numbers, M4B uses timestamps, EPUB uses hrefs. Parsing adds chapters during file sync; API provides read/write access.

**Tech Stack:** Go with Bun ORM, Echo web framework, SQLite. Existing mp4 chapter parsing reused; new parsing for CBZ and EPUB navigation documents.

---

## Task 1: Database Migration

**Files:**
- Create: `pkg/migrations/20260116000000_add_chapters.go`

**Step 1: Write the migration file**

```go
package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE chapters (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
				parent_id INTEGER REFERENCES chapters(id) ON DELETE CASCADE,
				sort_order INTEGER NOT NULL,
				title TEXT NOT NULL,
				start_page INTEGER,
				start_timestamp_ms INTEGER,
				href TEXT
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for fast lookup by file
		_, err = db.Exec(`CREATE INDEX ix_chapters_file_id ON chapters(file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Index for parent lookups (nested chapters)
		_, err = db.Exec(`CREATE INDEX ix_chapters_parent_id ON chapters(parent_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Unique constraint: sort_order is unique within siblings
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_chapters_file_sort ON chapters(file_id, parent_id, sort_order)`)
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS chapters")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
```

**Step 2: Run the migration**

Run: `make db:migrate`
Expected: Migration applies successfully, new `chapters` table created.

**Step 3: Verify migration with rollback test**

Run: `make db:rollback && make db:migrate`
Expected: Rollback succeeds, re-migration succeeds.

**Step 4: Commit**

```bash
git add pkg/migrations/20260116000000_add_chapters.go
git commit -m "$(cat <<'EOF'
[Database] Add chapters table migration

Adds chapters table with:
- file_id FK with cascade delete
- parent_id self-reference for EPUB nesting
- Position fields: start_page (CBZ), start_timestamp_ms (M4B), href (EPUB)
- Unique constraint on (file_id, parent_id, sort_order)
EOF
)"
```

---

## Task 2: Chapter Model

**Files:**
- Create: `pkg/models/chapter.go`

**Step 1: Write the Chapter model**

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Chapter struct {
	bun.BaseModel `bun:"table:chapters,alias:ch" tstype:"-"`

	ID        int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",notnull" json:"file_id"`
	ParentID  *int      `json:"parent_id"`
	SortOrder int       `bun:",notnull" json:"sort_order"`
	Title     string    `bun:",notnull" json:"title"`

	// Position data (mutually exclusive based on file type)
	StartPage        *int    `json:"start_page"`         // CBZ: 0-indexed page number
	StartTimestampMs *int64  `json:"start_timestamp_ms"` // M4B: milliseconds from start
	Href             *string `json:"href"`               // EPUB: content document href

	// Relations
	File     *File      `bun:"rel:belongs-to,join:file_id=id" json:"-"`
	Parent   *Chapter   `bun:"rel:belongs-to,join:parent_id=id" json:"-"`
	Children []*Chapter `bun:"rel:has-many,join:id=parent_id" json:"children,omitempty"`
}
```

**Step 2: Run type generation**

Run: `make tygo`
Expected: TypeScript types regenerated (may show "Nothing to be done" if already up-to-date, which is fine).

**Step 3: Commit**

```bash
git add pkg/models/chapter.go
git commit -m "[Models] Add Chapter model for file chapters"
```

---

## Task 3: ParsedChapter in mediafile Package

**Files:**
- Modify: `pkg/mediafile/mediafile.go`

**Step 1: Add ParsedChapter struct**

Add after `ParsedIdentifier` struct (around line 22):

```go
// ParsedChapter represents a chapter parsed from file metadata.
// Position fields are mutually exclusive based on file type.
type ParsedChapter struct {
	Title            string
	StartPage        *int            // CBZ: 0-indexed page number
	StartTimestampMs *int64          // M4B: milliseconds from start
	Href             *string         // EPUB: content document href
	Children         []ParsedChapter // EPUB nesting only; CBZ/M4B always empty
}
```

**Step 2: Add Chapters field to ParsedMetadata**

Add to ParsedMetadata struct (after Identifiers field):

```go
	// Chapters contains chapter information parsed from file metadata
	Chapters []ParsedChapter
```

**Step 3: Commit**

```bash
git add pkg/mediafile/mediafile.go
git commit -m "[Models] Add ParsedChapter struct for parsed chapter data"
```

---

## Task 4: M4B Chapter Integration

**Files:**
- Modify: `pkg/mp4/metadata.go`

**Step 1: Write test for M4B chapter conversion**

Create `pkg/mp4/chapters_test.go`:

```go
package mp4

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConvertChaptersToParsed(t *testing.T) {
	chapters := []Chapter{
		{Title: "Chapter 1", Start: 0, End: 5 * time.Minute},
		{Title: "Chapter 2", Start: 5 * time.Minute, End: 10 * time.Minute},
	}

	parsed := convertChaptersToParsed(chapters)

	assert.Len(t, parsed, 2)
	assert.Equal(t, "Chapter 1", parsed[0].Title)
	assert.NotNil(t, parsed[0].StartTimestampMs)
	assert.Equal(t, int64(0), *parsed[0].StartTimestampMs)
	assert.Nil(t, parsed[0].StartPage)
	assert.Nil(t, parsed[0].Href)
	assert.Empty(t, parsed[0].Children)

	assert.Equal(t, "Chapter 2", parsed[1].Title)
	assert.Equal(t, int64(300000), *parsed[1].StartTimestampMs) // 5 minutes in ms
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/mp4/... -run TestConvertChaptersToParsed -v`
Expected: FAIL - undefined: convertChaptersToParsed

**Step 3: Implement chapter conversion**

Add to `pkg/mp4/metadata.go` (after imports):

```go
import (
	"github.com/shishobooks/shisho/pkg/mediafile"
)
```

Add function at end of file:

```go
// convertChaptersToParsed converts mp4.Chapter slice to mediafile.ParsedChapter slice.
func convertChaptersToParsed(chapters []Chapter) []mediafile.ParsedChapter {
	parsed := make([]mediafile.ParsedChapter, 0, len(chapters))
	for _, ch := range chapters {
		ms := ch.Start.Milliseconds()
		parsed = append(parsed, mediafile.ParsedChapter{
			Title:            ch.Title,
			StartTimestampMs: &ms,
		})
	}
	return parsed
}
```

**Step 4: Update convertRawMetadata to include chapters**

Find `convertRawMetadata` function and add after duration is set:

```go
	// Convert chapters
	parsed.Chapters = convertChaptersToParsed(m.Chapters)
```

**Step 5: Run test to verify it passes**

Run: `go test ./pkg/mp4/... -run TestConvertChaptersToParsed -v`
Expected: PASS

**Step 6: Commit**

```bash
git add pkg/mp4/chapters_test.go pkg/mp4/metadata.go
git commit -m "[M4B] Integrate chapter parsing with ParsedMetadata"
```

---

## Task 5: CBZ Chapter Parsing - Phase 1 (Folder-based)

**Files:**
- Modify: `pkg/cbz/cbz.go`
- Create: `pkg/cbz/chapters.go`
- Create: `pkg/cbz/chapters_test.go`

**Step 1: Write tests for folder-based chapter detection**

Create `pkg/cbz/chapters_test.go`:

```go
package cbz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectChaptersFromFolders(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "chapters from immediate parent directories",
			files: []string{
				"Series Title/Chapter 1/page001.jpg",
				"Series Title/Chapter 1/page002.jpg",
				"Series Title/Chapter 2/page003.jpg",
				"Series Title/Chapter 2/page004.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "all files in same directory - no chapters",
			files: []string{
				"page001.jpg",
				"page002.jpg",
				"page003.jpg",
			},
			expected: nil,
		},
		{
			name: "single chapter folder",
			files: []string{
				"Chapter 1/page001.jpg",
				"Chapter 1/page002.jpg",
			},
			expected: nil, // Single folder = no chapters
		},
		{
			name: "deeply nested - uses immediate parent only",
			files: []string{
				"Volume 1/Arc 1/Chapter 1/page001.jpg",
				"Volume 1/Arc 1/Chapter 2/page002.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := detectChaptersFromFolders(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}

type expectedChapter struct {
	title     string
	startPage int
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/cbz/... -run TestDetectChaptersFromFolders -v`
Expected: FAIL - undefined: detectChaptersFromFolders

**Step 3: Implement folder-based chapter detection**

Create `pkg/cbz/chapters.go`:

```go
package cbz

import (
	"path/filepath"
	"sort"

	"github.com/shishobooks/shisho/pkg/mediafile"
)

// detectChaptersFromFolders detects chapters based on immediate parent directories.
// Returns empty slice if all files share the same parent or only one unique parent exists.
func detectChaptersFromFolders(files []string) []mediafile.ParsedChapter {
	if len(files) == 0 {
		return nil
	}

	// Group files by their immediate parent directory
	type chapterInfo struct {
		name       string
		firstPage  int
		pageCount  int
	}
	chapterMap := make(map[string]*chapterInfo)
	var chapterOrder []string

	for i, file := range files {
		parent := filepath.Dir(file)
		chapterName := filepath.Base(parent)

		// Root level files (parent = ".") have no chapter
		if parent == "." {
			chapterName = ""
		}

		if chapterName == "" {
			continue
		}

		if _, exists := chapterMap[parent]; !exists {
			chapterMap[parent] = &chapterInfo{
				name:      chapterName,
				firstPage: i,
			}
			chapterOrder = append(chapterOrder, parent)
		}
		chapterMap[parent].pageCount++
	}

	// If all files are in root or only one chapter folder, no chapters
	if len(chapterMap) <= 1 {
		return nil
	}

	// Build chapter list sorted by first page
	chapters := make([]mediafile.ParsedChapter, 0, len(chapterOrder))
	for _, parent := range chapterOrder {
		info := chapterMap[parent]
		startPage := info.firstPage
		chapters = append(chapters, mediafile.ParsedChapter{
			Title:     info.name,
			StartPage: &startPage,
		})
	}

	// Sort by start page
	sort.Slice(chapters, func(i, j int) bool {
		return *chapters[i].StartPage < *chapters[j].StartPage
	})

	return chapters
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/cbz/... -run TestDetectChaptersFromFolders -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/cbz/chapters.go pkg/cbz/chapters_test.go
git commit -m "[CBZ] Add folder-based chapter detection"
```

---

## Task 6: CBZ Chapter Parsing - Phase 2 (Filename Pattern)

**Files:**
- Modify: `pkg/cbz/chapters.go`
- Modify: `pkg/cbz/chapters_test.go`

**Step 1: Add tests for filename pattern detection**

Add to `pkg/cbz/chapters_test.go`:

```go
func TestDetectChaptersFromFilenames(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "ch prefix pattern",
			files: []string{
				"page001_ch01.jpg",
				"page002_ch01.jpg",
				"page003_ch02.jpg",
				"page004_ch02.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "chapter prefix pattern",
			files: []string{
				"chapter1_page001.jpg",
				"chapter1_page002.jpg",
				"chapter2_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "no pattern found",
			files: []string{
				"page001.jpg",
				"page002.jpg",
				"page003.jpg",
			},
			expected: nil,
		},
		{
			name: "case insensitive",
			files: []string{
				"CH01_page001.jpg",
				"CH02_page002.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := detectChaptersFromFilenames(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/cbz/... -run TestDetectChaptersFromFilenames -v`
Expected: FAIL - undefined: detectChaptersFromFilenames

**Step 3: Implement filename pattern detection**

Add to `pkg/cbz/chapters.go`:

```go
import (
	"fmt"
	"regexp"
	"strconv"
)

// chapterPattern matches "ch" or "chapter" followed by digits, case-insensitive.
var chapterPattern = regexp.MustCompile(`(?i)ch(?:apter)?[\s_-]*(\d+)`)

// detectChaptersFromFilenames detects chapters from filename patterns.
// Only used when all files share the same parent directory.
func detectChaptersFromFilenames(files []string) []mediafile.ParsedChapter {
	if len(files) == 0 {
		return nil
	}

	// Track chapter numbers and their first occurrence
	type chapterInfo struct {
		number    int
		firstPage int
	}
	chapterMap := make(map[int]*chapterInfo)

	for i, file := range files {
		filename := filepath.Base(file)
		matches := chapterPattern.FindStringSubmatch(filename)
		if matches == nil {
			continue
		}

		chNum, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}

		if _, exists := chapterMap[chNum]; !exists {
			chapterMap[chNum] = &chapterInfo{
				number:    chNum,
				firstPage: i,
			}
		}
	}

	// Need at least 2 chapters to be meaningful
	if len(chapterMap) < 2 {
		return nil
	}

	// Convert to sorted slice
	chapters := make([]mediafile.ParsedChapter, 0, len(chapterMap))
	for _, info := range chapterMap {
		startPage := info.firstPage
		chapters = append(chapters, mediafile.ParsedChapter{
			Title:     fmt.Sprintf("Chapter %d", info.number),
			StartPage: &startPage,
		})
	}

	// Sort by chapter number (which correlates with start page for well-formed archives)
	sort.Slice(chapters, func(i, j int) bool {
		return *chapters[i].StartPage < *chapters[j].StartPage
	})

	return chapters
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/cbz/... -run TestDetectChaptersFromFilenames -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/cbz/chapters.go pkg/cbz/chapters_test.go
git commit -m "[CBZ] Add filename pattern chapter detection"
```

---

## Task 7: CBZ Chapter Integration

**Files:**
- Modify: `pkg/cbz/chapters.go`
- Modify: `pkg/cbz/chapters_test.go`
- Modify: `pkg/cbz/cbz.go`

**Step 1: Add integration test**

Add to `pkg/cbz/chapters_test.go`:

```go
func TestDetectChapters(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []expectedChapter
	}{
		{
			name: "prefers folders over filenames",
			files: []string{
				"Chapter 1/ch01_page001.jpg",
				"Chapter 1/ch01_page002.jpg",
				"Chapter 2/ch02_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
		{
			name: "falls back to filenames when single folder",
			files: []string{
				"Comics/ch01_page001.jpg",
				"Comics/ch01_page002.jpg",
				"Comics/ch02_page003.jpg",
			},
			expected: []expectedChapter{
				{title: "Chapter 1", startPage: 0},
				{title: "Chapter 2", startPage: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chapters := DetectChapters(tt.files)
			if tt.expected == nil {
				assert.Empty(t, chapters)
				return
			}
			require.Len(t, chapters, len(tt.expected))
			for i, exp := range tt.expected {
				assert.Equal(t, exp.title, chapters[i].Title)
				require.NotNil(t, chapters[i].StartPage)
				assert.Equal(t, exp.startPage, *chapters[i].StartPage)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/cbz/... -run TestDetectChapters -v`
Expected: FAIL - undefined: DetectChapters

**Step 3: Implement DetectChapters function**

Add to `pkg/cbz/chapters.go`:

```go
// DetectChapters detects chapters from a list of image file paths.
// Uses folder-based detection first, falls back to filename patterns.
func DetectChapters(files []string) []mediafile.ParsedChapter {
	// Phase 1: Try folder-based detection
	chapters := detectChaptersFromFolders(files)
	if len(chapters) > 0 {
		return chapters
	}

	// Phase 2: Fall back to filename pattern detection
	return detectChaptersFromFilenames(files)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/cbz/... -run TestDetectChapters -v`
Expected: PASS

**Step 5: Integrate with Parse function**

Modify `pkg/cbz/cbz.go` - in the `Parse` function, after collecting image files and before returning, add chapter detection:

Find where `pageCount` is set and add after it:

```go
	// Detect chapters from image file paths
	chapters := DetectChapters(imageFiles)
```

Then include in the returned ParsedMetadata:

```go
	return &mediafile.ParsedMetadata{
		// ... existing fields ...
		Chapters:   chapters,
	}, nil
```

**Step 6: Run all CBZ tests**

Run: `go test ./pkg/cbz/... -v`
Expected: All tests PASS

**Step 7: Commit**

```bash
git add pkg/cbz/chapters.go pkg/cbz/chapters_test.go pkg/cbz/cbz.go
git commit -m "[CBZ] Integrate chapter detection with Parse function"
```

---

## Task 8: EPUB Navigation Document Parsing

**Files:**
- Create: `pkg/epub/nav.go`
- Create: `pkg/epub/nav_test.go`

**Step 1: Write tests for EPUB 3 nav document parsing**

Create `pkg/epub/nav_test.go`:

```go
package epub

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNavDocument(t *testing.T) {
	navXML := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
<nav epub:type="toc">
  <ol>
    <li><a href="chapter1.xhtml">Chapter 1</a></li>
    <li>
      <a href="part2.xhtml">Part 2</a>
      <ol>
        <li><a href="chapter2.xhtml">Chapter 2</a></li>
        <li><a href="chapter3.xhtml#section1">Chapter 3</a></li>
      </ol>
    </li>
  </ol>
</nav>
</body>
</html>`

	chapters, err := parseNavDocument(strings.NewReader(navXML))
	require.NoError(t, err)
	require.Len(t, chapters, 2)

	// Chapter 1 - flat
	assert.Equal(t, "Chapter 1", chapters[0].Title)
	require.NotNil(t, chapters[0].Href)
	assert.Equal(t, "chapter1.xhtml", *chapters[0].Href)
	assert.Empty(t, chapters[0].Children)

	// Part 2 - nested
	assert.Equal(t, "Part 2", chapters[1].Title)
	require.NotNil(t, chapters[1].Href)
	assert.Equal(t, "part2.xhtml", *chapters[1].Href)
	require.Len(t, chapters[1].Children, 2)

	// Nested children
	assert.Equal(t, "Chapter 2", chapters[1].Children[0].Title)
	assert.Equal(t, "chapter2.xhtml", *chapters[1].Children[0].Href)
	assert.Equal(t, "Chapter 3", chapters[1].Children[1].Title)
	assert.Equal(t, "chapter3.xhtml#section1", *chapters[1].Children[1].Href)
}

func TestParseNavDocument_SpanWithoutLink(t *testing.T) {
	navXML := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<body>
<nav epub:type="toc">
  <ol>
    <li><span>Part 1 (no link)</span>
      <ol>
        <li><a href="chapter1.xhtml">Chapter 1</a></li>
      </ol>
    </li>
  </ol>
</nav>
</body>
</html>`

	chapters, err := parseNavDocument(strings.NewReader(navXML))
	require.NoError(t, err)
	require.Len(t, chapters, 1)

	// Part 1 - span without href
	assert.Equal(t, "Part 1 (no link)", chapters[0].Title)
	assert.Nil(t, chapters[0].Href)
	require.Len(t, chapters[0].Children, 1)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./pkg/epub/... -run TestParseNavDocument -v`
Expected: FAIL - undefined: parseNavDocument

**Step 3: Implement EPUB 3 nav document parsing**

Create `pkg/epub/nav.go`:

```go
package epub

import (
	"encoding/xml"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

// NavHTML represents the EPUB 3 navigation document structure.
type NavHTML struct {
	XMLName xml.Name `xml:"html"`
	Body    struct {
		Nav []NavElement `xml:"nav"`
	} `xml:"body"`
}

// NavElement represents a nav element in the navigation document.
type NavElement struct {
	Type string  `xml:"type,attr"`
	OL   *NavOL  `xml:"ol"`
}

// NavOL represents an ordered list in the navigation.
type NavOL struct {
	Items []NavLI `xml:"li"`
}

// NavLI represents a list item in the navigation.
type NavLI struct {
	A        *NavLink `xml:"a"`
	Span     *NavSpan `xml:"span"`
	Children *NavOL   `xml:"ol"`
}

// NavLink represents an anchor element.
type NavLink struct {
	Href string `xml:"href,attr"`
	Text string `xml:",chardata"`
}

// NavSpan represents a span element (heading without link).
type NavSpan struct {
	Text string `xml:",chardata"`
}

// parseNavDocument parses an EPUB 3 navigation document and returns chapters.
func parseNavDocument(r io.Reader) ([]mediafile.ParsedChapter, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var nav NavHTML
	if err := xml.Unmarshal(data, &nav); err != nil {
		return nil, errors.WithStack(err)
	}

	// Find the toc nav element
	for _, n := range nav.Body.Nav {
		if n.Type == "toc" && n.OL != nil {
			return parseNavOL(n.OL), nil
		}
	}

	return nil, nil
}

// parseNavOL recursively parses an ordered list into chapters.
func parseNavOL(ol *NavOL) []mediafile.ParsedChapter {
	if ol == nil {
		return nil
	}

	chapters := make([]mediafile.ParsedChapter, 0, len(ol.Items))
	for _, li := range ol.Items {
		ch := mediafile.ParsedChapter{}

		// Get title and href from anchor or span
		if li.A != nil {
			ch.Title = strings.TrimSpace(li.A.Text)
			if li.A.Href != "" {
				href := li.A.Href
				ch.Href = &href
			}
		} else if li.Span != nil {
			ch.Title = strings.TrimSpace(li.Span.Text)
		}

		// Skip items without a title
		if ch.Title == "" {
			continue
		}

		// Parse nested children
		if li.Children != nil {
			ch.Children = parseNavOL(li.Children)
		}

		chapters = append(chapters, ch)
	}

	return chapters
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./pkg/epub/... -run TestParseNavDocument -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/epub/nav.go pkg/epub/nav_test.go
git commit -m "[EPUB] Add EPUB 3 navigation document parser"
```

---

## Task 9: EPUB NCX Fallback Parsing

**Files:**
- Modify: `pkg/epub/nav.go`
- Modify: `pkg/epub/nav_test.go`

**Step 1: Add tests for NCX parsing**

Add to `pkg/epub/nav_test.go`:

```go
func TestParseNCX(t *testing.T) {
	ncxXML := `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
    <navPoint id="ch1-1" playOrder="2">
      <navLabel><text>Section 1.1</text></navLabel>
      <content src="chapter1.xhtml#s1"/>
    </navPoint>
  </navPoint>
  <navPoint id="ch2" playOrder="3">
    <navLabel><text>Chapter 2</text></navLabel>
    <content src="chapter2.xhtml"/>
  </navPoint>
</navMap>
</ncx>`

	chapters, err := parseNCX(strings.NewReader(ncxXML))
	require.NoError(t, err)
	require.Len(t, chapters, 2)

	// Chapter 1 with nested section
	assert.Equal(t, "Chapter 1", chapters[0].Title)
	require.NotNil(t, chapters[0].Href)
	assert.Equal(t, "chapter1.xhtml", *chapters[0].Href)
	require.Len(t, chapters[0].Children, 1)
	assert.Equal(t, "Section 1.1", chapters[0].Children[0].Title)
	assert.Equal(t, "chapter1.xhtml#s1", *chapters[0].Children[0].Href)

	// Chapter 2 - flat
	assert.Equal(t, "Chapter 2", chapters[1].Title)
	assert.Equal(t, "chapter2.xhtml", *chapters[1].Href)
	assert.Empty(t, chapters[1].Children)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/epub/... -run TestParseNCX -v`
Expected: FAIL - undefined: parseNCX

**Step 3: Implement NCX parsing**

Add to `pkg/epub/nav.go`:

```go
// NCX represents the EPUB 2 NCX structure.
type NCX struct {
	XMLName xml.Name `xml:"ncx"`
	NavMap  struct {
		NavPoints []NCXNavPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

// NCXNavPoint represents a navigation point in NCX.
type NCXNavPoint struct {
	ID       string `xml:"id,attr"`
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []NCXNavPoint `xml:"navPoint"`
}

// parseNCX parses an EPUB 2 NCX file and returns chapters.
func parseNCX(r io.Reader) ([]mediafile.ParsedChapter, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var ncx NCX
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, errors.WithStack(err)
	}

	return parseNCXNavPoints(ncx.NavMap.NavPoints), nil
}

// parseNCXNavPoints recursively parses NCX navigation points.
func parseNCXNavPoints(navPoints []NCXNavPoint) []mediafile.ParsedChapter {
	chapters := make([]mediafile.ParsedChapter, 0, len(navPoints))
	for _, np := range navPoints {
		title := strings.TrimSpace(np.NavLabel.Text)
		if title == "" {
			continue
		}

		ch := mediafile.ParsedChapter{
			Title: title,
		}

		if np.Content.Src != "" {
			src := np.Content.Src
			ch.Href = &src
		}

		if len(np.Children) > 0 {
			ch.Children = parseNCXNavPoints(np.Children)
		}

		chapters = append(chapters, ch)
	}
	return chapters
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/epub/... -run TestParseNCX -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/epub/nav.go pkg/epub/nav_test.go
git commit -m "[EPUB] Add EPUB 2 NCX parser for chapter fallback"
```

---

## Task 10: EPUB Chapter Integration

**Files:**
- Modify: `pkg/epub/opf.go`
- Modify: `pkg/epub/nav.go`

**Step 1: Add chapter fields to OPF struct**

In `pkg/epub/opf.go`, add to the `OPF` struct:

```go
	Chapters []mediafile.ParsedChapter
```

**Step 2: Update ParseOPF to track nav and NCX locations**

In `pkg/epub/opf.go`, add to the `Package` struct's `Manifest.Item`:

```go
	Properties string `xml:"properties,attr"`
```

**Step 3: Add helper to find nav document in manifest**

Add to `pkg/epub/nav.go`:

```go
// findNavDocumentHref finds the navigation document href from an OPF package.
// Returns empty string if not found.
func findNavDocumentHref(pkg *Package, basePath string) string {
	for _, item := range pkg.Manifest.Item {
		if strings.Contains(item.Properties, "nav") {
			return basePath + item.Href
		}
	}
	return ""
}

// findNCXHref finds the NCX file href from an OPF package.
// Returns empty string if not found.
func findNCXHref(pkg *Package, basePath string) string {
	ncxID := pkg.Spine.Toc
	if ncxID == "" {
		return ""
	}
	for _, item := range pkg.Manifest.Item {
		if item.ID == ncxID {
			return basePath + item.Href
		}
	}
	return ""
}
```

**Step 4: Update Parse function to extract chapters**

In `pkg/epub/opf.go`, modify the `Parse` function to also read chapters. After the OPF is parsed and before the cover is extracted, add chapter extraction:

```go
	// Try to find and parse chapters from nav document or NCX
	navHref := findNavDocumentHref(pkg, basePath)
	ncxHref := findNCXHref(pkg, basePath)

	for _, file := range zipReader.File {
		if navHref != "" && file.Name == navHref {
			r, err := file.Open()
			if err == nil {
				chapters, _ := parseNavDocument(r)
				r.Close()
				if len(chapters) > 0 {
					opf.Chapters = chapters
					break
				}
			}
		}
		if ncxHref != "" && file.Name == ncxHref && len(opf.Chapters) == 0 {
			r, err := file.Open()
			if err == nil {
				chapters, _ := parseNCX(r)
				r.Close()
				opf.Chapters = chapters
			}
		}
	}
```

Then include chapters in the returned ParsedMetadata:

```go
	return &mediafile.ParsedMetadata{
		// ... existing fields ...
		Chapters:   opf.Chapters,
	}, nil
```

**Step 5: Run all EPUB tests**

Run: `go test ./pkg/epub/... -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
git add pkg/epub/opf.go pkg/epub/nav.go
git commit -m "[EPUB] Integrate chapter parsing with Parse function"
```

---

## Task 11: Chapter Service Layer

**Files:**
- Create: `pkg/chapters/service.go`

**Step 1: Create the chapters service**

```go
package chapters

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

type Service struct {
	db *bun.DB
}

func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// ListChapters retrieves all chapters for a file, building nested structure.
func (s *Service) ListChapters(ctx context.Context, fileID int) ([]*models.Chapter, error) {
	var chapters []*models.Chapter
	err := s.db.NewSelect().
		Model(&chapters).
		Where("file_id = ?", fileID).
		Order("sort_order ASC").
		Scan(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return buildChapterTree(chapters), nil
}

// ReplaceChapters deletes all existing chapters for a file and inserts new ones.
func (s *Service) ReplaceChapters(ctx context.Context, fileID int, chapters []mediafile.ParsedChapter) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Delete existing chapters
		_, err := tx.NewDelete().
			Model((*models.Chapter)(nil)).
			Where("file_id = ?", fileID).
			Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert new chapters
		return insertChapters(ctx, tx, fileID, nil, chapters)
	})
}

// DeleteChaptersForFile deletes all chapters for a file.
func (s *Service) DeleteChaptersForFile(ctx context.Context, fileID int) error {
	_, err := s.db.NewDelete().
		Model((*models.Chapter)(nil)).
		Where("file_id = ?", fileID).
		Exec(ctx)
	return errors.WithStack(err)
}

// insertChapters recursively inserts chapters with their children.
func insertChapters(ctx context.Context, tx bun.Tx, fileID int, parentID *int, chapters []mediafile.ParsedChapter) error {
	for i, ch := range chapters {
		model := &models.Chapter{
			FileID:           fileID,
			ParentID:         parentID,
			SortOrder:        i,
			Title:            ch.Title,
			StartPage:        ch.StartPage,
			StartTimestampMs: ch.StartTimestampMs,
			Href:             ch.Href,
		}

		_, err := tx.NewInsert().Model(model).Exec(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		// Recursively insert children
		if len(ch.Children) > 0 {
			if err := insertChapters(ctx, tx, fileID, &model.ID, ch.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildChapterTree converts a flat list of chapters into a nested tree.
func buildChapterTree(chapters []*models.Chapter) []*models.Chapter {
	// Build lookup map
	byID := make(map[int]*models.Chapter)
	for _, ch := range chapters {
		ch.Children = []*models.Chapter{} // Initialize empty slice
		byID[ch.ID] = ch
	}

	// Build tree
	var roots []*models.Chapter
	for _, ch := range chapters {
		if ch.ParentID == nil {
			roots = append(roots, ch)
		} else if parent, ok := byID[*ch.ParentID]; ok {
			parent.Children = append(parent.Children, ch)
		}
	}

	return roots
}
```

**Step 2: Commit**

```bash
git add pkg/chapters/service.go
git commit -m "[Chapters] Add chapter service for database operations"
```

---

## Task 12: Chapter API Handlers

**Files:**
- Create: `pkg/chapters/handlers.go`
- Create: `pkg/chapters/routes.go`
- Create: `pkg/chapters/validators.go`

**Step 1: Create validators**

Create `pkg/chapters/validators.go`:

```go
package chapters

// ChapterInput represents a chapter in API requests.
type ChapterInput struct {
	Title            string         `json:"title"`
	StartPage        *int           `json:"start_page"`
	StartTimestampMs *int64         `json:"start_timestamp_ms"`
	Href             *string        `json:"href"`
	Children         []ChapterInput `json:"children"`
}

// ReplaceChaptersPayload is the request body for replacing chapters.
type ReplaceChaptersPayload struct {
	Chapters []ChapterInput `json:"chapters"`
}
```

**Step 2: Create handlers**

Create `pkg/chapters/handlers.go`:

```go
package chapters

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

type handler struct {
	chapterService *Service
	bookService    *books.Service
}

func (h *handler) list(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Verify file exists and check access
	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	chapters, err := h.chapterService.ListChapters(ctx, fileID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"chapters": chapters,
	})
}

func (h *handler) replace(c echo.Context) error {
	ctx := c.Request().Context()

	fileID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return errcodes.NotFound("File")
	}

	// Bind payload
	var payload ReplaceChaptersPayload
	if err := c.Bind(&payload); err != nil {
		return errors.WithStack(err)
	}

	// Verify file exists and check access
	file, err := h.bookService.RetrieveFile(ctx, books.RetrieveFileOptions{ID: &fileID})
	if err != nil {
		return errors.WithStack(err)
	}

	if user, ok := c.Get("user").(*models.User); ok {
		if !user.HasLibraryAccess(file.LibraryID) {
			return errcodes.Forbidden("You don't have access to this library")
		}
	}

	// Validate chapters based on file type
	if err := validateChapters(file, payload.Chapters); err != nil {
		return err
	}

	// Convert input to ParsedChapter
	chapters := convertInputToChapters(payload.Chapters)

	// Replace chapters
	if err := h.chapterService.ReplaceChapters(ctx, fileID, chapters); err != nil {
		return errors.WithStack(err)
	}

	// Return updated chapters
	updatedChapters, err := h.chapterService.ListChapters(ctx, fileID)
	if err != nil {
		return errors.WithStack(err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"chapters": updatedChapters,
	})
}

// validateChapters validates chapter data against file constraints.
func validateChapters(file *models.File, chapters []ChapterInput) error {
	for _, ch := range chapters {
		switch file.FileType {
		case models.FileTypeCBZ:
			if ch.StartPage != nil && file.PageCount != nil {
				if *ch.StartPage >= *file.PageCount {
					return errcodes.ValidationError("start_page must be less than page_count")
				}
			}
		case models.FileTypeM4B:
			if ch.StartTimestampMs != nil && file.AudiobookDurationSeconds != nil {
				maxMs := int64(*file.AudiobookDurationSeconds * 1000)
				if *ch.StartTimestampMs > maxMs {
					return errcodes.ValidationError("start_timestamp_ms exceeds file duration")
				}
			}
		}

		// Validate children recursively
		if len(ch.Children) > 0 {
			if err := validateChapters(file, ch.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

// convertInputToChapters converts ChapterInput slice to ParsedChapter slice.
func convertInputToChapters(inputs []ChapterInput) []mediafile.ParsedChapter {
	chapters := make([]mediafile.ParsedChapter, 0, len(inputs))
	for _, in := range inputs {
		ch := mediafile.ParsedChapter{
			Title:            in.Title,
			StartPage:        in.StartPage,
			StartTimestampMs: in.StartTimestampMs,
			Href:             in.Href,
		}
		if len(in.Children) > 0 {
			ch.Children = convertInputToChapters(in.Children)
		}
		chapters = append(chapters, ch)
	}
	return chapters
}
```

**Step 3: Create routes**

Create `pkg/chapters/routes.go`:

```go
package chapters

import (
	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/uptrace/bun"
)

func RegisterRoutes(g *echo.Group, db *bun.DB, authMiddleware *auth.Middleware) {
	h := &handler{
		chapterService: NewService(db),
		bookService:    books.NewService(db),
	}

	g.GET("/files/:id/chapters", h.list)
	g.PUT("/files/:id/chapters", h.replace, authMiddleware.RequirePermission(models.ResourceBooks, models.OperationWrite))
}
```

**Step 4: Commit**

```bash
git add pkg/chapters/
git commit -m "[Chapters] Add API handlers for list and replace chapters"
```

---

## Task 13: Register Chapter Routes

**Files:**
- Modify: `pkg/server/server.go`

**Step 1: Import chapters package**

Add to imports:

```go
	"github.com/shishobooks/shisho/pkg/chapters"
```

**Step 2: Register routes**

Find where other routes are registered (look for similar patterns like `books.RegisterRoutes`) and add:

```go
	chapters.RegisterRoutes(api, db, authMiddleware)
```

**Step 3: Commit**

```bash
git add pkg/server/server.go
git commit -m "[Server] Register chapter API routes"
```

---

## Task 14: Sync Worker Integration

**Files:**
- Modify: `pkg/worker/sync.go` (or wherever file sync happens)

**Step 1: Import chapters package**

Add to imports in the sync worker file:

```go
	"github.com/shishobooks/shisho/pkg/chapters"
```

**Step 2: Add chapter sync after file metadata sync**

Find where file metadata is saved after parsing, and add chapter sync:

```go
	// Sync chapters from parsed metadata
	if len(parsed.Chapters) > 0 {
		chapterService := chapters.NewService(db)
		if err := chapterService.ReplaceChapters(ctx, file.ID, parsed.Chapters); err != nil {
			log.Warn("failed to sync chapters", logger.Data{"file_id": file.ID, "error": err.Error()})
		}
	}
```

**Step 3: Commit**

```bash
git add pkg/worker/sync.go
git commit -m "[Worker] Integrate chapter sync during file scan"
```

---

## Task 15: Run Full Test Suite

**Step 1: Run all tests**

Run: `make check`
Expected: All tests pass, no lint errors.

**Step 2: Manual verification**

1. Start dev server: `make start`
2. Trigger a library scan
3. Check API: `curl http://localhost:8080/api/files/1/chapters`
4. Verify chapters are returned for files that have them

**Step 3: Final commit if any fixes needed**

```bash
git add -A
git commit -m "[Chapters] Fix any issues found during integration testing"
```

---

## Summary

This plan implements:

1. **Database** - Migration for chapters table with indexes and cascade delete
2. **Models** - Chapter model with Bun relations, ParsedChapter in mediafile
3. **Parsing**:
   - M4B: Integration with existing mp4.Chapter → ParsedChapter conversion
   - CBZ: Two-phase detection (folders then filename patterns)
   - EPUB: EPUB 3 nav document with NCX fallback, supporting nested chapters
4. **API** - GET/PUT endpoints for `/api/files/:id/chapters` with validation
5. **Worker** - Chapter sync during file scan

Each task is designed to be completed in 2-5 minutes with immediate test verification.
