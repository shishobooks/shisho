---
name: cbz
description: You MUST use this before working on anything CBZ/comic-related (e.g. parsing, generation, ComicInfo.xml metadata, etc.). Covers creator roles, page handling, and the cbz/filegen packages.
---

# CBZ Format Reference

This skill documents the CBZ (Comic Book ZIP) format as used in Shisho for parsing and generation.

## File Structure

CBZ files are ZIP archives containing:

```
ComicInfo.xml             # Metadata file (optional but common)
000.png                   # Page images (PNG, JPG, WEBP, GIF)
001.png
002.png
...
cover.jpg                 # Sometimes a separate cover file
```

## ComicInfo.xml Schema

The ComicInfo.xml file follows the ComicRack schema.

### Basic Metadata

```xml
<?xml version="1.0" encoding="UTF-8"?>
<ComicInfo>
  <Title>Comic Title</Title>
  <Series>Series Name</Series>
  <Number>5</Number>           <!-- Issue number, can be decimal like "1.5" -->
  <Volume>1</Volume>
  <Year>2024</Year>
  <Month>6</Month>
  <Day>15</Day>
  <Publisher>Publisher Name</Publisher>
  <Imprint>Imprint Name</Imprint>
  <Summary>Description text</Summary>
  <Web>https://example.com</Web>
  <PageCount>24</PageCount>
  <LanguageISO>en</LanguageISO>
  <Format>Trade Paperback</Format>
  <BlackAndWhite>No</BlackAndWhite>
  <Manga>No</Manga>
  <AgeRating>Teen</AgeRating>
  <CommunityRating>4.5</CommunityRating>
  <GTIN>9781234567890</GTIN>
  <Characters>Character1, Character2</Characters>
  <Teams>Team1, Team2</Teams>
  <Locations>Location1, Location2</Locations>
  <StoryArc>Arc Name</StoryArc>
</ComicInfo>
```

### Creator Roles

CBZ uses separate fields for each creator role (8 types supported):

```xml
<Writer>Writer Name</Writer>           <!-- Script/story writer -->
<Penciller>Artist Name</Penciller>     <!-- Pencil artist -->
<Inker>Inker Name</Inker>              <!-- Ink artist -->
<Colorist>Colorist Name</Colorist>     <!-- Color artist -->
<Letterer>Letterer Name</Letterer>     <!-- Text/lettering -->
<CoverArtist>Cover Artist</CoverArtist>
<Editor>Editor Name</Editor>
<Translator>Translator Name</Translator>
```

Multiple creators per role are comma-separated:

```xml
<Writer>Writer One, Writer Two</Writer>
```

### Genres and Tags

```xml
<Genre>Action, Adventure</Genre>       <!-- Comma-separated genres -->
<Tags>Must Read, Favorites</Tags>      <!-- Comma-separated tags -->
```

### Page Information

```xml
<Pages>
  <Page Image="0" Type="FrontCover"/>
  <Page Image="1"/>
  <Page Image="2" DoublePage="true"/>
  <Page Image="3" ImageSize="1234567" ImageWidth="800" ImageHeight="1200"/>
</Pages>
```

**Page Types:** `FrontCover`, `InnerCover`, `Roundup`, `Story`, `Advertisement`, `Editorial`, `Letters`, `Preview`, `BackCover`, `Other`, `Deleted`

## Shisho Implementation

### Parsing (`pkg/cbz/cbz.go`)

**All Metadata Fields Extracted:**

| Field | XML Element | Notes |
|-------|-------------|-------|
| Title | `<Title>` | Direct extraction |
| Series | `<Series>` | Series name |
| Series Number | `<Number>` | Parsed as float (supports decimals) |
| Volume | `<Volume>` | Volume number |
| Authors | 8 creator fields | Each role mapped to AuthorInfo with role |
| Genres | `<Genre>` | Comma-separated, split into array |
| Tags | `<Tags>` | Comma-separated, split into array |
| Description | `<Summary>` | Full text |
| URL | `<Web>` | Direct extraction |
| Publisher | `<Publisher>` | Direct extraction |
| Imprint | `<Imprint>` | Direct extraction |
| Release Date | `<Year>/<Month>/<Day>` | Combined into time.Time |
| Cover Page | `<Pages>` | Index of page with Type="FrontCover" |
| Page Count | Image files | Counted from actual images in ZIP |

**Cover Detection Strategy (3-tier fallback):**
1. Look for `<Page Type="FrontCover">` in ComicInfo
2. Fallback to `<Page Type="InnerCover">`
3. Use first image file alphabetically

**Series Number Fallback:**
If not in ComicInfo, regex patterns extract from filename: `#7`, `v7`, or ` 7` at end of filename.

**Data Source:** All extracted metadata tagged with `models.DataSourceCBZMetadata`

### Generation (`pkg/filegen/cbz.go`)

When generating CBZ files, Shisho:

1. Preserves all original page images unchanged (byte-for-byte)
2. Creates/updates ComicInfo.xml with metadata from book model
3. Uses atomic write pattern (temp file + rename)

**Metadata Written Back:**

| Field | ComicInfo Element | Source |
|-------|-------------------|--------|
| Title | `<Title>` | book.Title |
| Series | `<Series>` | Primary BookSeries (sorted by SortOrder) |
| Number | `<Number>` | Primary series number |
| All 8 author roles | Role-specific elements | Authors mapped by role, comma-separated |
| Genres | `<Genre>` | Comma-separated from BookGenres |
| Tags | `<Tags>` | Comma-separated from BookTags |
| Description | `<Summary>` | book.Description |
| URL | `<Web>` | file.URL |
| Publisher | `<Publisher>` | file.Publisher.Name |
| Imprint | `<Imprint>` | file.Imprint.Name |
| Release Date | `<Year>/<Month>/<Day>` | file.ReleaseDate |

**Author Role Mapping:**

| Role | XML Field |
|------|-----------|
| writer (or empty) | `<Writer>` |
| penciller | `<Penciller>` |
| inker | `<Inker>` |
| colorist | `<Colorist>` |
| letterer | `<Letterer>` |
| cover_artist | `<CoverArtist>` |
| editor | `<Editor>` |
| translator | `<Translator>` |

**Cover Page Handling:**
- Updates `<Pages>` section with correct `<Page Image="N" Type="FrontCover"/>`
- Creates Pages section if missing
- Clears previous FrontCover types before setting new one

**Preservation Behavior:**
- Untracked fields in original ComicInfo.xml are preserved
- If book has no genres, original genres preserved
- If book has no tags, original tags preserved

### Key Types

```go
// ComicInfo XML structure (pkg/cbz/types.go)
type cbzComicInfo struct {
    Title       string       `xml:"Title"`
    Series      string       `xml:"Series"`
    Number      string       `xml:"Number"`
    Volume      string       `xml:"Volume"`
    Writer      string       `xml:"Writer"`
    Penciller   string       `xml:"Penciller"`
    Inker       string       `xml:"Inker"`
    Colorist    string       `xml:"Colorist"`
    Letterer    string       `xml:"Letterer"`
    CoverArtist string       `xml:"CoverArtist"`
    Editor      string       `xml:"Editor"`
    Translator  string       `xml:"Translator"`
    Publisher   string       `xml:"Publisher"`
    Imprint     string       `xml:"Imprint"`
    Genre       string       `xml:"Genre"`
    Tags        string       `xml:"Tags"`
    Web         string       `xml:"Web"`
    Summary     string       `xml:"Summary"`
    Year        string       `xml:"Year"`
    Month       string       `xml:"Month"`
    Day         string       `xml:"Day"`
    PageCount   int          `xml:"PageCount"`
    Pages       *cbzPages    `xml:"Pages"`
}

// Author role constants (pkg/models/author.go)
const (
    AuthorRoleWriter      = "writer"
    AuthorRolePenciller   = "penciller"
    AuthorRoleInker       = "inker"
    AuthorRoleColorist    = "colorist"
    AuthorRoleLetterer    = "letterer"
    AuthorRoleCoverArtist = "cover_artist"
    AuthorRoleEditor      = "editor"
    AuthorRoleTranslator  = "translator"
)
```

### Key Functions

```go
// Parse CBZ metadata
func Parse(path string) (*mediafile.ParsedMetadata, error)

// Generate CBZ with updated metadata (atomic write)
func (g *CBZGenerator) Generate(ctx context.Context, src, dest string, book *models.Book, file *models.File) error
```

## Image Formats

Supported page image formats:

- PNG (`.png`)
- JPEG (`.jpg`, `.jpeg`)
- WebP (`.webp`)
- GIF (`.gif`)

Images are sorted **naturally** (page2 < page10) to determine page order.

## Scanner Integration

**File Discovery:**
- Extension: `.cbz`
- MIME type: `application/zip`

**CBZ-Specific Processing:**
- Volume indicator removal from title during normalization
- Series inference from title if not in metadata or sidecar
- Cover page index tracked in `metadata.CoverPage`

**Metadata Priority:**
```
Priority 0: Manual
Priority 1: Sidecar
Priority 2: CBZ Metadata
Priority 3: Filepath
```

## KePub Conversion

CBZ files can be converted to fixed-layout KePub for Kobo devices. See `kepub.md` skill for details.

**Quick Reference:**
- Converts to fixed-layout EPUB with `rendition:layout="pre-paginated"`
- Images processed: PNG→JPEG conversion, resizing for Kobo screen (1264x1680)
- Grayscale detection for manga optimization
- All metadata preserved in OPF

## Edge Cases

**Multiple Authors Per Role:**
- Multiple authors in same role comma-separated in ComicInfo
- During parsing, split and stored with same role value

**Authors Without Role:**
- Default to Writer field during generation
- Parsed as writer role if found in Writer field

**Decimal Series Numbers:**
- `1.5` formatted as "1.5" (string)
- `1.0` formatted as "1" (no decimal)

**Missing ComicInfo.xml:**
- Generator creates new ComicInfo.xml with all metadata
- Parser returns metadata from images only (page count, cover)

## Chapter Detection

CBZ files can have chapters detected from their structure. Chapters are stored in the database and accessible via API.

### Detection Strategy (2-phase)

**Phase 1: Folder-based Detection** (`pkg/cbz/chapters.go`)
- Groups pages by their immediate parent directory
- Each unique folder becomes a chapter
- Chapter title = folder name
- Chapter start page = first page in that folder
- Only triggers if 2+ distinct folders exist

**Phase 2: Filename Pattern Detection** (fallback)
- Regex pattern: `(?i)ch(?:apter)?[\s_-]*(\d+)`
- Matches: `ch01`, `chapter_02`, `Ch-3`, `CHAPTER 10`
- Groups consecutive pages with same chapter number
- Chapter title = "Chapter N"

**Detection Priority:**
1. Try folder-based detection first
2. If no folders found (all pages in root), try filename patterns
3. If neither works, no chapters detected

### Key Functions

```go
// Detect chapters from CBZ file structure
func DetectChapters(files []string) []mediafile.ParsedChapter

// Internal: folder-based detection
func detectChaptersFromFolders(files []string) []mediafile.ParsedChapter

// Internal: filename pattern detection
func detectChaptersFromFilenames(files []string) []mediafile.ParsedChapter
```

### Chapter Data Structure

```go
type ParsedChapter struct {
    Title     string
    StartPage *int  // 0-indexed page number for CBZ
}
```

## Frontend Display Conventions

**Page numbers are 1-indexed in the UI.** While the backend and data layer use 0-indexed page numbers, all user-facing displays should show 1-indexed values:

- "Page 1" instead of "Page 0"
- Page input fields accept 1-indexed values and convert to 0-indexed for storage
- Page picker shows 1-indexed labels on thumbnails
- "Uncovered pages" warnings use 1-indexed ranges (e.g., "Pages 1-3" not "Pages 0-2")

This applies to:
- `ChapterRow.tsx` - page display and edit inputs
- `CBZPagePicker.tsx` - page selection dialog
- `CBZReader.tsx` - reader page counter
- `FileChaptersTab.tsx` - uncovered pages warning

### Integration

- Chapters extracted during `Parse()` and included in `ParsedMetadata.Chapters`
- Worker syncs chapters to database via `chapterService.ReplaceChapters()`
- API: `GET /books/files/:id/chapters` returns chapter list
- API: `PUT /books/files/:id/chapters` allows manual chapter editing

## Related Files

- `pkg/cbz/cbz.go` - CBZ parsing
- `pkg/cbz/chapters.go` - Chapter detection logic
- `pkg/cbz/chapters_test.go` - Chapter detection tests
- `pkg/cbz/types.go` - ComicInfo types
- `pkg/filegen/cbz.go` - CBZ generation
- `pkg/filegen/cbz_test.go` - CBZ generation tests
- `pkg/kepub/cbz.go` - CBZ to KePub conversion
- `pkg/kepub/cbz_test.go` - CBZ conversion tests
- `pkg/models/author.go` - Author role constants
