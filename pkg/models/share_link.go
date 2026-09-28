package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Share Link states. The state is derived from revoked_at and expires_at on
// every read and never stored.
const (
	//tygo:emit export type ShareLinkState = typeof ShareLinkStateActive | typeof ShareLinkStateExpired | typeof ShareLinkStateRevoked;
	ShareLinkStateActive  = "active"
	ShareLinkStateExpired = "expired"
	ShareLinkStateRevoked = "revoked"
)

// Reasons an active Share Link is paused: it does not resolve because of its
// creator, and works again if the creator's access returns. Like the state,
// the reason is derived on every read and never stored.
const (
	//tygo:emit export type ShareLinkPausedReason = typeof ShareLinkPausedCreatorDeactivated | typeof ShareLinkPausedCreatorNoLibraryAccess;
	ShareLinkPausedCreatorDeactivated     = "creator_deactivated"
	ShareLinkPausedCreatorNoLibraryAccess = "creator_no_library_access"
)

// ShareLink is an anonymous, optionally expiring link that grants access to
// one Book. See "Share Link" in CONTEXT.md.
type ShareLink struct {
	bun.BaseModel `bun:"table:share_links,alias:sl" tstype:"-"`

	ID              int        `bun:",pk,nullzero" json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Token           string     `bun:",nullzero" json:"token"`
	BookID          int        `bun:",nullzero" json:"book_id"`
	CreatedByUserID int        `bun:",nullzero" json:"created_by_user_id"`
	CreatedByUser   *User      `bun:"rel:belongs-to,join:created_by_user_id=id" json:"-"`
	Label           *string    `json:"label"`
	ExpiresAt       *time.Time `json:"expires_at"`
	RevokedAt       *time.Time `json:"revoked_at"`
	OpenCount       int        `json:"open_count"`
	DownloadCount   int        `json:"download_count"`
	LastAccessedAt  *time.Time `json:"last_accessed_at"`
}

// State derives the link's state at now: revoked if it was revoked,
// otherwise expired once expires_at is at or before now, otherwise active.
func (l *ShareLink) State(now time.Time) string {
	if l.RevokedAt != nil {
		return ShareLinkStateRevoked
	}
	if l.ExpiresAt != nil && !l.ExpiresAt.After(now) {
		return ShareLinkStateExpired
	}
	return ShareLinkStateActive
}

// PausedReason reports why the link's creator stops it from resolving for a
// book in libraryID, or "" when the creator can still share it. It needs
// CreatedByUser loaded with its LibraryAccess; a missing creator counts as
// deactivated so a caller that forgot to load it fails closed.
func (l *ShareLink) PausedReason(libraryID int) string {
	creator := l.CreatedByUser
	if creator == nil || !creator.IsActive {
		return ShareLinkPausedCreatorDeactivated
	}
	if !creator.HasLibraryAccess(libraryID) {
		return ShareLinkPausedCreatorNoLibraryAccess
	}
	return ""
}
