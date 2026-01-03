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
	Title *string `json:"title,omitempty" validate:"omitempty,max=300"`
}
