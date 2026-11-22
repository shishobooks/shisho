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
	// LibraryID or ListID fetches one Library or List the user just gained
	// whole, as Access says (ADR 0023): only its records, from its own
	// cursor, starting at 0. One outside the user's reach is a 404.
	LibraryID int `query:"library_id" json:"library_id,omitempty" validate:"min=0,excluded_with=ListID"`
	ListID    int `query:"list_id" json:"list_id,omitempty" validate:"min=0"`
}

// The kinds of record the feed sends. An app ignores a kind it doesn't
// know, and its cursor moves past it anyway (ADR 0023).
const (
	//tygo:emit export type FeedKind = typeof FeedKindBook | typeof FeedKindLibrary | typeof FeedKindSeries | typeof FeedKindPerson | typeof FeedKindGenre | typeof FeedKindTag | typeof FeedKindPublisher | typeof FeedKindBookLeft | typeof FeedKindReadingPosition | typeof FeedKindReadThrough | typeof FeedKindUserBook | typeof FeedKindList;
	FeedKindBook      = "book"
	FeedKindLibrary   = "library"
	FeedKindSeries    = "series"
	FeedKindPerson    = "person"
	FeedKindGenre     = "genre"
	FeedKindTag       = "tag"
	FeedKindPublisher = "publisher"
	// FeedKindBookLeft is always a deletion: the Book moved out of the
	// Library the entry names. The app drops it unless it's since in
	// another Library there.
	FeedKindBookLeft = "book_left"
	// The signed-in user's own records. A Reading Position's entry is
	// keyed by its File's id, and a user Book's by its Book's.
	FeedKindReadingPosition = "reading_position"
	FeedKindReadThrough     = "read_through"
	FeedKindUserBook        = "user_book"
	// A List the user owns or that's shared with them.
	FeedKindList = "list"
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
	// Access is what the user can see now, answered outside the cursor
	// (ADR 0023): the app drops a Library or List it has that isn't listed,
	// and fetches one listed that it doesn't have whole, with FeedQuery's
	// LibraryID or ListID, since its entries are behind the cursor.
	Access FeedAccess `json:"access"`
}

// FeedAccess lists the Libraries and Lists the user can see, in id order.
type FeedAccess struct {
	LibraryIDs []int `json:"library_ids"`
	ListIDs    []int `json:"list_ids"`
	// BehindLibraryIDs are those of LibraryIDs with records at or before
	// the cursor the page was asked for, which the feed won't send again. A
	// Library the app gains needs fetching whole only when it's listed: one
	// made since, say, arrives in the feed. A Library or List's whole fetch
	// leaves it empty, so the app fetches whatever it gains then.
	BehindLibraryIDs []int `json:"behind_library_ids"`
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
	Series          *FeedSeries          `json:"series,omitempty"`
	Person          *FeedPerson          `json:"person,omitempty"`
	Genre           *FeedGenre           `json:"genre,omitempty"`
	Tag             *FeedTag             `json:"tag,omitempty"`
	Publisher       *FeedPublisher       `json:"publisher,omitempty"`
	ReadingPosition *FeedReadingPosition `json:"reading_position,omitempty"`
	ReadThrough     *FeedReadThrough     `json:"read_through,omitempty"`
	UserBook        *FeedUserBook        `json:"user_book,omitempty"`
	List            *FeedList            `json:"list,omitempty"`
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
	// Description is the Book's, as plain text or HTML, as the web app
	// shows it.
	Description *string `json:"description"`
	// CreatedAt is when the Book was added, which the Library sorts by,
	// newest first, as the web app does by default.
	CreatedAt time.Time `json:"created_at"`
	// Authors and Series are in the Book's order.
	Authors []FeedAuthor           `json:"authors"`
	Series  []FeedSeriesMembership `json:"series"`
	// Genres and Tags are by name.
	Genres []FeedGenreRef `json:"genres"`
	Tags   []FeedTagRef   `json:"tags"`
	Files  []FeedFile     `json:"files"`
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

// FeedGenreRef is one of a Book's genres.
type FeedGenreRef struct {
	GenreID int    `json:"genre_id"`
	Name    string `json:"name"`
}

// FeedTagRef is one of a Book's tags.
type FeedTagRef struct {
	TagID int    `json:"tag_id"`
	Name  string `json:"name"`
}

// FeedPublisherRef is a File's publisher.
type FeedPublisherRef struct {
	PublisherID int    `json:"publisher_id"`
	Name        string `json:"name"`
}

// FeedIdentifier is one of a File's identifiers, such as its ISBN, in the
// form the server stores it.
type FeedIdentifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
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
	// CoverVersion keys the File's own cover (GET
	// /mobile/files/:id/cover?v=), which can differ from its Book's, so it
	// can be cached as immutable. It's empty for a File with no cover.
	CoverVersion string `json:"cover_version"`
	// Narrators are the File's, in order.
	Narrators []FeedNarrator `json:"narrators"`
	// Chapters are Shisho's chapter list, which the app navigates by
	// instead of the file's own (ADR 0022), in order, each after its
	// parent.
	Chapters []FeedChapter `json:"chapters"`
	// Publisher is the File's, or null.
	Publisher   *FeedPublisherRef `json:"publisher"`
	Identifiers []FeedIdentifier  `json:"identifiers"`
	// Pages is a CBZ's pages, as the names of its archive entries in the
	// order the server counts and sorts them (ADR 0025), so a page number
	// means the same page everywhere. It's empty for every other File
	// Type, and for a CBZ the server couldn't read.
	Pages []string `json:"pages"`
}

// FeedNarrator is one of a File's narrators.
type FeedNarrator struct {
	PersonID int    `json:"person_id"`
	Name     string `json:"name"`
	SortName string `json:"sort_name"`
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

// FeedSeries is a Series in a Library the user can access. Its Books name
// it in their Series memberships.
type FeedSeries struct {
	ID          int     `json:"id"`
	LibraryID   int     `json:"library_id"`
	Name        string  `json:"name"`
	SortName    string  `json:"sort_name"`
	Description *string `json:"description"`
	// Aliases are other names it's found by, as search finds it on the web.
	Aliases []string `json:"aliases"`
}

// FeedPerson is an author or narrator in a Library the user can access.
type FeedPerson struct {
	ID        int      `json:"id"`
	LibraryID int      `json:"library_id"`
	Name      string   `json:"name"`
	SortName  string   `json:"sort_name"`
	Aliases   []string `json:"aliases"`
}

// FeedGenre is a genre in a Library the user can access.
type FeedGenre struct {
	ID        int      `json:"id"`
	LibraryID int      `json:"library_id"`
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
}

// FeedTag is a tag in a Library the user can access.
type FeedTag struct {
	ID        int      `json:"id"`
	LibraryID int      `json:"library_id"`
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
}

// FeedPublisher is a publisher in a Library the user can access. ParentID
// is the publisher it's an imprint of, or null.
type FeedPublisher struct {
	ID        int      `json:"id"`
	LibraryID int      `json:"library_id"`
	Name      string   `json:"name"`
	ParentID  *int     `json:"parent_id"`
	Aliases   []string `json:"aliases"`
}

// The user's standing with a List.
const (
	//tygo:emit export type FeedListPermission = typeof FeedListOwner | typeof FeedListViewer | typeof FeedListEditor | typeof FeedListManager;
	FeedListOwner   = "owner"
	FeedListViewer  = "viewer"
	FeedListEditor  = "editor"
	FeedListManager = "manager"
)

// FeedList is a List the user owns or that's shared with them.
type FeedList struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	// IsOrdered is a List whose Books are in the order its people chose.
	IsOrdered   bool    `json:"is_ordered"`
	DefaultSort string  `json:"default_sort" tstype:"ListSort"`
	Owner       AppUser `json:"owner"`
	// Permission is what the user may do with it: "owner", or the List
	// Share's permission.
	Permission string `json:"permission" tstype:"FeedListPermission"`
	// Books are in the List's order: by SortOrder, then most recently
	// added. A Book in a Library the user can't access is left out, so the
	// app fetches its Lists again when its access to a Library changes.
	Books []FeedListBook `json:"books"`
	// Shares are who the List is shared with, as the web app shows them,
	// for its owner and managers; empty for anyone else.
	Shares []FeedListShare `json:"shares"`
}

// FeedListShare is one user a List is shared with, and what they may do.
type FeedListShare struct {
	User       AppUser `json:"user"`
	Permission string  `json:"permission" tstype:"ListPermission"`
}

// FeedListBook is one of a List's Books.
type FeedListBook struct {
	BookID    int       `json:"book_id"`
	SortOrder *int      `json:"sort_order"`
	AddedAt   time.Time `json:"added_at"`
}

// FeedUserBook is the user's own record for a Book: their Rating and To
// Read. Their Reading Status follows from it and their Read-throughs, as
// the server derives it.
type FeedUserBook struct {
	BookID int `json:"book_id"`
	// Rating is in quarter stars, 1 to 20, or null when unrated.
	Rating *int `json:"rating"`
	// ToReadPosition is the Book's place on To Read, or null when it isn't
	// on it.
	ToReadPosition *int `json:"to_read_position"`
	// Reference is the user's choice to dip into the Book rather than read
	// it through (ADR 0028).
	Reference bool `json:"reference"`
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
	// ListenedSeconds is how long this sitting has played. Five seconds of
	// it start or resume a Read-through, as in the web player.
	ListenedSeconds *float64 `json:"listened_seconds,omitempty" validate:"omitempty,min=0"`
	// ListeningSince is when this sitting began, on the device's clock: a
	// Read-through it starts begins that day.
	ListeningSince *time.Time `json:"listening_since,omitempty"`
}

// JournalSessionPayload is a Reading Session, timed on the device with the
// web reader's rules, on the device's clock: a finished one, or one so far.
type JournalSessionPayload struct {
	// SessionID is the device's own id for the session, unique to the
	// installation; the server's id for it is a different number. The app
	// sends the session so far on every pause under it, and the server
	// keeps one session for it, extended by each later snapshot. A session
	// the user deleted stays deleted. Without it, each event is a session of
	// its own.
	SessionID       string    `json:"session_id,omitempty" validate:"omitempty,max=64"`
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
