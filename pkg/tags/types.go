package tags

import (
	"github.com/shishobooks/shisho/pkg/bookpreviews"
	"github.com/shishobooks/shisho/pkg/models"
)

// TagResponse is the single-tag API response. It embeds the Tag model by value
// (so tygo emits `extends Tag`) and reshapes the aliases relation into a flat
// []string. BookCount and Aliases shadow the embedded model's same-json-tag
// fields, so the wire format is byte-identical to the previous anonymous struct.
type TagResponse struct {
	models.Tag `tstype:",extends"`
	BookCount  int      `json:"book_count"`
	Aliases    []string `json:"aliases"`
}

// TagListItem is a tag row of the list endpoint: the single-tag
// response plus the covers the list shows for it.
type TagListItem struct {
	TagResponse  `tstype:",extends"`
	PreviewBooks []bookpreviews.PreviewBook `json:"preview_books" tstype:"PreviewBook[]"`
}

// ListTagsResponse is the list-endpoint envelope.
type ListTagsResponse struct {
	Items []TagListItem `json:"items"`
	Total int           `json:"total"`
}

type ListTagsQuery struct {
	Limit     int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=100"`
	Offset    int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search    *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	Sort      string  `query:"sort" json:"sort,omitempty" validate:"omitempty,max=200"`
}

type UpdateTagPayload struct {
	Name    *string  `json:"name,omitempty" validate:"omitempty,min=1,max=300"`
	Aliases []string `json:"aliases,omitempty" validate:"omitempty,dive,min=1,max=300"`
}

type MergeTagsPayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}
