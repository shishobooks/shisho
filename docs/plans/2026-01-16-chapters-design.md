# Chapter Parsing and Storage Design

This document covers the design for parsing and storing chapter information for CBZ, M4B, and EPUB files.

## Overview

Chapters are associated with individual files (not books). Each file type has different chapter semantics:

- **CBZ**: Chapters identified by folders or filename patterns, position is page number
- **M4B**: Chapters from MP4 metadata (QuickTime or Nero format), position is timestamp
- **EPUB**: Chapters from navigation document (EPUB 3 nav or EPUB 2 NCX), position is href to content document

EPUB chapters support nesting (e.g., Part 1 > Chapter 1); CBZ and M4B are flat only.

## Database Schema

```sql
CREATE TABLE chapters (
    id INTEGER PRIMARY KEY,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    parent_id INTEGER REFERENCES chapters(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL,
    title TEXT NOT NULL,

    -- Position data (mutually exclusive based on file type)
    start_page INTEGER,           -- CBZ: 0-indexed page number
    start_timestamp_ms INTEGER,   -- M4B: milliseconds from start
    href TEXT,                    -- EPUB: "chapter1.xhtml" or "chapter1.xhtml#section3"

    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX ix_chapters_file_id ON chapters(file_id);
CREATE INDEX ix_chapters_parent_id ON chapters(parent_id);
CREATE UNIQUE INDEX ux_chapters_file_sort ON chapters(file_id, parent_id, sort_order);
```

### Sort Order

`sort_order` is scoped to siblings (chapters sharing the same `parent_id`):

| id | file_id | parent_id | sort_order | title |
|----|---------|-----------|------------|-------|
| 1  | 100     | NULL      | 0          | Part 1 |
| 2  | 100     | NULL      | 1          | Part 2 |
| 3  | 100     | 1         | 0          | Chapter 1 |
| 4  | 100     | 1         | 1          | Chapter 2 |
| 5  | 100     | 2         | 0          | Chapter 3 |

Children of different parents can reuse sort_order values.

## Go Models

```go
// pkg/models/chapter.go

type Chapter struct {
    bun.BaseModel `bun:"table:chapters"`

    ID        int64     `bun:"id,pk,autoincrement"`
    FileID    int64     `bun:"file_id,notnull"`
    ParentID  *int64    `bun:"parent_id"`
    SortOrder int       `bun:"sort_order,notnull"`
    Title     string    `bun:"title,notnull"`

    // Position data (one set per file type)
    StartPage        *int    `bun:"start_page"`          // CBZ
    StartTimestampMs *int64  `bun:"start_timestamp_ms"`  // M4B
    Href             *string `bun:"href"`                // EPUB

    CreatedAt time.Time `bun:"created_at,notnull,default:current_timestamp"`
    UpdatedAt time.Time `bun:"updated_at,notnull,default:current_timestamp"`

    // Relations
    File     *File      `bun:"rel:belongs-to,join:file_id=id"`
    Parent   *Chapter   `bun:"rel:belongs-to,join:parent_id=id"`
    Children []*Chapter `bun:"rel:has-many,join:id=parent_id"`
}
```

For parsed metadata (`pkg/mediafile/mediafile.go`):

```go
type ParsedChapter struct {
    Title            string
    StartPage        *int            // CBZ
    StartTimestampMs *int64          // M4B
    Href             *string         // EPUB
    Children         []ParsedChapter // EPUB nesting only
}
```

## CBZ Chapter Parsing

Two-phase detection:

### Phase 1: Folder-based detection

Only the **immediate parent directory** of pages is used as chapter name:

```
archive.cbz/
├── Series Title/           # Ignored (not immediate parent)
│   ├── Chapter 1/          # Chapter name
│   │   ├── page001.jpg
│   │   └── page002.jpg
│   ├── Chapter 2/          # Chapter name
│   │   ├── page003.jpg
│   │   └── page004.jpg
```

### Phase 2: Filename pattern fallback

If all pages share the same parent directory, scan filenames for `ch?\d+` pattern (case-insensitive):

```
archive.cbz/
├── page001_ch01.jpg   → "Chapter 1" (start_page: 0)
├── page002_ch01.jpg
├── page003_ch02.jpg   → "Chapter 2" (start_page: 2)
```

### Logic

1. Find all image files in archive
2. For each image, get its immediate parent directory
3. Group images by immediate parent
4. If all images share same parent → try filename pattern detection
5. If images have different parents → parent directory names become chapter names
6. Sort chapters by first page number

### Edge Cases

- No chapters detected → return empty slice (valid)
- Same chapter name from different paths → keep both (names don't need to be unique)

## CBZ Chapter Generation

When generating a CBZ with chapter data, create folder structure:

```
output.cbz/
├── ComicInfo.xml
├── Chapter 1/
│   ├── 001.jpg  (page 0)
│   ├── 002.jpg  (page 1)
│   └── 003.jpg  (page 2)
├── Chapter 2/
│   ├── 004.jpg  (page 3)
│   └── 005.jpg  (page 4)
```

### Logic

1. Load chapters for file, sorted by `sort_order`
2. Determine page ranges: chapter N owns pages from `start_page` to `next_chapter.start_page - 1`
3. Place each image in appropriate chapter folder
4. Sanitize folder names (remove invalid filesystem chars)
5. Empty chapters (0 pages) are skipped

## M4B Chapter Parsing

Already implemented in `pkg/mp4/chapters.go`. Supports:

- **QuickTime chapters**: `moov → trak → tref/chap` (priority)
- **Nero chapters**: `moov → udta → chpl` (fallback)

Existing `mp4.Chapter` struct:

```go
type Chapter struct {
    Title string
    Start time.Duration
    End   time.Duration
}
```

Convert to `ParsedChapter` during parsing:

```go
for _, ch := range metadata.Chapters {
    ms := ch.Start.Milliseconds()
    parsed.Chapters = append(parsed.Chapters, mediafile.ParsedChapter{
        Title:            ch.Title,
        StartTimestampMs: &ms,
    })
}
```

## M4B Chapter Generation

Already supported in `pkg/mp4/writer.go`. Convert from database model back to `[]mp4.Chapter` before calling the writer.

## EPUB Chapter Parsing

### EPUB 3 Navigation Document

```xml
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
```

### EPUB 2 NCX

```xml
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
    <navPoint id="ch1-1">
      <navLabel><text>Section 1.1</text></navLabel>
      <content src="chapter1.xhtml#s1"/>
    </navPoint>
  </navPoint>
</navMap>
```

### Parsing Logic

1. Look for nav document in OPF manifest (`properties="nav"`)
2. If found → parse `<nav epub:type="toc">` structure
3. If not found → look for NCX file (declared in OPF `<spine toc="ncx-id">`)
4. Parse NCX `<navMap>/<navPoint>` structure
5. Return `[]ParsedChapter` with nested `Children`

Both formats support nesting.

### Edge Cases

- `<span>` instead of `<a>` (heading without link) → store title, href = nil
- No nav document or NCX found → return empty chapters (valid)

## EPUB Chapter Generation

Write chapters back to whichever format was parsed (nav or NCX):

1. Load chapters for file (with nested structure via `parent_id`)
2. Detect which format source EPUB uses
3. Build new TOC structure from chapter data
4. Replace TOC content in nav document or NCX file
5. Preserve other nav elements (landmarks, page-list) untouched

Works for KePub since KePub uses the same EPUB 3 nav structure.

## API Endpoints

```
GET    /api/files/:id/chapters     → List chapters (nested structure for EPUB)
PUT    /api/files/:id/chapters     → Replace all chapters (validates hrefs)
```

### Response Structure

```json
{
  "chapters": [
    {
      "id": 1,
      "title": "Part 1",
      "href": "part1.xhtml",
      "start_page": null,
      "start_timestamp_ms": null,
      "children": [
        {
          "id": 2,
          "title": "Chapter 1",
          "href": "chapter1.xhtml",
          "children": []
        }
      ]
    }
  ]
}
```

For M4B/CBZ, `children` is always empty array.

## Validation

### On Chapter Save (PUT)

Server loads the file and validates:

| File Type | Validation |
|-----------|------------|
| EPUB | If href changed or is new chapter, validate href exists in spine. Existing invalid hrefs are preserved (allows editing other fields without fixing pre-existing issues) |
| M4B | `start_timestamp_ms` must not exceed `file.audiobook_duration_seconds * 1000` |
| CBZ | `start_page` must be less than `file.page_count` |

### Timing

- **Parse time**: No validation (source file might already be broken)
- **Chapter save (API)**: Server validates against loaded file
- **Generation time**: Trust DB data (already validated on save)

## Skill Updates Required

| Skill | Section to Add |
|-------|----------------|
| `cbz/SKILL.md` | "## Chapters" - folder-based detection, filename pattern, generation |
| `m4b/SKILL.md` | "## Chapter Database Integration" - mapping to chapters table |
| `epub/SKILL.md` | "## Navigation & Chapters" - EPUB 3 nav, EPUB 2 NCX, parsing/generation |
| `kepub/SKILL.md` | "## Chapter-Aware TOC Generation" - structured TOC from chapter data |
| `backend/SKILL.md` | Add chapters to Metadata Sync Checklist (items 15-19) |
| `frontend/SKILL.md` | "## Nested Data Editing (Chapters)" - tree display, reordering |

## Testing Requirements

| Component | Tests |
|-----------|-------|
| CBZ parsing | Folder-based detection, nested directories (immediate parent only), filename pattern, no chapters |
| CBZ generation | Folder creation, empty chapter skip, page distribution |
| EPUB parsing | Nav document, NCX fallback, nested chapters, missing nav/NCX |
| EPUB generation | Nav update, NCX update, preserve other elements |
| EPUB validation | Invalid href detection, existing invalid href preservation |
| M4B | Verify integration with ParsedChapter |
| M4B validation | Timestamp exceeds duration |
| CBZ validation | Page exceeds page count |
| KePub | Chapter-aware TOC generation |
| API | Nested structure serialization, validation errors |
