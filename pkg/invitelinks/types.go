package invitelinks

import "github.com/shishobooks/shisho/pkg/models"

// CreateInviteLinkPayload is the request body for POST /invite-links. The
// role and Library access are what the new user gets. Library ids are
// ignored when all_library_access is set; neither means no Library at all.
type CreateInviteLinkPayload struct {
	RoleID           int   `json:"role_id" validate:"required"`
	AllLibraryAccess bool  `json:"all_library_access,omitempty"`
	LibraryIDs       []int `json:"library_ids,omitempty"`
	// ExpiresInDays is how long the link works for. The form offers a few
	// choices; the server takes any whole number of days in range.
	ExpiresInDays int `json:"expires_in_days,omitempty" default:"7" validate:"min=1,max=30"`
}

// InviteLinkResponse is one pending Invite Link as an admin sees it: the
// model with its role, the Libraries it grants, and who created it. The list
// endpoint returns a bare array of these, and create returns one.
type InviteLinkResponse struct {
	models.InviteLink `tstype:",extends"`
	LibraryIDs        []int  `json:"library_ids"`
	CreatedByUsername string `json:"created_by_username"`
}

// InviteResponse is what GET /invites/:token tells whoever opened an invite.
// A known invite that can't be accepted (used, revoked, expired, or its
// creator can no longer add users) is valid false, never saying which, and
// still names who sent it so the page can say whom to ask. An unknown token
// is a 404.
type InviteResponse struct {
	Valid     bool   `json:"valid"`
	InvitedBy string `json:"invited_by"`
}

// AcceptInvitePayload is the request body for POST /invites/:token/accept:
// the account the person chooses for themselves.
type AcceptInvitePayload struct {
	Username string `json:"username" mod:"trim" validate:"required,min=3,max=50"`
	// Email is optional; blank means none.
	Email    string `json:"email,omitempty" mod:"trim" validate:"omitempty,email"`
	Password string `json:"password" validate:"required,min=8"`
}
