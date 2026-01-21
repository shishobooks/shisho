---
name: m4b
description: You MUST use this before working on anything M4B/audiobook-related (e.g. parsing, generation, MP4 atom metadata, etc.). Covers iTunes atoms, chapters, narrators, and the mp4/filegen packages.
---

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
  mvhd                    # Movie header (duration, timescale)
  trak                    # Track box (audio)
    tref/chap             # Chapter track reference (QuickTime chapters)
  udta                    # User data box
    meta                  # Metadata box
      ilst                # Item list (iTunes-style metadata)
        ©nam              # Title
        ©ART              # Artist (authors)
        ©nrt              # Narrator (dedicated audiobook atom)
        ©cmp              # Composer (fallback narrator)
        ©wrt              # Writer (fallback narrator)
        ©alb              # Album (series)
        ©gen              # Genre (text form)
        ©pub              # Publisher
        ©grp              # Grouping (series info)
        gnre              # Genre ID (ID3v1 style)
        desc              # Description
        covr              # Cover art
        stik              # Media type (2 = audiobook)
        rldt              # Release date (Audible format)
        ----              # Freeform atoms (custom metadata)
    chpl                  # Nero chapter list (alternative)
mdat                      # Media data box (audio)
```

## iTunes-Style Atoms

### Standard Atoms

| Atom | Code | Shisho Usage |
|------|------|--------------|
| `©nam` | Title | Book title |
| `©ART` | Artist | Authors (comma/semicolon-separated) |
| `©nrt` | Narrator | Narrators (preferred source) |
| `©cmp` | Composer | Narrators (fallback 1) |
| `©wrt` | Writer | Narrators (fallback 2) |
| `©alb` | Album | "Series Name #N" format |
| `©gen` | Genre | Genres (comma-separated text) |
| `gnre` | Genre ID | Genre by ID3v1 index (1-based) |
| `©day` | Year | Publication year |
| `©cmt` | Comment | Description (alternative) |
| `desc` | Description | Description (preferred) |
| `©pub` | Publisher | Publisher name |
| `©grp` | Grouping | Series info (parsed for name/number) |
| `©cpy` | Copyright | Copyright notice |
| `©too` | Encoder | Encoding tool info |
| `covr` | Cover art | Cover image data (JPEG/PNG/BMP) |
| `stik` | Media type | Value 2 = audiobook |
| `rldt` | Release date | Audible format (ISO 8601) |

### Freeform Atoms (`----`)

Custom metadata uses freeform atoms with namespace:

```
----:com.apple.iTunes:SUBTITLE     # Subtitle (preferred)
----:com.pilabor.tone:SUBTITLE     # Subtitle (fallback, Tone audiobook player)
----:com.apple.iTunes:SERIES       # Series name (Audible style)
----:com.apple.iTunes:SERIES-PART  # Series number (Audible style)
----:com.shisho:tags               # Tags (comma-separated)
----:com.shisho:imprint            # Imprint name
----:com.shisho:url                # URL
```

Freeform atom structure:

```
----
  mean                    # Namespace (e.g., "com.apple.iTunes")
  name                    # Key name (e.g., "SUBTITLE")
  data                    # Value with type flag
```

## Shisho Implementation

### Parsing (`pkg/mp4/`)

**Core Files:**
- `mp4.go` - Public API entry points
- `metadata.go` - Metadata structures and conversion
- `reader.go` - Metadata extraction logic
- `atoms.go` - Atom type definitions and data parsing
- `chapters.go` - Chapter extraction logic

**All Metadata Fields Extracted:**

| Field | Source | Notes |
|-------|--------|-------|
| Title | `©nam` | Direct extraction |
| Subtitle | `com.apple.iTunes:SUBTITLE` → `com.pilabor.tone:SUBTITLE` | Fallback chain |
| Authors | `©ART` | Split by comma/semicolon |
| Narrators | `©nrt` → `©cmp` → `©wrt` | Fallback chain |
| Series Name | Album or `©grp` | Parsed from album format |
| Series Number | Album or `©grp` | Extracted by regex patterns |
| Genres | `©gen` | Comma-separated text |
| Tags | `com.shisho:tags` | Freeform atom, comma-separated |
| Description | `desc` or `©cmt` | desc preferred |
| Publisher | `©pub` | Direct extraction |
| Imprint | `com.shisho:imprint` | Freeform atom |
| URL | `com.shisho:url` | Freeform atom |
| Release Date | `rldt` or `©day` | ISO 8601 or year |
| Duration | `mvhd` box | Calculated from timescale |
| Bitrate | `esds` box | From AvgBitrate field |
| Chapters | `chpl` or `tref/chap` | Nero or QuickTime format |
| Cover | `covr` atom | With MIME type detection |
| Media Type | `stik` | Value 2 = audiobook |

**Series Parsing from Album:**
Regex patterns extract series from album field:
```
"Series Name #N"           → Series: "Series Name", Number: N
"Series Name, Book N"      → Series: "Series Name", Number: N
"Series Name - Volume N"   → Series: "Series Name", Number: N
"Series Name (Book N)"     → Series: "Series Name", Number: N
```
Supports decimal numbers (e.g., "3.5").

**Narrator Fallback Chain:**
1. `©nrt` (dedicated narrator atom) - preferred
2. `©cmp` (composer) - common in FFmpeg-generated files
3. `©wrt` (writer) - fallback

**Cover Format Detection:**
1. Explicit type (JPEG=13, PNG=14, BMP=27)
2. Magic byte detection fallback:
   - JPEG: `FF D8 FF`
   - PNG: `89 50 4E 47 0D 0A 1A 0A`
   - BMP: `42 4D`

**Chapter Extraction Priority:**
1. QuickTime chapters (`tref/chap` track reference)
2. Nero chapters (`chpl` in udta) - fallback

**Data Source:** All extracted metadata tagged with `models.DataSourceM4BMetadata` (priority 3)

### Generation (`pkg/filegen/m4b.go`)

Uses `mp4.WriteToFile()` for atomic writes.

**Metadata Written Back:**

| Field | Atom | Source |
|-------|------|--------|
| Title | `©nam` | book.Title |
| Authors | `©ART` | Joined author names |
| Narrators | `©nrt` AND `©cmp` | Written to both for compatibility |
| Album | `©alb` | "Series Name #N" format |
| Subtitle | `com.apple.iTunes:SUBTITLE` | book.Subtitle |
| Genres | `©gen` | Comma-separated |
| Tags | `com.shisho:tags` | Comma-separated |
| Description | `desc` | book.Description |
| Publisher | `©pub` | file.Publisher.Name |
| Imprint | `com.shisho:imprint` | file.Imprint.Name |
| URL | `com.shisho:url` | file.URL |
| Cover | `covr` | Image with type flag (13=JPEG, 14=PNG) |

**Preserved from Source:**
- Description, comments, year, copyright, encoder
- Duration, bitrate
- Chapters
- Unknown atoms (e.g., `aART`, `cprt`)
- All freeform atoms not explicitly overwritten

**Series Formatting:**
- With number: `"Series Name #1"` or `"Series Name #1.5"`
- Without number: `"Series Name"`

### Key Types

```go
// Parsed M4B metadata (pkg/mp4/metadata.go)
type Metadata struct {
    Title        string
    Subtitle     string
    Album        string
    Genre        string           // Single genre (legacy)
    Genres       []string         // All genres
    Description  string
    Comment      string
    Year         string
    Copyright    string
    Encoder      string
    Publisher    string
    Imprint      string
    URL          string
    Authors      []ParsedAuthor
    Narrators    []string
    Series       string
    SeriesNumber *float64
    Tags         []string
    Duration     time.Duration
    Bitrate      int              // bits per second
    MediaType    int              // 2 = audiobook
    Chapters     []Chapter
    CoverData    []byte
    CoverMimeType string
    Freeform     map[string]string  // All freeform atoms
    UnknownAtoms []RawAtom          // Preserved unrecognized atoms
}

// Chapter information
type Chapter struct {
    Title string
    Start time.Duration
    End   time.Duration
}

// Raw atom for preservation
type RawAtom struct {
    Type [4]byte  // 4-byte atom type code
    Data []byte   // Complete atom data
}
```

### Key Functions

```go
// Parse basic metadata for scanning
func Parse(path string) (*mediafile.ParsedMetadata, error)

// Parse full metadata including chapters, duration, unknown atoms
func ParseFull(path string) (*Metadata, error)

// Modify file in place with optional backup
func Write(path string, metadata *Metadata, opts WriteOptions) error

// Atomic write to new file (temp file + rename)
func WriteToFile(srcPath, destPath string, metadata *Metadata) error

// Generate M4B with updated metadata
func (g *M4BGenerator) Generate(ctx, srcPath, destPath string, book *models.Book, file *models.File) error
```

## Data Type Handling

```go
const (
    DataTypeUTF8     = 1   // Standard text (most common)
    DataTypeUTF16BE  = 2   // UTF-16 big-endian
    DataTypeJPEG     = 13  // JPEG image
    DataTypePNG      = 14  // PNG image
    DataTypeGenre    = 18  // Special genre type (handled specially)
    DataTypeInteger  = 21  // Signed big-endian integer
    DataTypeBMP      = 27  // BMP image
)
```

**Data Type 18 Genre Handling:**
Some M4B files use data type 18 for genre text. The parser handles this:
```go
case DataTypeUTF8, DataTypeGenre:  // Both types contain UTF-8
    return string(value)
```

## Chapter Formats

### QuickTime Chapters
- Path: `moov → trak → tref/chap` (chapter track reference)
- Uses dedicated text track with timing information
- Complex extraction via sample table (stts, stsz, stsc, stco/co64)

### Nero Chapters
- Path: `moov → udta → chpl` (chapter list)
- Format: `[version][flags][count][entries...]`
- Entry: `[8 bytes timestamp in 100ns][1 byte title length][title]`
- Simpler format, more commonly used

## Scanner Integration

**File Discovery:**
- Extension: `.m4b`
- MIME types: `audio/x-m4a`, `video/mp4`

**Fallback to Filename:**
If no authors in metadata, extracts from filename using `[author names]` pattern.

**Metadata Priority:**
```
Priority 0: Manual
Priority 1: Sidecar
Priority 2: Existing Cover
Priority 3: M4B Metadata
Priority 4: Filepath
```

## Sidecar Handling

**FileSidecar Fields for M4B:**
```go
type FileSidecar struct {
    Version     int
    Narrators   []NarratorMetadata  // Audio-specific
    URL         *string
    Publisher   *string
    Imprint     *string
    ReleaseDate *string             // ISO 8601
}
```

**NOT Stored in Sidecars:**
- Duration (intrinsic to file)
- Bitrate (intrinsic to file)
- Cover image (intrinsic to file)

## Edge Cases

**Narrator Compatibility:**
Narrators written to BOTH `©nrt` AND `©cmp` for maximum compatibility across players.

**Unknown Atom Preservation:**
Custom atoms like `aART` (album artist), `cprt` (copyright) are preserved byte-for-byte through round-trips.

**Series Parsing Robustness:**
- Multiple format patterns supported
- Decimal numbers handled (e.g., "3.5")
- Whitespace trimmed from extracted names
- Empty string returned if no pattern matches

**Atomic Write Pattern:**
```
1. Read source file
2. Modify in-memory
3. Write to destPath.tmp
4. Rename .tmp → destPath (atomic on POSIX)
5. On error, cleanup .tmp
```

**Overflow Safety:**
- File offsets clamped to prevent int64 overflow
- Box sizes limited to prevent allocation bombs
- UTF-16 decoding includes null terminator handling

## Related Files

- `pkg/mp4/mp4.go` - Public API
- `pkg/mp4/metadata.go` - Metadata types and conversion
- `pkg/mp4/reader.go` - Atom reading logic
- `pkg/mp4/writer.go` - Atom writing logic
- `pkg/mp4/atoms.go` - Atom type definitions
- `pkg/mp4/chapters.go` - Chapter extraction
- `pkg/mp4/mp4_test.go` - M4B parsing tests
- `pkg/filegen/m4b.go` - M4B generation
- `pkg/filegen/m4b_test.go` - M4B generation tests
- `internal/testgen/m4b.go` - Test file generation

## Debug Tools

- `cmd/scripts/debug/print-mp4-atoms/` - CLI tool to inspect M4B metadata and atoms
  - Run with: `go run ./cmd/scripts/debug/print-mp4-atoms <file.m4b>`
  - Shows all parsed metadata, freeform atoms, unknown atoms, and chapters

## Browser Playback (Frontend Audio Preview)

### Codec Compatibility Issue

**Problem:** Some M4B files fail to play in the browser audio preview, getting stuck in a seeking state.

**Root Cause:** The AAC codec profile determines browser compatibility:

| Profile | Browser Support | Seeking Support |
|---------|-----------------|-----------------|
| **AAC-LC** (Low Complexity) | Full | Works |
| **HE-AAC** (High Efficiency) | Full | Works |
| **xHE-AAC** (Extended HE) | Partial | **Fails** |

Browsers report `canPlayType('audio/mp4; codecs="mp4a.40.42"')` as "probably" for xHE-AAC, but actually fail when seeking to arbitrary positions. The audio gets stuck with:
- `seeking: true` (never completes)
- `readyState: 1` (HAVE_METADATA only)
- `canplay` event never fires

### Diagnosing Codec Issues

Use `ffprobe` to check the AAC profile:

```bash
ffprobe -v error -show_streams <file.m4b> | grep profile
```

**Output comparison:**
```
# Working file
profile=LC

# Problematic file
profile=xHE-AAC
```

### Frontend Fix Pattern

When implementing audio preview for M4B files, use timeouts to handle codec incompatibility gracefully:

```typescript
const handleAudioPlay = (timestampMs: number) => {
  const audio = audioRef.current;
  audio.currentTime = timestampMs / 1000;

  let playbackStarted = false;

  const tryPlay = () => {
    if (audio.readyState >= 3) {
      playbackStarted = true;
      audio.play();
    } else {
      // Wait for canplay, with timeout fallback
      const onCanPlay = () => {
        playbackStarted = true;
        audio.removeEventListener("canplay", onCanPlay);
        audio.play();
      };
      audio.addEventListener("canplay", onCanPlay);

      // Timeout: codec may not be supported
      setTimeout(() => {
        if (!playbackStarted) {
          audio.removeEventListener("canplay", onCanPlay);
          setPlayingState(null);
          toast.error("Unable to play audio preview", {
            description: "The audio codec may not be supported by your browser.",
          });
        }
      }, 5000);
    }
  };

  // Also timeout if seeking never completes
  if (audio.seeking) {
    const onSeeked = () => {
      audio.removeEventListener("seeked", onSeeked);
      tryPlay();
    };
    audio.addEventListener("seeked", onSeeked);

    setTimeout(() => {
      if (!playbackStarted && audio.seeking) {
        audio.removeEventListener("seeked", onSeeked);
        setPlayingState(null);
        toast.error("Unable to play audio preview", {
          description: "The audio codec may not be supported by your browser.",
        });
      }
    }, 5000);
  } else {
    tryPlay();
  }
};
```

**Key points:**
1. Don't call `play()` until `readyState >= 3` (HAVE_FUTURE_DATA)
2. Add timeout for both `seeked` and `canplay` events
3. Show user-friendly error when timeout triggers
4. Reset UI state so button doesn't stay in "playing" mode

### Implementation Reference

See `app/components/files/FileChaptersTab.tsx`:
- `handleAudioPlay` - Playback with timeout handling
- `handleAudioStop` - Cleanup and state reset
