package books

type ListBooksQuery struct {
	Limit     int      `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int     `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	SeriesID  *int     `query:"series_id" json:"series_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string  `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	FileTypes []string `query:"file_types" json:"file_types,omitempty"` // Filter by file types (e.g., ["epub", "m4b"])
	GenreIDs  []int    `query:"genre_ids" json:"genre_ids,omitempty"`   // Filter by genre IDs
	TagIDs    []int    `query:"tag_ids" json:"tag_ids,omitempty"`       // Filter by tag IDs
}

type UpdateBookPayload struct {
	Title       *string       `json:"title,omitempty" validate:"omitempty,max=300"`
	SortTitle   *string       `json:"sort_title,omitempty" validate:"omitempty,max=300"`
	Subtitle    *string       `json:"subtitle,omitempty" validate:"omitempty,max=500"`
	Description *string       `json:"description,omitempty" validate:"omitempty,max=10000"`
	Authors     []AuthorInput `json:"authors,omitempty"`
	Series      []SeriesInput `json:"series,omitempty"`
	Genres      []string      `json:"genres,omitempty" validate:"omitempty,dive,max=100"` // Genre names
	Tags        []string      `json:"tags,omitempty" validate:"omitempty,dive,max=100"`   // Tag names
}

// AuthorInput represents an author with an optional role (for CBZ files).
type AuthorInput struct {
	Name string  `json:"name" validate:"required,max=200"`
	Role *string `json:"role,omitempty" validate:"omitempty,oneof=writer penciller inker colorist letterer cover_artist editor translator"`
}

// SeriesInput represents a series association with optional number.
type SeriesInput struct {
	Name   string   `json:"name" validate:"required,max=200"`
	Number *float64 `json:"number,omitempty"`
}

// IdentifierPayload represents an identifier in update requests.
type IdentifierPayload struct {
	Type  string `json:"type" validate:"required,oneof=isbn_10 isbn_13 asin uuid goodreads google other"`
	Value string `json:"value" validate:"required,max=100"`
}

// UpdateFilePayload is the payload for updating a file's metadata.
type UpdateFilePayload struct {
	Narrators   []string             `json:"narrators,omitempty" validate:"omitempty,dive,max=200"`
	URL         *string              `json:"url,omitempty" validate:"omitempty,max=500,url"`
	Publisher   *string              `json:"publisher,omitempty" validate:"omitempty,max=200"`
	Imprint     *string              `json:"imprint,omitempty" validate:"omitempty,max=200"`
	ReleaseDate *string              `json:"release_date,omitempty" validate:"omitempty"` // ISO 8601 date string
	Identifiers *[]IdentifierPayload `json:"identifiers,omitempty" validate:"omitempty,dive"`
}
