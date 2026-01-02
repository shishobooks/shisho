package auth

// LoginPayload represents the login request body.
type LoginPayload struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8"`
}

// SetupPayload represents the initial setup request body.
type SetupPayload struct {
	Username string  `json:"username" validate:"required,min=3,max=50"`
	Email    *string `json:"email" validate:"omitempty,email"`
	Password string  `json:"password" validate:"required,min=8"`
}

// StatusResponse represents the auth status response.
type StatusResponse struct {
	NeedsSetup bool `json:"needs_setup"`
}

// MeResponse represents the current user response.
type MeResponse struct {
	ID            int      `json:"id"`
	Username      string   `json:"username"`
	Email         *string  `json:"email,omitempty"`
	RoleID        int      `json:"role_id"`
	RoleName      string   `json:"role_name"`
	Permissions   []string `json:"permissions"`
	LibraryAccess *[]int   `json:"library_access"` // nil = all libraries, empty = none, populated = specific libraries
}
