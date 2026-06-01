package roles

// PermissionInput represents a permission to grant to a role.
type PermissionInput struct {
	Resource  string `json:"resource" validate:"required"`
	Operation string `json:"operation" validate:"required"`
}

// CreateRolePayload represents the request body for creating a role.
type CreateRolePayload struct {
	Name        string            `json:"name" validate:"required,min=1,max=50"`
	Permissions []PermissionInput `json:"permissions"`
}

// UpdateRolePayload represents the request body for updating a role.
type UpdateRolePayload struct {
	Name        *string           `json:"name" validate:"omitempty,min=1,max=50"`
	Permissions []PermissionInput `json:"permissions"` // Replaces all permissions if provided
}

// ListRolesQuery represents the query parameters for listing roles.
type ListRolesQuery struct {
	Limit  int `query:"limit" default:"50"`
	Offset int `query:"offset" default:"0"`
}
