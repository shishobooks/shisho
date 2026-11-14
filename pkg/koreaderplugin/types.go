package koreaderplugin

import "time"

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
	// Unrecognized describes copies the batch has data for that no server
	// the plugin signed in to recognizes or has linked (ADR 0017). The
	// server holds their data and lists them on the Device's page. It can
	// also describe a copy the server already holds with nothing else of it,
	// as lookup asks (Undescribed).
	// It can describe every book in the batch's pages and events.
	Unrecognized []UnrecognizedPayload `json:"unrecognized,omitempty" validate:"max=1200,dive"`
	// Claimed names copies another server the plugin signed in to now
	// recognizes. This server drops what it held for them. The plugin sends
	// them in batches of at most this many.
	Claimed []string `json:"claimed,omitempty" validate:"max=500,dive,required,max=64"`
	// Backfill marks a batch of the history KOReader recorded before the
	// plugin signed in, sent when the user chose to (ADR 0016). Its times
	// may predate the Device. Its pages start no Read-through, and its
	// status and Rating events only fill in what Shisho has none of: a
	// status makes a Read-through for a Book with none, and a Rating sets
	// one for a Book with none.
	Backfill bool `json:"backfill,omitempty"`
}

// UnrecognizedPayload is what KOReader knows about a copy no server the
// plugin signed in to recognizes: from its statistics and its sidecar.
type UnrecognizedPayload struct {
	Document    string   `json:"document" validate:"required,max=64"`
	Title       string   `json:"title,omitempty" validate:"max=1024"`
	Authors     []string `json:"authors,omitempty" validate:"max=50,dive,max=512"`
	Identifiers []string `json:"identifiers,omitempty" validate:"max=50,dive,max=512"`
	Language    string   `json:"language,omitempty" validate:"max=64"`
	// FileType is the copy's file extension, such as epub or kepub.
	FileType string `json:"file_type,omitempty" validate:"max=16"`
	// ReadingSeconds is KOReader's total reading time in the copy, and
	// FirstRead and LastRead when it was first and last read, on the
	// device's clock, in Unix seconds.
	ReadingSeconds int   `json:"reading_seconds,omitempty" validate:"min=0"`
	FirstRead      int64 `json:"first_read,omitempty" validate:"min=0"`
	LastRead       int64 `json:"last_read,omitempty" validate:"min=0"`
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

// Journal event types: a position, or a change to a book's status or Rating
// on the device.
const (
	//tygo:emit export type PluginEventType = typeof PluginEventPosition | typeof PluginEventStatus | typeof PluginEventRating;
	PluginEventPosition = "position"
	PluginEventStatus   = "status"
	PluginEventRating   = "rating"
)

// KOReader's statuses, as a book's sidecar holds them. KOReader shows
// abandoned as On hold and complete as Finished.
const (
	//tygo:emit export type KOReaderStatus = typeof KOReaderStatusReading | typeof KOReaderStatusOnHold | typeof KOReaderStatusFinished;
	KOReaderStatusReading  = "reading"
	KOReaderStatusOnHold   = "abandoned"
	KOReaderStatusFinished = "complete"
)

// EventPayload is one entry of the plugin's journal.
type EventPayload struct {
	// ID names the event for good, so a repeat is recognized.
	ID   string `json:"id" validate:"required,max=64"`
	Type string `json:"type" validate:"required,oneof=position status rating" tstype:"PluginEventType"`
	// Document is the hash of the book the event is about.
	Document string `json:"document" validate:"required,max=64"`
	// At is when it happened on the device's clock, in Unix seconds. A status
	// or Rating change the plugin didn't see happen has none, only a Day.
	At int64 `json:"at,omitempty" validate:"required_without=Day,omitempty,min=1"`
	// Day is the day KOReader recorded a status or Rating change, as
	// YYYY-MM-DD on the device's calendar.
	Day string `json:"day,omitempty" validate:"omitempty,datetime=2006-01-02"`
	// Progress and Percentage are a position: an XPointer, or the 1-based
	// page as a string, as KOReader's progress sync sends them.
	Progress   string   `json:"progress,omitempty" validate:"required_if=Type position,max=4096"`
	Percentage *float64 `json:"percentage,omitempty" validate:"required_if=Type position,omitempty,min=0,max=1"`
	// Status is a status event's new status, and PreviousStatus the one the
	// plugin last synced with this server, empty when it never did.
	Status         string `json:"status,omitempty" validate:"required_if=Type status,omitempty,oneof=reading abandoned complete" tstype:"KOReaderStatus"`
	PreviousStatus string `json:"previous_status,omitempty" validate:"omitempty,oneof=reading abandoned complete" tstype:"KOReaderStatus"`
	// Rating is a rating event's new whole stars, 0 for none, and
	// PreviousRating the stars the plugin last synced with this server.
	Rating         int `json:"rating,omitempty" validate:"min=0,max=5"`
	PreviousRating int `json:"previous_rating,omitempty" validate:"min=0,max=5"`
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

// LookupPayload asks which of the copies the plugin holds data for the
// server recognizes, before the plugin reports any as unrecognized (ADR
// 0017).
type LookupPayload struct {
	Documents []string `json:"documents" validate:"max=500,dive,required,max=64"`
}

// What the server knows of a copy.
const (
	//tygo:emit export type DocumentState = typeof DocumentRecognized | typeof DocumentHeld | typeof DocumentDismissed | typeof DocumentUnknown;
	// DocumentRecognized is a copy Shisho produced or a user linked, of a
	// File the Device reaches.
	DocumentRecognized = "recognized"
	// DocumentHeld is a copy the server holds data for as unrecognized.
	DocumentHeld = "held"
	// DocumentDismissed is an unrecognized copy the user dismissed, whose
	// data the server drops.
	DocumentDismissed = "dismissed"
	// DocumentUnknown is a copy the server knows nothing of.
	DocumentUnknown = "unknown"
)

// LookupResponse says what the server knows of each copy asked about.
type LookupResponse struct {
	Server    ServerInfo         `json:"server"`
	Documents []DocumentLookedUp `json:"documents"`
	// Undescribed names copies the server holds for this Device with
	// nothing KOReader knows of them, no title or file type, asked about or
	// not. The plugin sends what it knows of each at its next upload
	// (Unrecognized), so the Device's page can say what they are.
	Undescribed []string `json:"undescribed"`
}

// DocumentLookedUp is what the server knows of one copy.
type DocumentLookedUp struct {
	Document string `json:"document"`
	State    string `json:"state" tstype:"DocumentState"`
}

// PullPayload asks for the positions of the books the plugin names, and for
// the statuses and Ratings it should write to the device.
type PullPayload struct {
	// Clock is the device's clock, in Unix seconds, so positions come back
	// timed on it.
	Clock int64 `json:"clock" validate:"required,min=1"`
	// Documents names the books whose positions the plugin wants.
	Documents []string `json:"documents" validate:"max=100,dive,required,max=64"`
	// Statuses names books whose status and Rating the plugin wants as they
	// are, such as ones it hasn't asked this server about before.
	Statuses []string `json:"statuses,omitempty" validate:"max=500,dive,required,max=64"`
	// Since is the Cursor of the plugin's last pull. The status and Rating
	// of every book that changed after it come back as well.
	Since int64 `json:"since,omitempty" validate:"min=0"`
}

// PullResponse holds a position for each named book that has one this
// device can go to, and the states the device should hold. Books with none
// are left out.
type PullResponse struct {
	Server      ServerInfo       `json:"server"`
	ClockOffset int64            `json:"clock_offset"`
	Positions   []PulledPosition `json:"positions"`
	// States holds the status and Rating of each book Statuses named that
	// this device reaches, and of each book whose status or Rating changed
	// after Since.
	States []PulledState `json:"states"`
	// Cursor is what the next pull sends as Since.
	Cursor int64 `json:"cursor"`
}

// PulledState is what KOReader should show for a book's status and Rating
// (ADR 0016).
type PulledState struct {
	Document string `json:"document"`
	// Status is the book's Reading Status as KOReader has it: Read is
	// complete, Paused and Did Not Finish are abandoned, and Currently
	// Reading is reading. To Read and no status have none, which leaves the
	// device's alone.
	Status string `json:"status,omitempty" tstype:"KOReaderStatus"`
	// Rating is the Rating in whole stars, rounded with halves up, and at
	// least one. It's 0 for a Rating removed after Since, and absent for
	// none, which leaves the device's alone.
	Rating *int `json:"rating,omitempty"`
}

// PulledPosition is a book's Reading Position, translated for the copy the
// document hash names. A position with no Progress is a percentage only, for
// a copy a user linked (ADR 0017) or a position kept as a percentage, and
// KOReader goes to it by percentage.
type PulledPosition struct {
	Document string `json:"document"`
	// Progress is an XPointer, or the 1-based page as a string.
	Progress   string  `json:"progress,omitempty"`
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

// UnrecognizedBook is a copy on a KOReader Device that no server the plugin
// signed in to recognized (ADR 0017), as its Device's page lists it: the
// ones still to link first, then the linked ones.
type UnrecognizedBook struct {
	Document    string   `json:"document"`
	Title       *string  `json:"title" tstype:"string | null"`
	Authors     []string `json:"authors"`
	Identifiers []string `json:"identifiers"`
	Language    *string  `json:"language" tstype:"string | null"`
	FileType    *string  `json:"file_type" tstype:"string | null"`
	// ReadingSeconds is KOReader's reading time in the copy, and FirstReadAt
	// and LastReadAt when it was first and last read.
	ReadingSeconds int        `json:"reading_seconds"`
	FirstReadAt    *time.Time `json:"first_read_at" tstype:"string | null"`
	LastReadAt     *time.Time `json:"last_read_at" tstype:"string | null"`
	// LinkedAt is when the user linked the copy to a File, and Linked that
	// File, nil once it's gone. A copy not linked yet has Suggestions.
	LinkedAt    *time.Time      `json:"linked_at" tstype:"string | null"`
	Linked      *LinkCandidate  `json:"linked" tstype:"LinkCandidate | null"`
	Suggestions []LinkCandidate `json:"suggestions"`
}

// How a candidate File for a link was found.
const (
	//tygo:emit export type LinkMatch = typeof LinkMatchIdentifier | typeof LinkMatchSearch;
	LinkMatchIdentifier = "identifier"
	LinkMatchSearch     = "search"
)

// LinkCandidate is a File an unrecognized copy could be linked to.
type LinkCandidate struct {
	FileID      int      `json:"file_id"`
	BookID      int      `json:"book_id"`
	LibraryID   int      `json:"library_id"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	FileType    string   `json:"file_type"`
	LibraryName string   `json:"library_name"`
	// Match is how it was found, and empty for the File a copy is linked to.
	Match string `json:"match,omitempty" tstype:"LinkMatch"`
}

// LinkCandidatesQuery searches the Files a copy could be linked to.
type LinkCandidatesQuery struct {
	Q string `query:"q" json:"q" mod:"trim" validate:"required,max=256"`
}

// DismissAllPayload dismisses the copies still to link that the Device's
// page listed. One that arrived since isn't dismissed unseen.
type DismissAllPayload struct {
	Documents []string `json:"documents" validate:"required,min=1,max=10000,dive,required,max=64"`
}

// LinkPayload links an unrecognized copy to a File.
type LinkPayload struct {
	FileID int `json:"file_id" validate:"required,min=1"`
}
