package sharelinks

import (
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// SharingSettingsResponse is the response for GET/PUT /settings/sharing.
//
// Field shape must stay identical to Settings (same field names, same types,
// same order) because the handler converts between them with
// `SharingSettingsResponse(settings)`.
type SharingSettingsResponse struct {
	Enabled           bool `json:"enabled"`
	RequireExpiration bool `json:"require_expiration"`
}

// UpdateSharingSettingsPayload is the request body for PUT /settings/sharing.
//
// Fields are pointers so each switch can save on its own: an omitted field
// keeps its saved value.
type UpdateSharingSettingsPayload struct {
	Enabled           *bool `json:"enabled,omitempty"`
	RequireExpiration *bool `json:"require_expiration,omitempty"`
}

// ShareLinkResponse is one Share Link as the sharer sees it: the model plus
// its derived state, why an active link is paused (omitted when it is not),
// and the creator's username. The list endpoint returns a bare array of
// these, and create and revoke return one.
type ShareLinkResponse struct {
	models.ShareLink  `tstype:",extends"`
	State             string `json:"state" tstype:"ShareLinkState"`
	PausedReason      string `json:"paused_reason,omitempty" tstype:"ShareLinkPausedReason"`
	CreatedByUsername string `json:"created_by_username"`
}

// CreateShareLinkPayload is the request body for POST /books/:id/share-links.
// The client turns an expiration preset into an absolute timestamp; the
// server knows nothing about presets. An omitted expires_at means the link
// never expires, which the sharing settings can forbid.
type CreateShareLinkPayload struct {
	Label     *string    `json:"label,omitempty" mod:"trim" validate:"omitempty,max=200" tstype:"string"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" tstype:"string"`
}

// SharedBookResponse is the book a recipient sees through a Share Link. It is
// the Book shape the detail page already renders, with every filesystem path
// and cover filename blanked and the library removed, plus who shared it,
// when the link expires, the library's cover aspect ratio so the cover box
// keeps its shape, and which file the book cover comes from (the page cannot
// work that out without the blanked cover filenames).
type SharedBookResponse struct {
	models.Book      `tstype:",extends"`
	SharedBy         string     `json:"shared_by"`
	ExpiresAt        *time.Time `json:"expires_at"`
	CoverAspectRatio string     `json:"cover_aspect_ratio" tstype:"CoverAspectRatio"`
	CoverFileID      *int       `json:"cover_file_id"` // nil when the book has no cover
}
