package genres

import "github.com/shishobooks/shisho/pkg/models"

// GenreResponse is the single-genre API response. It embeds the Genre model by
// value (so tygo emits `extends Genre`) and reshapes the aliases relation into a
// flat []string. BookCount and Aliases shadow the embedded model's same-json-tag
// fields, so the wire format is byte-identical to the previous anonymous struct.
type GenreResponse struct {
	models.Genre `tstype:",extends"`
	BookCount    int      `json:"book_count"`
	Aliases      []string `json:"aliases"`
}

// ListGenresResponse is the list-endpoint envelope.
type ListGenresResponse struct {
	Items []GenreResponse `json:"items"`
	Total int             `json:"total"`
}

type ListGenresQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
}

type SubResourceQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type UpdateGenrePayload struct {
	Name    *string  `json:"name,omitempty" validate:"omitempty,min=1,max=300"`
	Aliases []string `json:"aliases,omitempty" validate:"omitempty,dive,min=1,max=300"`
}

type MergeGenresPayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
