package models

import (
	"context"
	"time"

	"github.com/uptrace/bun"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
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

	// UsernameKey is UsernameKey(Username), unique among users, so two
	// usernames that differ only in case can't both exist. It is NULL only
	// for users who already shared a key with another user when keys were
	// added; auth.MatchUsername explains how they sign in.
	UsernameKey string `bun:",nullzero" json:"-"`

	// Getting Started state. pkg/gettingstarted owns it; creating a user sets
	// its starting values. It stays out of user payloads: an admin never sees
	// a user's progress.
	GettingStartedEnabled     bool       `bun:",notnull" json:"-"`
	GettingStartedAutoOpen    bool       `bun:",notnull" json:"-"`
	GettingStartedDismissedAt *time.Time `json:"-"`
	GettingStartedNotForMe    StringList `bun:",notnull" json:"-"`
	// GettingStartedDismissedSteps are the steps that applied when the list
	// was dismissed; one that applies later brings the list back.
	GettingStartedDismissedSteps StringList `bun:",notnull" json:"-"`

	// Relations
	Role          *Role                `bun:"rel:belongs-to,join:role_id=id" json:"role,omitempty" tstype:"Role"`
	LibraryAccess []*UserLibraryAccess `bun:"rel:has-many,join:id=user_id" json:"library_access,omitempty" tstype:"UserLibraryAccess[]"`
}

// UsernameKey returns the form of username that usernames are compared in:
// with every letter's case folded (not just English letters, as SQLite's
// NOCASE does) and in composed form, so "Émile", "ÉMILE", and "émile", or
// "Straße" and "STRASSE", give the same key. It follows Unicode's canonical
// caseless match: decompose, fold, compose.
func UsernameKey(username string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFD.String(username)))
}

var _ bun.BeforeAppendModelHook = (*User)(nil)

// BeforeAppendModel keys every inserted user by its username, so no insert
// path can leave the key out. An update that changes the username lists
// username_key among its columns as well.
func (u *User) BeforeAppendModel(_ context.Context, query bun.Query) error {
	if _, ok := query.(*bun.InsertQuery); ok && u.Username != "" {
		u.UsernameKey = UsernameKey(u.Username)
	}
	return nil
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
