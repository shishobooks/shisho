package koreader

import "encoding/json"

// UpdateProgressPayload is KOReader pushing its position in a document.
type UpdateProgressPayload struct {
	// Document is the document hash.
	Document string `json:"document" validate:"required,max=64"`
	// Progress is an XPointer, or the 1-based page as a string for page-based
	// documents.
	Progress   string   `json:"progress" validate:"required,max=4096"`
	Percentage *float64 `json:"percentage" validate:"required,min=0,max=1" tstype:",required"`
	// Device and DeviceID are how KOReader names itself. Shisho knows the
	// Device from its credential; it stores these with the position and sends
	// them back to the same Device, which then knows the position as its own.
	Device   string `json:"device,omitempty" validate:"max=256"`
	DeviceID string `json:"device_id,omitempty" validate:"max=256"`
	// Metadata is what KOReader knows about the book, when its "send
	// metadata" option is on. It's only logged, to name a book Shisho doesn't
	// recognize.
	Metadata json.RawMessage `json:"metadata,omitempty" tstype:"unknown"`
	// Timestamp is when KOReader was at the position, in Unix seconds. No
	// KOReader release sends it yet; one that does dates its offline pushes.
	Timestamp *int64 `json:"timestamp,omitempty" validate:"omitempty,min=1"`
}

// UpdateProgressResponse answers a push. Timestamp is in Unix seconds.
type UpdateProgressResponse struct {
	Document  string `json:"document"`
	Timestamp int64  `json:"timestamp"`
}

// ProgressResponse answers a pull. With no position to give, every field is
// absent: an empty object is KOReader's "no progress".
type ProgressResponse struct {
	Document   string   `json:"document,omitempty"`
	Progress   string   `json:"progress,omitempty"`
	Percentage *float64 `json:"percentage,omitempty"`
	Device     string   `json:"device,omitempty"`
	DeviceID   string   `json:"device_id,omitempty"`
	// Timestamp is when the position was reported, in Unix seconds. KOReader
	// compares it to its own clock.
	Timestamp *int64 `json:"timestamp,omitempty"`
}

// AuthorizeResponse answers a successful login.
type AuthorizeResponse struct {
	Authorized string `json:"authorized"`
}

// ProtocolErrorResponse is an error in the protocol's shape, not Shisho's
// usual one. KOReader shows Message to the user as it stands.
type ProtocolErrorResponse struct {
	Message string `json:"message"`
}
