package mediafile

import (
	"fmt"
	"time"
)

type ParsedMetadata struct {
	Title         string
	Subtitle      string // from M4B freeform SUBTITLE atom
	Authors       []string
	Narrators     []string
	Series        string
	SeriesNumber  *float64
	CoverMimeType string
	CoverData     []byte
	// DataSource should be a value of books.DataSource
	DataSource string
	// Duration is the length of the audiobook (M4B files only)
	Duration time.Duration
	// BitrateBps is the audio bitrate in bits per second (M4B files only)
	BitrateBps int
}

func (m *ParsedMetadata) String() string {
	return fmt.Sprintf("Title:           %s\nAuthor(s):       %v\nNarrator(s):     %v\nHas Cover Data:  %v\nCover Mime Type: %s\nData Source:     %s", m.Title, m.Authors, m.Narrators, len(m.CoverData) > 0, m.CoverMimeType, m.DataSource)
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
