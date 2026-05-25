package publishers

import "encoding/json"

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
