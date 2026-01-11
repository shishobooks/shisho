# M4B Format Reference

This skill documents the M4B (MPEG-4 Audiobook) format as used in Shisho for parsing and generation.

## File Structure

M4B files are MP4 containers with:

- AAC audio stream(s)
- Metadata atoms in the `moov` box
- Optional chapter markers
- Optional cover art

## MP4 Atom Structure

```
ftyp                      # File type box
moov                      # Movie box (metadata container)
  mvhd                    # Movie header
  trak                    # Track box (audio)
  udta                    # User data box
    meta                  # Metadata box
      ilst                # Item list (iTunes-style metadata)
        ©nam              # Title
        ©ART              # Artist (authors)
        ©wrt              # Composer (narrators)
        ©alb              # Album (series)
        ©gen              # Genre
        covr              # Cover art
        ----              # Freeform atoms (custom metadata)
mdat                      # Media data box (audio)
```

## iTunes-Style Atoms

### Standard Atoms

| Atom    | Description      | Shisho Usage                |
| ------- | ---------------- | --------------------------- |
| `©nam` | Title            | Book title                  |
| `©ART` | Artist           | Authors (comma-separated)   |
| `©wrt` | Composer         | Narrators (comma-separated) |
| `©alb` | Album            | "Series Name #N" format     |
| `©gen` | Genre            | Genres (comma-separated)    |
| `©day` | Year             | Publication year            |
| `©cmt` | Comment          | Description                 |
| `desc`  | Description      | Description (alternative)   |
| `ldes`  | Long description | Description (iTunes)        |
| `covr`  | Cover art        | Cover image data            |
| `trkn`  | Track number     | Series index                |

### Freeform Atoms

Custom metadata uses freeform atoms (`----`):

```
----:com.apple.iTunes:SUBTITLE     # Subtitle
----:com.apple.iTunes:SERIES       # Series name (Audible style)
----:com.apple.iTunes:SERIES-PART  # Series number (Audible style)
----:com.shisho:tags               # Tags (comma-separated)
```

Freeform atom structure:

```
----
  mean                    # Namespace (e.g., "com.apple.iTunes")
  name                    # Key name (e.g., "SUBTITLE")
  data                    # Value with type flag
```

## Shisho Implementation

### Parsing (`pkg/mp4/metadata.go`)

Extracts:

- Title from `©nam`
- Authors from `©ART` (split by comma/semicolon)
- Narrators from `©wrt` (split by comma/semicolon)
- Series from Audible freeform atoms or album parsing
- Genre from `©gen` (primary genre)
- Genres from `©gen` if comma-separated
- Tags from `----:com.shisho:tags` freeform atom
- Cover from `covr` atom

### Generation (`pkg/filegen/m4b.go`)

Uses AtomicParsley to write metadata:

1. Copies source file to destination
2. Builds AtomicParsley command with metadata flags
3. Writes:
   - Title, Authors, Narrators
   - Album as "Series Name #N"
   - Series freeform atoms
   - Genres as comma-separated `©gen`
   - Tags as freeform `----:com.shisho:tags`
   - Cover image if available

### Key Types

```go
// Parsed M4B metadata
type M4BMetadata struct {
    Title        string
    Authors      []AuthorInfo
    Narrators    []string
    Series       string
    SeriesNumber *float64
    Album        string
    Genre        string      // Single genre (legacy)
    Genres       []string    // All genres
    Tags         []string    // Custom tags
    Description  string
    Subtitle     string
    CoverData    []byte
    CoverMimeType string
    Duration     time.Duration
    Bitrate      int
    Chapters     []Chapter
}

// Chapter information
type Chapter struct {
    Title string
    Start time.Duration
    End   time.Duration
}
```

### Key Functions

```go
// Parse full M4B metadata
func ParseFull(path string) (*M4BMetadata, error)

// Parse basic metadata for scanning
func Parse(path string) (*mediafile.ParsedMetadata, error)

// Generate M4B with updated metadata
func (g *M4BGenerator) Generate(ctx context.Context, src, dest string, book *models.Book, file *models.File) error
```

## AtomicParsley Usage

Shisho uses AtomicParsley for M4B generation:

```bash
AtomicParsley input.m4b \
  --title "Book Title" \
  --artist "Author Name" \
  --composer "Narrator Name" \
  --album "Series Name #1" \
  --genre "Fantasy, Science Fiction" \
  --artwork cover.jpg \
  --freefrom "SERIES" --freeformName "com.apple.iTunes" --freeformValue "Series Name" \
  --output output.m4b
```

## Cover Art

Cover images are stored in the `covr` atom:

- Supports JPEG and PNG formats
- MIME type determined by magic bytes
- Extracted during parsing, embedded during generation

## Chapters

Chapters are stored in:

- `chpl` atom (Nero chapters)
- Or in a text track

Chapter format:

```go
type Chapter struct {
    Title string
    Start time.Duration  // Start time
    End   time.Duration  // End time
}
```

## Related Files

- `pkg/mp4/metadata.go` - M4B parsing
- `pkg/mp4/atoms.go` - Atom reading utilities
- `pkg/mp4/mp4_test.go` - M4B parsing tests
- `pkg/filegen/m4b.go` - M4B generation
- `pkg/filegen/m4b_test.go` - M4B generation tests
