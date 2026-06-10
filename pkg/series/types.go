package series

import "github.com/shishobooks/shisho/pkg/models"

// SeriesResponse is the single-series API response. It embeds the Series model
// by value (so tygo emits `extends Series`) and reshapes the aliases relation
// into a flat []string. BookCount and Aliases shadow the embedded model's
// same-json-tag fields, so the wire format is byte-identical to the previous
// anonymous struct. The name follows the project-wide {Entity}Response
// convention (ADR 0004); the revive "stutter" warning is a false positive
// because the entity is itself named "Series".
//
//nolint:revive // SeriesResponse name is mandated by the {Entity}Response convention (ADR 0004)
type SeriesResponse struct {
	models.Series `tstype:",extends"`
	BookCount     int      `json:"book_count"`
	Aliases       []string `json:"aliases"`
}

// ListSeriesResponse is the list-endpoint envelope.
//
//nolint:revive // ListSeriesResponse name is mandated by the List{Entities}Response convention (ADR 0004)
type ListSeriesResponse struct {
	Items []SeriesResponse `json:"items"`
	Total int              `json:"total"`
}

// ListSeriesBooksResponse is the envelope for the series books sub-resource.
//
//nolint:revive // ListSeriesBooksResponse name is mandated by the List{Entities}Response convention (ADR 0004)
type ListSeriesBooksResponse struct {
	Items []*models.Book `json:"items" tstype:"Book[]"`
	Total int            `json:"total"`
}

type ListSeriesQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
}

type SubResourceQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type UpdateSeriesPayload struct {
	Name        *string  `json:"name,omitempty" validate:"omitempty,max=300"`
	SortName    *string  `json:"sort_name,omitempty" validate:"omitempty,max=300"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=2000"`
	Aliases     []string `json:"aliases,omitempty" validate:"omitempty,dive,min=1,max=300"`
}

type MergeSeriesPayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
