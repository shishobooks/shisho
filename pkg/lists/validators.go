package lists

// Query params for list endpoints.
type ListListsQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"50" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type ListBooksQuery struct {
	Limit  int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=100"`
	Offset int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Sort   *string `query:"sort" json:"sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

// Payloads for create/update endpoints.
type CreateListPayload struct {
	Name        string  `json:"name" validate:"required,min=1,max=200"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   bool    `json:"is_ordered"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

type UpdateListPayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=200" tstype:"string"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   *bool   `json:"is_ordered,omitempty" tstype:"boolean"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"string"`
}

type AddBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type RemoveBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type ReorderBooksPayload struct {
	BookIDs []int `json:"book_ids" validate:"required,min=1,max=500,dive,min=1"`
}

type CreateSharePayload struct {
	UserID     int    `json:"user_id" validate:"required,min=1"`
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager"`
}

type UpdateSharePayload struct {
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager"`
}

type UpdateBookListsPayload struct {
	ListIDs []int `json:"list_ids" validate:"dive,min=1"`
}

type CheckVisibilityQuery struct {
	UserID int `query:"user_id" json:"user_id" validate:"required,min=1" tstype:"number"`
}

type MoveBookPositionPayload struct {
	Position int `json:"position" validate:"required,min=1"`
}

type CreateFromTemplatePayload struct {
	// No fields needed - template name comes from URL
}
