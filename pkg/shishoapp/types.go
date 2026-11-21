package shishoapp

// The Shisho app's Device surface (ADR 0020). These types are a contract
// with app releases that can't be forced to update: within a protocol,
// responses only grow, and request fields stay strict. Removing a response
// field or changing its type means a new protocol. The app decodes them
// leniently, ignoring fields it doesn't know and mapping unknown enum values
// to an unknown case (ADR 0021).

import (
	"encoding/json"
	"time"
)

// ServerInfo is what the server accepts and offers, for any app protocol.
type ServerInfo struct {
	// ProtocolMin and ProtocolMax are the app protocols the server accepts.
	// An app outside them gets 426 Upgrade Required.
	ProtocolMin int `json:"protocol_min"`
	ProtocolMax int `json:"protocol_max"`
	// Capabilities lists the features the server offers to the protocol the
	// request named, or none for a protocol it doesn't accept.
	Capabilities []string `json:"capabilities"`
	// DemoMode is a server that refuses every write, including signing in,
	// so the app can say it doesn't connect to the demo.
	DemoMode bool `json:"demo_mode"`
	// ThumbnailRenderKey goes in cover URLs as r, so covers resized by an
	// older renderer aren't kept once the renderer changes.
	ThumbnailRenderKey string `json:"thumbnail_render_key"`
}

// SignInPayload signs an installation in with the user's username and
// password, which creates a Shisho app Device for it.
type SignInPayload struct {
	Username string `json:"username" mod:"trim" validate:"required,max=256"`
	Password string `json:"password" validate:"required,max=1024"`
	// InstallationID is the installation's own id, made when the app was
	// installed and kept across signing out and back in.
	InstallationID string `json:"installation_id" mod:"trim" validate:"required,max=64"`
	// DeviceName names the hardware, such as "iPhone", for the Device list.
	DeviceName string `json:"device_name" mod:"trim" validate:"required,max=100"`
	AppVersion string `json:"app_version" mod:"trim" validate:"required,max=64"`
}

// SignInResponse is what the installation keeps for the account: the
// credential it sends as "Authorization: Bearer <credential>" on every
// other request. The credential never expires; revoking the Device or
// deactivating the user ends it.
type SignInResponse struct {
	Credential    string     `json:"credential"`
	DeviceID      string     `json:"device_id"`
	DeviceName    string     `json:"device_name"`
	User          AppUser    `json:"user"`
	ServerVersion string     `json:"server_version"`
	Server        ServerInfo `json:"server"`
}

// AppUser is the signed-in user, as the app names the account.
type AppUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// FeedQuery asks for the changes after Cursor, the cursor of the last page
// applied, or 0 for everything.
type FeedQuery struct {
	Cursor int64 `query:"cursor" json:"cursor" validate:"min=0"`
	// Limit is the most entries one page holds.
	Limit int `query:"limit" json:"limit,omitempty" validate:"omitempty,min=1,max=1000"`
	// ExpiredThrough is the last page's ExpiredThrough, or 0 from the start.
	ExpiredThrough int64 `query:"expired_through" json:"expired_through,omitempty" validate:"min=0"`
}

// The kinds of record the feed sends. An app ignores a kind it doesn't
// know, and its cursor moves past it anyway (ADR 0023).
const (
	//tygo:emit export type FeedKind = typeof FeedKindBook | typeof FeedKindLibrary | typeof FeedKindReadingPosition | typeof FeedKindReadThrough;
	FeedKindBook    = "book"
	FeedKindLibrary = "library"
	// The signed-in user's own records. A Reading Position's entry is
	// keyed by its File's id.
	FeedKindReadingPosition = "reading_position"
	FeedKindReadThrough     = "read_through"
)

// FeedResponse is one page of the catalog's changes (ADR 0023).
type FeedResponse struct {
	// Entries are in the order they changed. An entry holds the record as it
	// is now, or says it was deleted.
	Entries []FeedEntry `json:"entries"`
	// Cursor is what the next request sends. The app stores it once the
	// page is applied.
	Cursor int64 `json:"cursor"`
	// HasMore says to ask again straight away.
	HasMore bool `json:"has_more"`
	// StartOver says the cursor is too old: deletions since it have
	// expired, so the app syncs again from 0 and drops what that doesn't
	// send.
	StartOver bool `json:"start_over"`
	// ExpiredThrough says how far deletions had expired when the page was
	// read. The app sends it back with the next cursor: a cursor below it is
	// stale only if deletions expired after the app last saw it, so a
	// download from the start can page past expired deletions it never had.
	ExpiredThrough int64 `json:"expired_through"`
}

// FeedEntry is one record that changed. Exactly one of the record fields
// is set, the one Kind names, unless Deleted.
type FeedEntry struct {
	Kind string `json:"kind" tstype:"FeedKind"`
	// ID is the record's id. A Reading Position has none of its own, so its
	// entry carries its File's.
	ID      int  `json:"id"`
	Deleted bool `json:"deleted"`
	// LibraryID is the Library a catalog record belongs to, and 0 for the
	// user's own records.
	LibraryID       int                  `json:"library_id"`
	Book            *FeedBook            `json:"book,omitempty"`
	Library         *FeedLibrary         `json:"library,omitempty"`
	ReadingPosition *FeedReadingPosition `json:"reading_position,omitempty"`
	ReadThrough     *FeedReadThrough     `json:"read_through,omitempty"`
}

// FeedLibrary is a Library the user can access.
type FeedLibrary struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	CoverAspectRatio string `json:"cover_aspect_ratio" tstype:"CoverAspectRatio"`
	// SortSpec is the reading user's saved sort for the Library, in the web
	// app's sort parameter form ("author:asc,date_added:desc"), or empty
	// for the default, newest first. A field the app doesn't know is
	// skipped.
	SortSpec string `json:"sort_spec"`
}

// FeedBook is a Book as the app keeps it.
type FeedBook struct {
	ID        int     `json:"id"`
	LibraryID int     `json:"library_id"`
	Title     string  `json:"title"`
	Subtitle  *string `json:"subtitle"`
	SortTitle string  `json:"sort_title"`
	// CreatedAt is when the Book was added, which the Library sorts by,
	// newest first, as the web app does by default.
	CreatedAt time.Time `json:"created_at"`
	// Authors and Series are in the Book's order.
	Authors []FeedAuthor           `json:"authors"`
	Series  []FeedSeriesMembership `json:"series"`
	Files   []FeedFile             `json:"files"`
	// CoverVersion keys the Book's cover, so its URL can be cached as
	// immutable. It's empty for a Book with no cover.
	CoverVersion string `json:"cover_version"`
}

// FeedAuthor is one of a Book's authors.
type FeedAuthor struct {
	PersonID int     `json:"person_id"`
	Name     string  `json:"name"`
	SortName string  `json:"sort_name"`
	Role     *string `json:"role" tstype:"AuthorRole"`
}

// FeedSeriesMembership is a Book's place in a Series.
type FeedSeriesMembership struct {
	SeriesID  int      `json:"series_id"`
	Name      string   `json:"name"`
	SortName  string   `json:"sort_name"`
	Number    *float64 `json:"number"`
	NumberEnd *float64 `json:"number_end"`
	Unit      *string  `json:"unit" tstype:"SeriesNumberUnit"`
}

// FeedFile is one of a Book's Files. An app that doesn't know its File Type
// shows it as not supported in that version (ADR 0021).
type FeedFile struct {
	ID        int    `json:"id"`
	FileType  string `json:"file_type" tstype:"FileType"`
	FileRole  string `json:"file_role" tstype:"FileRole"`
	SizeBytes int64  `json:"size_bytes"`
	// DisplayName labels the File among its Book's Files, as the web app
	// does: its name, or its filename (File.ResolveDisplayName).
	DisplayName string `json:"display_name"`
	// The Library's release date, page count, and duration sorts read the
	// Book's newest File that has each.
	ReleaseDate              *time.Time `json:"release_date"`
	PageCount                *int       `json:"page_count"`
	AudiobookDurationSeconds *float64   `json:"audiobook_duration_seconds"`
	// Chapters are Shisho's chapter list, which the app navigates by
	// instead of the file's own (ADR 0022), in order, each after its
	// parent.
	Chapters []FeedChapter `json:"chapters"`
}

// FeedChapter is one of a File's chapters. Where it starts takes the form
// the File Type reads: StartTimestampMs for an audiobook, StartPage for a
// Page-based File, and Href for an EPUB.
type FeedChapter struct {
	ID               int     `json:"id"`
	ParentID         *int    `json:"parent_id"`
	Title            string  `json:"title"`
	StartTimestampMs *int64  `json:"start_timestamp_ms"`
	StartPage        *int    `json:"start_page"`
	Href             *string `json:"href"`
}

// FeedReadingPosition is the user's place in a File, the newest any source
// reported (the web, a device, or this or another installation of the app).
// The locator is in one form, the one the source reads; a form the app
// can't go to, such as KOReader's, is opened at Percentage.
type FeedReadingPosition struct {
	FileID     int      `json:"file_id"`
	CFI        *string  `json:"cfi"`
	Page       *int     `json:"page"`
	Seconds    *float64 `json:"seconds"`
	Percentage float64  `json:"percentage"`
	// OccurredAt is when the reader was there, after the server corrected
	// the clock of the device that sent it.
	OccurredAt time.Time `json:"occurred_at"`
	Source     string    `json:"source" tstype:"ReadingSource"`
	// DeviceID is the Device that reported it, so an installation knows its
	// own, or null for the web.
	DeviceID *string `json:"device_id"`
}

// FeedReadThrough is one of the user's passes through a Book (ADR 0013).
// BookID is null once its Book was deleted.
type FeedReadThrough struct {
	ID       int     `json:"id"`
	BookID   *int    `json:"book_id"`
	BeginDay *string `json:"begin_day"`
	EndDay   *string `json:"end_day"`
	Outcome  string  `json:"outcome" tstype:"ReadThroughOutcome"`
	Paused   bool    `json:"paused"`
}

// JournalPayload is a batch of the app's journal (ADR 0024): events made on
// the device, each judged on its own. Events are raw so that one the server
// doesn't understand, an unknown type or field, is answered for that event
// alone instead of refusing the batch.
type JournalPayload struct {
	// Clock is the device's clock when it sent the batch, which the server
	// corrects the events' times by when it's far off.
	Clock  time.Time         `json:"clock" validate:"required"`
	Events []json.RawMessage `json:"events" validate:"max=500" tstype:"JournalEvent[]"`
}

// The journal's event types. A new kind of change the app can make is a new
// type, not a new protocol (ADR 0024).
const (
	//tygo:emit export type JournalEventType = typeof JournalEventReadingPosition | typeof JournalEventReadingSession;
	JournalEventReadingPosition = "reading_position"
	JournalEventReadingSession  = "reading_session"
)

// JournalEvent is one change the app made. ID is unique to the
// installation and never reused; At is when it happened, on the device's
// clock.
type JournalEvent struct {
	ID      string          `json:"id"`
	Type    string          `json:"type" tstype:"JournalEventType"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload" tstype:"JournalPositionPayload | JournalSessionPayload"`
}

// JournalPositionPayload is a Reading Position update: where the user is in
// a File, in one form, plus how far through it that is.
type JournalPositionPayload struct {
	FileID     int      `json:"file_id" validate:"required,min=1"`
	CFI        *string  `json:"cfi,omitempty" validate:"omitempty,min=1,max=4096"`
	Page       *int     `json:"page,omitempty" validate:"omitempty,min=0"`
	Seconds    *float64 `json:"seconds,omitempty" validate:"omitempty,min=0"`
	Percentage float64  `json:"percentage" validate:"min=0,max=1"`
	// AtEnd is playback or the reader reaching the end of the File, which
	// finishes the Read-through underway, as in the web readers.
	AtEnd bool `json:"at_end,omitempty"`
	// ListenedSeconds is how long this sitting has played. A minute of it
	// starts or resumes a Read-through, as in the web player.
	ListenedSeconds *float64 `json:"listened_seconds,omitempty" validate:"omitempty,min=0"`
	// ListeningSince is when this sitting began, on the device's clock: a
	// Read-through it starts begins that day.
	ListeningSince *time.Time `json:"listening_since,omitempty"`
}

// JournalSessionPayload is a finished Reading Session, timed on the device
// with the web reader's rules, on the device's clock.
type JournalSessionPayload struct {
	FileID          int       `json:"file_id" validate:"required,min=1"`
	StartedAt       time.Time `json:"started_at" validate:"required"`
	EndedAt         time.Time `json:"ended_at" validate:"required"`
	StartPercentage float64   `json:"start_percentage" validate:"min=0,max=1"`
	EndPercentage   float64   `json:"end_percentage" validate:"min=0,max=1"`
	// Duration is the seconds the session counted, when less than its span.
	Duration *int `json:"duration,omitempty" validate:"omitempty,min=0"`
}

// What the server did with each event.
const (
	//tygo:emit export type JournalResultKind = typeof JournalAccepted | typeof JournalDuplicate | typeof JournalNotUnderstood | typeof JournalRefused;
	// JournalAccepted is applied. The app drops it.
	JournalAccepted = "accepted"
	// JournalDuplicate was taken in before, from this installation, under any
	// of its sign-ins. The app drops it.
	JournalDuplicate = "duplicate"
	// JournalNotUnderstood is a type or field this server doesn't know. The
	// app keeps it queued for when the server lists it (ADR 0024).
	JournalNotUnderstood = "not_understood"
	// JournalRefused was understood and can never apply, such as a deleted
	// File's. The app drops it and tells the user why.
	JournalRefused = "refused"
)

// JournalResponse answers every event in the batch, in its order.
type JournalResponse struct {
	Results []JournalResult `json:"results"`
	// ClockOffsetSeconds is the server's clock minus the device's when the
	// batch arrived, for the app's diagnostics.
	ClockOffsetSeconds float64 `json:"clock_offset_seconds"`
}

// JournalResult is what happened to one event. Reason says why it was
// refused or not understood, for the user and the app's diagnostics.
type JournalResult struct {
	ID     string `json:"id"`
	Result string `json:"result" tstype:"JournalResultKind"`
	Reason string `json:"reason,omitempty"`
}
