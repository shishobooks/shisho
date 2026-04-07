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
	Name string `json:"name"`
	Role string `json:"role"` // empty for generic author, or one of: writer, penciller, inker, colorist, letterer, cover_artist, editor, translator
}

// ParsedIdentifier represents an identifier parsed from file metadata.
type ParsedIdentifier struct {
	Type  string `json:"type"`  // One of the IdentifierType constants (isbn_10, isbn_13, asin, uuid, goodreads, google, other)
	Value string `json:"value"` // The identifier value
}

// ParsedChapter represents a chapter parsed from file metadata.
// Position fields are mutually exclusive based on file type.
type ParsedChapter struct {
	Title            string          `json:"title"`
	StartPage        *int            `json:"start_page,omitempty"`         // CBZ: 0-indexed page number
	StartTimestampMs *int64          `json:"start_timestamp_ms,omitempty"` // M4B: milliseconds from start
	Href             *string         `json:"href,omitempty"`               // EPUB: content document href
	Children         []ParsedChapter `json:"children,omitempty"`           // EPUB nesting only; CBZ/M4B always empty
}

type ParsedMetadata struct {
	Title         string         `json:"title"`
	Subtitle      string         `json:"subtitle"` // from M4B freeform SUBTITLE atom
	Authors       []ParsedAuthor `json:"authors"`
	Narrators     []string       `json:"narrators"`
	Series        string         `json:"series"`
	SeriesNumber  *float64       `json:"series_number,omitempty"`
	Genres        []string       `json:"genres"` // Genre names from file metadata
	Tags          []string       `json:"tags"`   // Tag names from file metadata
	Description   string         `json:"description"`
	Publisher     string         `json:"publisher"`
	Imprint       string         `json:"imprint"`
	URL           string         `json:"url"`
	ReleaseDate   *time.Time     `json:"release_date,omitempty"`
	CoverMimeType string         `json:"cover_mime_type"`
	CoverURL      string         `json:"cover_url"`
	CoverData     []byte         `json:"-"`
	CoverPage     *int           `json:"cover_page,omitempty"` // 0-indexed page number for CBZ cover, nil for other file types
	// DataSource should be a value of books.DataSource
	DataSource string `json:"-"`
	// FieldDataSources maps individual field names to the data source that provided them.
	// Used when multiple enrichers contribute different fields (per-field first-wins tracking).
	// Keys are field names: "title", "subtitle", "authors", "narrators", "series",
	// "genres", "tags", "description", "publisher", "imprint", "url", "releaseDate",
	// "cover", "identifiers", "language", "abridged".
	FieldDataSources map[string]string `json:"-"`
	PluginScope      string            `json:"-"`
	PluginID         string            `json:"-"`
	// Duration is the length of the audiobook (M4B files only)
	Duration time.Duration `json:"duration"`
	// BitrateBps is the audio bitrate in bits per second (M4B files only)
	BitrateBps int `json:"bitrate_bps"`
	// Codec is the audio codec with profile (M4B files only), e.g. "AAC-LC", "xHE-AAC"
	Codec string `json:"-"`
	// Language is a BCP 47 language tag (e.g., "en", "en-US", "zh-Hans")
	Language *string `json:"language,omitempty"`
	// Abridged indicates whether this is an abridged edition
	Abridged *bool `json:"abridged,omitempty"`
	// PageCount is the number of pages (CBZ and PDF files)
	PageCount *int `json:"page_count,omitempty"`
	// Identifiers contains file identifiers (ISBN, ASIN, etc.) parsed from metadata
	Identifiers []ParsedIdentifier `json:"identifiers"`
	// Chapters contains chapter information parsed from file metadata
	Chapters []ParsedChapter `json:"chapters"`
	// Confidence is an optional score (0-1) indicating how confident the plugin
	// is that this result matches the search query. Used by the scan pipeline
	// to decide whether to auto-apply enrichment results.
	Confidence *float64 `json:"confidence,omitempty"`
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
