package koreaderplugin

// ServerInfo is what every successful plugin response says about the
// server, so the plugin can decide whether to update (ADR 0016).
type ServerInfo struct {
	// ProtocolMin and ProtocolMax are the plugin protocol versions the
	// server accepts. A plugin outside them gets 426 Upgrade Required.
	ProtocolMin int `json:"protocol_min"`
	ProtocolMax int `json:"protocol_max"`
	// PluginVersion and PluginHash name the plugin the server serves, and
	// PluginProtocol is the protocol that plugin speaks.
	PluginVersion  string `json:"plugin_version"`
	PluginHash     string `json:"plugin_hash"`
	PluginProtocol int    `json:"plugin_protocol"`
}

// PairPayload exchanges a Device short code for the Device's sign-in.
type PairPayload struct {
	Code string `json:"code" mod:"trim,lcase" validate:"required,max=32"`
}

// SignInResponse is what the plugin keeps to sign in to a server as a
// KOReader Device.
type SignInResponse struct {
	Username   string     `json:"username"`
	Credential string     `json:"credential"`
	DeviceID   string     `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Server     ServerInfo `json:"server"`
}

// StatusResponse answers a signed-in plugin's check-in.
type StatusResponse struct {
	DeviceID   string     `json:"device_id"`
	DeviceName string     `json:"device_name"`
	Server     ServerInfo `json:"server"`
}

// UploadPayload is one bounded batch of what the plugin has to send: page
// rows from KOReader's statistics and events from its own journal (ADR
// 0016). The upload is idempotent, so a batch sent again after a lost answer
// changes nothing.
type UploadPayload struct {
	// Clock is the device's clock when it sent the batch, in Unix seconds.
	// The server shifts every instant in the batch by how far off it is,
	// when that's more than a few minutes.
	Clock int64 `json:"clock" validate:"required,min=1"`
	// Pages are rows of KOReader's page_stat_data, each one visit to a page.
	Pages []PageRowPayload `json:"pages" validate:"max=1000,dive"`
	// Events are journal entries, each with an id that's never reused.
	Events []EventPayload `json:"events" validate:"max=200,dive"`
}

// PageRowPayload is one row of KOReader's page_stat_data, with the document
// hash of its book. Document, page, and start time are KOReader's own key.
type PageRowPayload struct {
	Document string `json:"document" validate:"required,max=64"`
	// Page is KOReader's 1-based page, out of TotalPages in the layout it was
	// read in.
	Page       int `json:"page" validate:"min=0"`
	TotalPages int `json:"total_pages" validate:"min=0"`
	// StartTime is when the visit began on the device's clock, in Unix
	// seconds, and Duration how long it lasted in seconds.
	StartTime int64 `json:"start_time" validate:"min=0"`
	Duration  int   `json:"duration" validate:"min=0"`
}

// Journal event types.
const (
	//tygo:emit export type PluginEventType = typeof PluginEventPosition;
	PluginEventPosition = "position"
)

// EventPayload is one entry of the plugin's journal.
type EventPayload struct {
	// ID names the event for good, so a repeat is recognized.
	ID   string `json:"id" validate:"required,max=64"`
	Type string `json:"type" validate:"required,oneof=position" tstype:"PluginEventType"`
	// Document is the hash of the book the event is about.
	Document string `json:"document" validate:"required,max=64"`
	// At is when it happened on the device's clock, in Unix seconds.
	At int64 `json:"at" validate:"required,min=1"`
	// Progress and Percentage are a position: an XPointer, or the 1-based
	// page as a string, as KOReader's progress sync sends them.
	Progress   string   `json:"progress,omitempty" validate:"required_if=Type position,max=4096"`
	Percentage *float64 `json:"percentage,omitempty" validate:"required_if=Type position,omitempty,min=0,max=1"`
}

// UploadResponse acknowledges a whole batch: everything in it is taken in,
// including what the server chose to ignore.
type UploadResponse struct {
	Server ServerInfo `json:"server"`
	// ClockOffset is the server's clock minus the device's, in seconds.
	ClockOffset int64 `json:"clock_offset"`
	// Pages is how many page rows the batch held, and Events the ids of its
	// events.
	Pages  int      `json:"pages"`
	Events []string `json:"events"`
}

// PullPayload asks for the positions of the books the plugin names.
type PullPayload struct {
	// Clock is the device's clock, in Unix seconds, so positions come back
	// timed on it.
	Clock     int64    `json:"clock" validate:"required,min=1"`
	Documents []string `json:"documents" validate:"required,min=1,max=100,dive,required,max=64"`
}

// PullResponse holds a position for each named book that has one this
// device can go to. Books with none are left out.
type PullResponse struct {
	Server      ServerInfo       `json:"server"`
	ClockOffset int64            `json:"clock_offset"`
	Positions   []PulledPosition `json:"positions"`
}

// PulledPosition is a book's Reading Position, translated for the copy the
// document hash names.
type PulledPosition struct {
	Document string `json:"document"`
	// Progress is an XPointer, or the 1-based page as a string.
	Progress   string  `json:"progress"`
	Percentage float64 `json:"percentage"`
	// Timestamp is when the reader was there, on the device's clock, in Unix
	// seconds.
	Timestamp int64 `json:"timestamp"`
	// Source names the reader that was there: another Device's name, or the
	// Shisho web reader.
	Source string `json:"source"`
	// Own is a position this Device sent itself.
	Own bool `json:"own"`
}
