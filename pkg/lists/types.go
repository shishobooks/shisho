package lists

import "github.com/shishobooks/shisho/pkg/models"

// PermissionOwner is the synthetic permission the list/retrieve handlers return
// for a list's owner. The persisted share grants are the ListPermission values
// (viewer/editor/manager); "owner" is computed per request, never stored.
const (
	//tygo:emit export type ListResponsePermission = "owner" | "manager" | "editor" | "viewer";
	PermissionOwner = "owner"
)

// ListResponse is a single list augmented with the requesting user's effective
// permission and the list's book count. It embeds the List model by value so
// tygo emits `extends List` and the wire format stays byte-identical.
type ListResponse struct {
	models.List `tstype:",extends"`
	BookCount   int    `json:"book_count"`
	Permission  string `json:"permission" tstype:"ListResponsePermission"`
}

// ListListsResponse is the list-endpoint envelope.
type ListListsResponse struct {
	Items []ListResponse `json:"items"`
	Total int            `json:"total"`
}

// RetrieveListResponse is the single-list API response. It mirrors ListResponse:
// the List model fields are flattened in, plus book_count and permission.
type RetrieveListResponse struct {
	models.List `tstype:",extends"`
	BookCount   int    `json:"book_count"`
	Permission  string `json:"permission" tstype:"ListResponsePermission"`
}

// ListListBooksResponse is the books-in-list envelope. Items are ListBook rows
// (each carrying its Book) so the frontend can read cover cache keys.
type ListListBooksResponse struct {
	Items []*models.ListBook `json:"items" tstype:"ListBook[]"`
	Total int                `json:"total"`
}

// CheckVisibilityResponse reports how many of a list's books a target user can
// see given their library access.
type CheckVisibilityResponse struct {
	Visible int `json:"visible"`
	Total   int `json:"total"`
}

// ListTemplate is a built-in list template offered by the templates endpoint.
type ListTemplate struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	IsOrdered   bool   `json:"is_ordered"`
	DefaultSort string `json:"default_sort" tstype:"ListSort"`
}

// Query params for list endpoints.
type ListListsQuery struct {
	Limit  int `query:"limit" json:"limit,omitempty" default:"50" validate:"min=1,max=100"`
	Offset int `query:"offset" json:"offset,omitempty" validate:"min=0"`
}

type ListBooksQuery struct {
	Limit  int     `query:"limit" json:"limit,omitempty" default:"24" validate:"min=1,max=100"`
	Offset int     `query:"offset" json:"offset,omitempty" validate:"min=0"`
	Sort   *string `query:"sort" json:"sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"ListSort"`
}

// Payloads for create/update endpoints.
type CreateListPayload struct {
	Name        string  `json:"name" validate:"required,min=1,max=200"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   bool    `json:"is_ordered"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"ListSort"`
}

type UpdateListPayload struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=200" tstype:"string"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000" tstype:"string"`
	IsOrdered   *bool   `json:"is_ordered,omitempty" tstype:"boolean"`
	DefaultSort *string `json:"default_sort,omitempty" validate:"omitempty,oneof=manual added_at_desc added_at_asc title_asc title_desc author_asc author_desc" tstype:"ListSort"`
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
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager" tstype:"ListPermission"`
}

type UpdateSharePayload struct {
	Permission string `json:"permission" validate:"required,oneof=viewer editor manager" tstype:"ListPermission"`
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
