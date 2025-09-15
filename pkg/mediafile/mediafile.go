package mediafile

import "fmt"

type ParsedMetadata struct {
	Title         string
	Authors       []string
	Series        string
	SeriesNumber  *float64
	CoverMimeType string
	CoverData     []byte
	// DataSource should be a value of books.DataSource
	DataSource string
}

func (m *ParsedMetadata) String() string {
	return fmt.Sprintf("Title:           %s\nAuthor(s):       %v\nHas Cover Data:  %v\nCover Mime Type: %s\nData Source:     %s", m.Title, m.Authors, len(m.CoverData) > 0, m.CoverMimeType, m.DataSource)
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
