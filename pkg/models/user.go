package models

import (
	"time"

	"github.com/uptrace/bun"
)

type User struct {
	bun.BaseModel `bun:"table:users,alias:u" tstype:"-"`

	ID                 int       `bun:",pk,nullzero" json:"id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Username           string    `bun:",nullzero" json:"username"`
	Email              *string   `json:"email,omitempty"`
	PasswordHash       string    `json:"-"` // Never expose password hash
	RoleID             int       `json:"role_id"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`

	// Relations
	Role          *Role                `bun:"rel:belongs-to,join:role_id=id" json:"role,omitempty" tstype:"Role"`
	LibraryAccess []*UserLibraryAccess `bun:"rel:has-many,join:id=user_id" json:"library_access,omitempty" tstype:"UserLibraryAccess[]"`
}

// HasPermission checks if the user has a specific permission.
func (u *User) HasPermission(resource, operation string) bool {
	if u.Role == nil {
		return false
	}
	return u.Role.HasPermission(resource, operation)
}

// HasLibraryAccess checks if the user can access a specific library.
// Returns true if user has access to all libraries (null library_id entry)
// or has explicit access to the specified library.
func (u *User) HasLibraryAccess(libraryID int) bool {
	for _, access := range u.LibraryAccess {
		// null library_id means access to all libraries
		if access.LibraryID == nil {
			return true
		}
		if *access.LibraryID == libraryID {
			return true
		}
	}
	return false
}

// HasAllLibraryAccess checks if the user has access to all libraries.
func (u *User) HasAllLibraryAccess() bool {
	for _, access := range u.LibraryAccess {
		if access.LibraryID == nil {
			return true
		}
	}
	return false
}

// GetAccessibleLibraryIDs returns the list of library IDs the user can access.
// Returns nil if user has access to all libraries.
func (u *User) GetAccessibleLibraryIDs() []int {
	if u.HasAllLibraryAccess() {
		return nil
	}
	ids := make([]int, 0, len(u.LibraryAccess))
	for _, access := range u.LibraryAccess {
		if access.LibraryID != nil {
			ids = append(ids, *access.LibraryID)
		}
	}
	return ids
}

// UserRef is a read-only view of a user carrying only the id and username.
// Payloads that any signed-in user can read, such as a list's owner, its
// shares, and who added each book, embed it instead of User so they never
// carry an email address, role, or account state.
type UserRef struct {
	bun.BaseModel `bun:"table:users,alias:u" tstype:"-"`

	ID       int    `bun:",pk,nullzero" json:"id"`
	Username string `json:"username"`
}

type UserLibraryAccess struct {
	bun.BaseModel `bun:"table:user_library_access,alias:ula" tstype:"-"`

	ID        int  `bun:",pk,nullzero" json:"id"`
	UserID    int  `json:"user_id"`
	LibraryID *int `json:"library_id"` // null means access to all libraries
}
