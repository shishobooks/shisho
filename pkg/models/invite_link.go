package models

import (
	"time"

	"github.com/uptrace/bun"
)

// InviteLink is a single-use link an admin creates to add one user, with the
// role and Library access that user gets. See "Invite Link" in CONTEXT.md.
type InviteLink struct {
	bun.BaseModel `bun:"table:invite_links,alias:il" tstype:"-"`

	ID               int        `bun:",pk,nullzero" json:"id"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Token            string     `bun:",nullzero" json:"token"`
	CreatedByUserID  int        `bun:",nullzero" json:"created_by_user_id"`
	CreatedByUser    *User      `bun:"rel:belongs-to,join:created_by_user_id=id" json:"-"`
	RoleID           int        `bun:",nullzero" json:"role_id"`
	Role             *Role      `bun:"rel:belongs-to,join:role_id=id" json:"role,omitempty" tstype:"Role"`
	AllLibraryAccess bool       `bun:",notnull" json:"all_library_access"`
	ExpiresAt        time.Time  `json:"expires_at"`
	UsedAt           *time.Time `json:"used_at"`
	UsedByUserID     *int       `json:"used_by_user_id"`
	RevokedAt        *time.Time `json:"revoked_at"`

	Libraries []*InviteLinkLibrary `bun:"rel:has-many,join:id=invite_link_id" json:"-"`
}

// Pending reports whether the invite can still be accepted as far as its own
// row says: not used, not revoked, and not expired at now. Whether its
// creator can still add users is checked separately.
func (l *InviteLink) Pending(now time.Time) bool {
	return l.UsedAt == nil && l.RevokedAt == nil && l.ExpiresAt.After(now)
}

// LibraryIDs returns the Libraries the invite grants when it doesn't grant
// all of them. Libraries must be loaded.
func (l *InviteLink) LibraryIDs() []int {
	ids := make([]int, 0, len(l.Libraries))
	for _, library := range l.Libraries {
		ids = append(ids, library.LibraryID)
	}
	return ids
}

// InviteLinkLibrary is one Library an Invite Link grants access to.
type InviteLinkLibrary struct {
	bun.BaseModel `bun:"table:invite_link_libraries,alias:ill" tstype:"-"`

	InviteLinkID int      `bun:",pk" json:"invite_link_id"`
	LibraryID    int      `bun:",pk" json:"library_id"`
	Library      *Library `bun:"rel:belongs-to,join:library_id=id" json:"-"`
}
