# CBZ Format Reference

This skill documents the CBZ (Comic Book ZIP) format as used in Shisho for parsing and generation.

## File Structure

CBZ files are ZIP archives containing:

```
ComicInfo.xml             # Metadata file (optional but common)
000.png                   # Page images (PNG, JPG, WEBP)
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
  <Year>2024</Year>
  <Publisher>Publisher Name</Publisher>
  <Summary>Description text</Summary>
  <PageCount>24</PageCount>
</ComicInfo>
```

### Creator Roles

CBZ uses separate fields for each creator role:

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
  <Page Image="2"/>
</Pages>
```

Page types: `FrontCover`, `InnerCover`, `Roundup`, `Story`, `Advertisement`, `Editorial`, `Letters`, `Preview`, `BackCover`, `Other`, `Deleted`

## Shisho Implementation

### Parsing (`pkg/cbz/cbz.go`)

Extracts metadata from ComicInfo.xml:

- Title, Series, Number
- All creator roles mapped to authors with roles
- Genre field parsed as comma-separated genres
- Tags field parsed as comma-separated tags
- Page count from image files

### Generation (`pkg/filegen/cbz.go`)

When generating CBZ files, Shisho:

1. Preserves all original page images unchanged
2. Creates/updates ComicInfo.xml with:
   - Title, Series, Number from book model
   - Authors mapped to appropriate role fields
   - Genres as comma-separated Genre field
   - Tags as comma-separated Tags field
   - Page information with FrontCover type on cover page

### Key Types

```go
// ComicInfo XML structure
type cbzComicInfo struct {
    Title       string       `xml:"Title"`
    Series      string       `xml:"Series"`
    Number      string       `xml:"Number"`
    Writer      string       `xml:"Writer"`
    Penciller   string       `xml:"Penciller"`
    // ... other creator fields
    Genre       string       `xml:"Genre"`
    Tags        string       `xml:"Tags"`
    Pages       *cbzPages    `xml:"Pages"`
}

// Author role constants
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

// Generate CBZ with updated metadata
func (g *CBZGenerator) Generate(ctx context.Context, src, dest string, book *models.Book, file *models.File) error
```

## Image Formats

Supported page image formats:

- PNG (`.png`)
- JPEG (`.jpg`, `.jpeg`)
- WebP (`.webp`)
- GIF (`.gif`)

Images are sorted alphanumerically to determine page order.

## Related Files

- `pkg/cbz/cbz.go` - CBZ parsing
- `pkg/cbz/types.go` - ComicInfo types
- `pkg/filegen/cbz.go` - CBZ generation
- `pkg/filegen/cbz_test.go` - CBZ generation tests
- `pkg/models/author.go` - Author role constants
