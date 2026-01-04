package books

type ListBooksQuery struct {
	Limit     int      `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int      `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int     `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	SeriesID  *int     `query:"series_id" json:"series_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string  `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	FileTypes []string `query:"file_types" json:"file_types,omitempty"` // Filter by file types (e.g., ["epub", "m4b"])
}

type UpdateBookPayload struct {
	Title     *string       `json:"title,omitempty" validate:"omitempty,max=300"`
	SortTitle *string       `json:"sort_title,omitempty" validate:"omitempty,max=300"`
	Subtitle  *string       `json:"subtitle,omitempty" validate:"omitempty,max=500"`
	Authors   []string      `json:"authors,omitempty" validate:"omitempty,dive,max=200"`
	Series    []SeriesInput `json:"series,omitempty"`
}

// SeriesInput represents a series association with optional number.
type SeriesInput struct {
	Name   string   `json:"name" validate:"required,max=200"`
	Number *float64 `json:"number,omitempty"`
}

// UpdateFilePayload is the payload for updating a file's metadata.
type UpdateFilePayload struct {
	Narrators []string `json:"narrators,omitempty" validate:"omitempty,dive,max=200"`
}
