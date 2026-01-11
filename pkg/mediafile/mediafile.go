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

type ParsedMetadata struct {
	Title         string
	Subtitle      string // from M4B freeform SUBTITLE atom
	Authors       []ParsedAuthor
	Narrators     []string
	Series        string
	SeriesNumber  *float64
	Genres        []string // Genre names from file metadata
	Tags          []string // Tag names from file metadata
	CoverMimeType string
	CoverData     []byte
	CoverPage     *int // 0-indexed page number for CBZ cover, nil for other file types
	// DataSource should be a value of books.DataSource
	DataSource string
	// Duration is the length of the audiobook (M4B files only)
	Duration time.Duration
	// BitrateBps is the audio bitrate in bits per second (M4B files only)
	BitrateBps int
	// PageCount is the number of pages (CBZ files only)
	PageCount *int
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
