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

// StatusResponse represents the auth status response: the server-wide state
// the app needs before and after sign-in.
type StatusResponse struct {
	DemoMode   bool `json:"demo_mode"`
	NeedsSetup bool `json:"needs_setup"`
	// PDFRenderKey identifies the PDF render settings (see
	// pdfpages.RenderKey). PDF page URLs carry it so a browser does not keep
	// showing pages rendered at old settings after a restart.
	PDFRenderKey string `json:"pdf_render_key"`
}

// MeResponse represents the current user response.
type MeResponse struct {
	ID                 int      `json:"id"`
	Username           string   `json:"username"`
	Email              *string  `json:"email,omitempty"`
	RoleID             int      `json:"role_id"`
	RoleName           string   `json:"role_name"`
	Permissions        []string `json:"permissions"`
	LibraryAccess      *[]int   `json:"library_access"` // nil = all libraries, empty = none, populated = specific libraries
	MustChangePassword bool     `json:"must_change_password"`
}
