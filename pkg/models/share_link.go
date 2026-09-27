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
