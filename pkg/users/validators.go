package users

// CreateUserPayload represents the request body for creating a user.
type CreateUserPayload struct {
	Username         string  `json:"username" validate:"required,min=3,max=50"`
	Email            *string `json:"email" validate:"omitempty,email"`
	Password         string  `json:"password" validate:"required,min=8"`
	RoleID           int     `json:"role_id" validate:"required"`
	LibraryIDs       []int   `json:"library_ids"`        // Empty means no access, special value -1 means all libraries
	AllLibraryAccess bool    `json:"all_library_access"` // If true, user has access to all libraries
}

// UpdateUserPayload represents the request body for updating a user.
type UpdateUserPayload struct {
	Username         *string `json:"username" validate:"omitempty,min=3,max=50"`
	Email            *string `json:"email" validate:"omitempty,email"`
	RoleID           *int    `json:"role_id"`
	IsActive         *bool   `json:"is_active"`
	LibraryIDs       *[]int  `json:"library_ids"`        // If provided, replaces library access
	AllLibraryAccess *bool   `json:"all_library_access"` // If true, grants access to all libraries
}

// ResetPasswordPayload represents the request body for resetting a password.
type ResetPasswordPayload struct {
	CurrentPassword *string `json:"current_password"` // Required if resetting own password
	NewPassword     string  `json:"new_password" validate:"required,min=8"`
}

// ListUsersQuery represents the query parameters for listing users.
type ListUsersQuery struct {
	Limit  int `query:"limit" default:"50"`
	Offset int `query:"offset" default:"0"`
}
