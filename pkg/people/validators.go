package people

type ListPeopleQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"25" validate:"min=1,max=100"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100"`
}

type UpdatePersonPayload struct {
	Name     *string `json:"name,omitempty" validate:"omitempty,max=300"`
	SortName *string `json:"sort_name,omitempty" validate:"omitempty,max=300"`
}

type MergePeoplePayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
