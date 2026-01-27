package mediafile

import (
	"fmt"
	"strings"
	"time"
)

// ParsedAuthor represents an author with optional role information.
// Role is used for CBZ ComicInfo.xml creator types (writer, penciller, etc.).
// For EPUB/M4B files, Role will be empty (generic author).
type ParsedAuthor struct {
	Name string
	Role string // empty for generic author, or one of: writer, penciller, inker, colorist, letterer, cover_artist, editor, translator
}

// ParsedIdentifier represents an identifier parsed from file metadata.
type ParsedIdentifier struct {
	Type  string // One of the IdentifierType constants (isbn_10, isbn_13, asin, uuid, goodreads, google, other)
	Value string // The identifier value
}

// ParsedChapter represents a chapter parsed from file metadata.
// Position fields are mutually exclusive based on file type.
type ParsedChapter struct {
	Title            string
	StartPage        *int            // CBZ: 0-indexed page number
	StartTimestampMs *int64          // M4B: milliseconds from start
	Href             *string         // EPUB: content document href
	Children         []ParsedChapter // EPUB nesting only; CBZ/M4B always empty
}

type ParsedMetadata struct {
	Title         string
	Subtitle      string // from M4B freeform SUBTITLE atom
	Authors       []ParsedAuthor
	Narrators     []string
	Series        string
	SeriesNumber  *float64
	Genres        []string // Genre names from file metadata
	Tags          []string // Tag names from file metadata
	Description   string
	Publisher     string
	Imprint       string
	URL           string
	ReleaseDate   *time.Time
	CoverMimeType string
	CoverData     []byte
	CoverPage     *int // 0-indexed page number for CBZ cover, nil for other file types
	// DataSource should be a value of books.DataSource
	DataSource string
	// FieldDataSources maps individual field names to the data source that provided them.
	// Used when multiple enrichers contribute different fields (per-field first-wins tracking).
	// Keys are field names: "title", "subtitle", "authors", "narrators", "series",
	// "genres", "tags", "description", "publisher", "imprint", "url", "releaseDate",
	// "cover", "identifiers".
	FieldDataSources map[string]string
	// Duration is the length of the audiobook (M4B files only)
	Duration time.Duration
	// BitrateBps is the audio bitrate in bits per second (M4B files only)
	BitrateBps int
	// Codec is the audio codec with profile (M4B files only), e.g. "AAC-LC", "xHE-AAC"
	Codec string
	// PageCount is the number of pages (CBZ files only)
	PageCount *int
	// Identifiers contains file identifiers (ISBN, ASIN, etc.) parsed from metadata
	Identifiers []ParsedIdentifier
	// Chapters contains chapter information parsed from file metadata
	Chapters []ParsedChapter
}

func (m *ParsedMetadata) String() string {
	authorNames := make([]string, len(m.Authors))
	for i, a := range m.Authors {
		if a.Role != "" {
			authorNames[i] = fmt.Sprintf("%s (%s)", a.Name, a.Role)
		} else {
			authorNames[i] = a.Name
		}
	}
	return fmt.Sprintf("Title:           %s\nAuthor(s):       %v\nNarrator(s):     %v\nHas Cover Data:  %v\nCover Mime Type: %s\nData Source:     %s", m.Title, strings.Join(authorNames, ", "), m.Narrators, len(m.CoverData) > 0, m.CoverMimeType, m.DataSource)
}

// SourceForField returns the data source for a specific field.
// If a per-field source is set, it returns that; otherwise falls back to DataSource.
func (m *ParsedMetadata) SourceForField(field string) string {
	if m.FieldDataSources != nil {
		if src, ok := m.FieldDataSources[field]; ok {
			return src
		}
	}
	return m.DataSource
}

func (m *ParsedMetadata) CoverExtension() string {
	ext := ""
	switch m.CoverMimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	}
	return ext
}
