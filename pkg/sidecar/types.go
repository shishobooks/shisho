package sidecar

// CurrentVersion is the current version of the sidecar file format.
// Increment this when making breaking changes to the schema.
const CurrentVersion = 1

// BookSidecar represents the metadata sidecar for a book.
// This is stored as {bookname}.metadata.json in the book directory.
type BookSidecar struct {
	Version   int              `json:"version"`
	Title     string           `json:"title,omitempty"`
	SortTitle string           `json:"sort_title,omitempty"`
	Subtitle  *string          `json:"subtitle,omitempty"`
	Authors   []AuthorMetadata `json:"authors,omitempty"`
	Series    []SeriesMetadata `json:"series,omitempty"`
}

// FileSidecar represents the metadata sidecar for a media file.
// This is stored as {filename}.metadata.json alongside the media file.
type FileSidecar struct {
	Version   int                `json:"version"`
	Narrators []NarratorMetadata `json:"narrators,omitempty"`
}

// AuthorMetadata represents an author in the sidecar file.
type AuthorMetadata struct {
	Name      string `json:"name"`
	SortName  string `json:"sort_name,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// NarratorMetadata represents a narrator in the sidecar file.
type NarratorMetadata struct {
	Name      string `json:"name"`
	SortName  string `json:"sort_name,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// SeriesMetadata represents series information in the sidecar file.
type SeriesMetadata struct {
	Name      string   `json:"name"`
	SortName  string   `json:"sort_name,omitempty"`
	Number    *float64 `json:"number,omitempty"`
	SortOrder int      `json:"sort_order,omitempty"`
}
