package books

type ListBooksQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"25" validate:"min=1,max=100"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *string `query:"library_id" json:"library_id,omitempty" validate:"omitempty,max=36" tstype:"string"`
}

type UpdateBookPayload struct {
	Title *string `json:"title,omitempty" validate:"omitempty,max=300"`
}
