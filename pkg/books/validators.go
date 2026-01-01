package books

type ListBooksQuery struct {
	Limit     int  `query:"limit" json:"limit,omitempty" default:"25" validate:"min=1,max=100"`
	Offset    int  `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	SeriesID  *int `query:"series_id" json:"series_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
}

type UpdateBookPayload struct {
	Title *string `json:"title,omitempty" validate:"omitempty,max=300"`
}
