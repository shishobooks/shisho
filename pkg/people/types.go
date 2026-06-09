package people

import "github.com/shishobooks/shisho/pkg/models"

// PersonResponse is the single-person API response. It embeds the Person model
// by value (so tygo emits `extends Person`) and reshapes the aliases relation
// into a flat []string. AuthoredBookCount and NarratedFileCount are derived
// counts; Aliases shadows the embedded model's same-json-tag field, so the wire
// format is byte-identical to the previous anonymous struct.
type PersonResponse struct {
	models.Person     `tstype:",extends"`
	AuthoredBookCount int      `json:"authored_book_count"`
	NarratedFileCount int      `json:"narrated_file_count"`
	Aliases           []string `json:"aliases"`
}

// ListPeopleResponse is the list-endpoint envelope.
type ListPeopleResponse struct {
	Items []PersonResponse `json:"items"`
	Total int              `json:"total"`
}

// ListAuthoredBooksResponse is the authored-books sub-resource envelope.
type ListAuthoredBooksResponse struct {
	Items []*models.Book `json:"items"`
	Total int            `json:"total"`
}

// ListNarratedFilesResponse is the narrated-files sub-resource envelope.
type ListNarratedFilesResponse struct {
	Items []*models.File `json:"items"`
	Total int            `json:"total"`
}

type ListPeopleQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
}

type SubResourceQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type UpdatePersonPayload struct {
	Name     *string  `json:"name,omitempty" validate:"omitempty,max=300"`
	SortName *string  `json:"sort_name,omitempty" validate:"omitempty,max=300"`
	Aliases  []string `json:"aliases,omitempty" validate:"omitempty,dive,min=1,max=300"`
}

type MergePeoplePayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
