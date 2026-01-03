package series

type ListSeriesQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
}

type UpdateSeriesPayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,max=300"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
}

type MergeSeriesPayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
