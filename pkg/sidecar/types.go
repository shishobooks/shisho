package sidecar

// CurrentVersion is the current version of the sidecar file format.
// Increment this when making breaking changes to the schema.
const CurrentVersion = 1

// BookSidecar represents the metadata sidecar for a book.
// This is stored as {bookname}.metadata.json in the book directory.
type BookSidecar struct {
	Version     int              `json:"version"`
	Title       string           `json:"title,omitempty"`
	SortTitle   string           `json:"sort_title,omitempty"`
	Subtitle    *string          `json:"subtitle,omitempty"`
	Description *string          `json:"description,omitempty"`
	Authors     []AuthorMetadata `json:"authors,omitempty"`
	Series      []SeriesMetadata `json:"series,omitempty"`
	Genres      []string         `json:"genres,omitempty"`
	Tags        []string         `json:"tags,omitempty"`
}

// FileSidecar represents the metadata sidecar for a media file.
// This is stored as {filename}.metadata.json alongside the media file.
type FileSidecar struct {
	Version     int                  `json:"version"`
	Narrators   []NarratorMetadata   `json:"narrators,omitempty"`
	URL         *string              `json:"url,omitempty"`
	Publisher   *string              `json:"publisher,omitempty"`
	Imprint     *string              `json:"imprint,omitempty"`
	ReleaseDate *string              `json:"release_date,omitempty"` // ISO 8601 date string (YYYY-MM-DD)
	Identifiers []IdentifierMetadata `json:"identifiers,omitempty"`
	Name        *string              `json:"name,omitempty"`
	Chapters    []ChapterMetadata    `json:"chapters,omitempty"`
	CoverPage   *int                 `json:"cover_page,omitempty"` // 0-indexed page number for CBZ cover
}

// AuthorMetadata represents an author in the sidecar file.
type AuthorMetadata struct {
	Name      string  `json:"name"`
	SortName  string  `json:"sort_name,omitempty"`
	SortOrder int     `json:"sort_order,omitempty"`
	Role      *string `json:"role,omitempty"` // CBZ creator role: writer, penciller, inker, etc.
}

// NarratorMetadata represents a narrator in the sidecar file.
type NarratorMetadata struct {
	Name      string `json:"name"`
	SortName  string `json:"sort_name,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// IdentifierMetadata represents an identifier in the sidecar file.
type IdentifierMetadata struct {
	Type  string `json:"type"` // isbn_10, isbn_13, asin, uuid, goodreads, google, other
	Value string `json:"value"`
}

// SeriesMetadata represents series information in the sidecar file.
type SeriesMetadata struct {
	Name      string   `json:"name"`
	SortName  string   `json:"sort_name,omitempty"`
	Number    *float64 `json:"number,omitempty"`
	SortOrder int      `json:"sort_order,omitempty"`
}

// ChapterMetadata represents a chapter in the sidecar file.
// Position fields are mutually exclusive based on file type:
// - CBZ uses StartPage (0-indexed).
// - M4B uses StartTimestampMs.
// - EPUB uses Href.
type ChapterMetadata struct {
	Title            string            `json:"title"`
	StartPage        *int              `json:"start_page,omitempty"`
	StartTimestampMs *int64            `json:"start_timestamp_ms,omitempty"`
	Href             *string           `json:"href,omitempty"`
	Children         []ChapterMetadata `json:"children,omitempty"`
}
