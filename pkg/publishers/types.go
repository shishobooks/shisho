package publishers

import (
	"encoding/json"

	"github.com/shishobooks/shisho/pkg/models"
)

// AncestorResponse is a lightweight ancestor node in a publisher's hierarchy
// chain, ordered from immediate parent to root in PublisherResponse.Ancestors.
type AncestorResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ChildResponse is a flattened direct child of a publisher, carrying its own
// direct file count. It replaces the model's Children []*Publisher relation in
// API responses.
type ChildResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
}

// PublisherResponse is the full single-publisher API response used by retrieve,
// update, and merge. It embeds the Publisher model by value (so tygo emits
// `extends Publisher`) and reshapes the hierarchy: aliases as a flat []string,
// ancestors and flattened children, and descendant ids. FileCount and Aliases
// shadow the embedded model's same-json-tag fields, keeping the wire format
// byte-identical to the previous anonymous struct.
type PublisherResponse struct {
	models.Publisher    `tstype:",extends"`
	FileCount           int                `json:"file_count"`
	DescendantFileCount int                `json:"descendant_file_count"`
	Aliases             []string           `json:"aliases"`
	Ancestors           []AncestorResponse `json:"ancestors"`
	DescendantIDs       []int              `json:"descendant_ids"`
	Children            []ChildResponse    `json:"children"`
}

// PublisherListItem is the light list-row shape returned by GET /publishers. It
// deliberately omits the full hierarchy (ancestors, flattened children,
// descendant ids) — computing those per row would be an N+1 — and instead
// carries the parent's name and rolled-up counts. It embeds the Publisher model
// by value (so tygo emits `extends Publisher`) and reshapes aliases to []string.
type PublisherListItem struct {
	models.Publisher         `tstype:",extends"`
	FileCount                int      `json:"file_count"`
	DescendantFileCount      int      `json:"descendant_file_count"`
	DescendantPublisherCount int      `json:"descendant_publisher_count"`
	ParentName               *string  `json:"parent_name"`
	Aliases                  []string `json:"aliases"`
}

// ListPublishersResponse is the list-endpoint envelope.
type ListPublishersResponse struct {
	Items []PublisherListItem `json:"items"`
	Total int                 `json:"total"`
}

// ListPublisherFilesResponse is the envelope for the files sub-resource.
type ListPublisherFilesResponse struct {
	Items []*models.File `json:"items"`
	Total int            `json:"total"`
}

type ListPublishersQuery struct {
	Limit      int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset     int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	LibraryID  *int    `query:"library_id" json:"library_id,omitempty" validate:"omitempty,min=1" tstype:"number"`
	Search     *string `query:"search" json:"search,omitempty" validate:"omitempty,max=100" tstype:"string"`
	ExcludeIDs []int   `query:"exclude_ids" json:"exclude_ids,omitempty" validate:"omitempty,dive,min=1" tstype:"number[]"`
}

type SubResourceQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=50"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

// NullableInt distinguishes between "field absent" (Set=false), "field present
// as null" (Set=true, Value=nil), and "field present with value" (Set=true,
// Value=&n). Standard encoding/json cannot differentiate absent from null on
// pointer types, so we use a custom UnmarshalJSON.
type NullableInt struct {
	Value *int
	Set   bool
}

func (n *NullableInt) UnmarshalJSON(data []byte) error {
	n.Set = true
	if string(data) == "null" {
		n.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	n.Value = &v
	return nil
}

// UpdatePublisherPayload is the request body for PATCH /publishers/:id.
// tstype is applied to the generated TypeScript type.
type UpdatePublisherPayload struct {
	Name       *string     `json:"name,omitempty" validate:"omitempty,min=1,max=300"`
	Aliases    []string    `json:"aliases,omitempty" validate:"omitempty,dive,min=1,max=300"`
	ParentID   NullableInt `json:"parent_id,omitempty" tstype:"number | null"`
	ParentName *string     `json:"parent_name,omitempty" validate:"omitempty,min=1,max=300"`
}

type MergePublishersPayload struct {
	SourceID int `json:"source_id" validate:"required,min=1"`
}

type SetChildPayload struct {
	ChildID int `json:"child_id" validate:"required,min=1"`
}
