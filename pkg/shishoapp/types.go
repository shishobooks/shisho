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
	// InstanceID tells this server apart from another set up again from
	// scratch at the same address, which hands out the same ids. The app
	// treats a sign-in with a different one as a different account. A
	// restore from backup keeps it.
	InstanceID string `json:"instance_id"`
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
	//tygo:emit export type FeedKind = typeof FeedKindBook | typeof FeedKindLibrary | typeof FeedKindSeries | typeof FeedKindPerson | typeof FeedKindGenre | typeof FeedKindTag | typeof FeedKindPublisher | typeof FeedKindBookLeft | typeof FeedKindReadingPosition | typeof FeedKindReadThrough | typeof FeedKindUserBook | typeof FeedKindList | typeof FeedKindBookmark;
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
	// One of the user's Bookmarks.
	FeedKindBookmark = "bookmark"
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
	Bookmark        *FeedBookmark        `json:"bookmark,omitempty"`
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
	// WordCount is how many words a reflowable ebook's text holds, null
	// until counted and for other File Types.
	WordCount *int `json:"word_count"`
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
	// The rest of what the web's file details show.
	AudiobookCodec      *string `json:"audiobook_codec"`
	AudiobookBitrateBps *int    `json:"audiobook_bitrate_bps"`
	// Language is a BCP 47 tag, such as "en".
	Language *string `json:"language"`
	Abridged *bool   `json:"abridged"`
	URL      *string `json:"url"`
	// ReadingDirection is a Page-based File's, or null, which runs left to
	// right.
	ReadingDirection *string `json:"reading_direction" tstype:"ReadingDirection | null"`
	// Filepath is where the server keeps the File, as the web shows it.
	Filepath       string     `json:"filepath"`
	FileModifiedAt *time.Time `json:"file_modified_at"`
	// CreatedAt is when the File was added.
	CreatedAt time.Time `json:"created_at"`
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
	// LocalID is the id an installation of the app made the List under, so
	// that installation knows it as one of its own, or null for one made on
	// the web.
	LocalID *string `json:"local_id"`
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
	// PageFlow is the flow the user chose for reading the Book's Page-based
	// Files, on the web or in the app, or null to pick from the pages'
	// shapes.
	PageFlow *string `json:"page_flow" tstype:"PageFlow | null"`
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
	// DeviceName is that Device's name as its user sees it on the Devices
	// page, or null for the web and once the Device is deleted. Renaming the
	// Device moves its positions in the feed, so the name stays current.
	DeviceName *string `json:"device_name"`
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
	// LastReadAt is its latest reading, as the web's Last read sort reads
	// it (reading.LastReadExpr): no place a link opened, and no supplement.
	// Null before any.
	LastReadAt *time.Time `json:"last_read_at"`
	// BookTitle and BookAuthors are the deleted Book's title and author
	// names, for Reading History, and null while the Book exists.
	BookTitle   *string  `json:"book_title"`
	BookAuthors []string `json:"book_authors" tstype:"string[] | null"`
}

// FeedBookmark is one of the user's Bookmarks: a moment in an audiobook
// File, with an optional note.
type FeedBookmark struct {
	ID      int     `json:"id"`
	FileID  int     `json:"file_id"`
	Seconds float64 `json:"seconds"`
	Note    *string `json:"note"`
	// CreatedAt is when it was made.
	CreatedAt time.Time `json:"created_at"`
	// LocalID is the id an installation of the app made it under, so that
	// installation knows the Bookmark as one of its own, or null for one
	// made on the web.
	LocalID *string `json:"local_id"`
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
	//tygo:emit export type JournalEventType = typeof JournalEventReadingPosition | typeof JournalEventReadingSession | typeof JournalEventBookmarkCreate | typeof JournalEventBookmarkNote | typeof JournalEventBookmarkDelete | typeof JournalEventPageFlow | typeof JournalEventReadThroughStart | typeof JournalEventReadThroughFinish | typeof JournalEventReadThroughAbandon | typeof JournalEventReadThroughPause | typeof JournalEventReadThroughResume | typeof JournalEventReadThroughDelete | typeof JournalEventReferenceMark | typeof JournalEventReferenceRemove | typeof JournalEventRatingSet | typeof JournalEventRatingClear | typeof JournalEventToReadAdd | typeof JournalEventToReadRemove | typeof JournalEventToReadMove | typeof JournalEventListCreate | typeof JournalEventListRename | typeof JournalEventListDelete | typeof JournalEventListAdd | typeof JournalEventListRemove | typeof JournalEventListMove | typeof JournalEventPositionClear;
	JournalEventReadingPosition = "reading_position"
	JournalEventReadingSession  = "reading_session"
	// The Bookmark events, sent only to a server that lists
	// CapabilityBookmarks.
	JournalEventBookmarkCreate = "bookmark_create"
	JournalEventBookmarkNote   = "bookmark_note"
	JournalEventBookmarkDelete = "bookmark_delete"
	// The Page flow event, sent only to a server that lists
	// CapabilityPageFlow.
	JournalEventPageFlow = "page_flow"
	// The Read-through and Reference events, sent only to a server that
	// lists CapabilityReadingStatus. Each does what the web's status menu
	// does, at the event's time (ADR 0028 orders them).
	JournalEventReadThroughStart   = "read_through_start"
	JournalEventReadThroughFinish  = "read_through_finish"
	JournalEventReadThroughAbandon = "read_through_abandon"
	JournalEventReadThroughPause   = "read_through_pause"
	JournalEventReadThroughResume  = "read_through_resume"
	JournalEventReadThroughDelete  = "read_through_delete"
	JournalEventReferenceMark      = "reference_mark"
	JournalEventReferenceRemove    = "reference_remove"
	// The Rating events, sent only to a server that lists CapabilityRatings.
	JournalEventRatingSet   = "rating_set"
	JournalEventRatingClear = "rating_clear"
	// The To Read events, sent only to a server that lists CapabilityToRead.
	JournalEventToReadAdd    = "to_read_add"
	JournalEventToReadRemove = "to_read_remove"
	JournalEventToReadMove   = "to_read_move"
	// The List events, sent only to a server that lists CapabilityLists.
	JournalEventListCreate = "list_create"
	JournalEventListRename = "list_rename"
	JournalEventListDelete = "list_delete"
	JournalEventListAdd    = "list_add"
	JournalEventListRemove = "list_remove"
	JournalEventListMove   = "list_move"
	// The event that clears a File's saved place, sent only to a server
	// that lists CapabilityClearPosition.
	JournalEventPositionClear = "position_clear"
)

// JournalEvent is one change the app made. ID is unique to the
// installation and never reused; At is when it happened, on the device's
// clock.
type JournalEvent struct {
	ID      string          `json:"id"`
	Type    string          `json:"type" tstype:"JournalEventType"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload" tstype:"JournalPositionPayload | JournalSessionPayload | JournalBookmarkCreatePayload | JournalBookmarkNotePayload | JournalBookmarkRef | JournalPageFlowPayload | JournalBookPayload | JournalReadThroughDeletePayload | JournalRatingPayload | JournalToReadMovePayload | JournalListCreatePayload | JournalListRenamePayload | JournalListRef | JournalListBookPayload | JournalListMovePayload | JournalFilePayload"`
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

// JournalBookmarkCreatePayload adds a Bookmark, made at the event's time.
type JournalBookmarkCreatePayload struct {
	// LocalID is the installation's own id for the Bookmark, a UUID, which
	// later events name it by until the feed brings its server id. A
	// Bookmark is made once per LocalID: sent again, even after it was
	// deleted, it changes nothing.
	LocalID string   `json:"local_id" validate:"required,max=64"`
	FileID  int      `json:"file_id" validate:"required,min=1"`
	Seconds *float64 `json:"seconds" validate:"required,gte=0" tstype:"number,required"`
	// Note is optional; a blank one is no note.
	Note *string `json:"note,omitempty" validate:"omitempty,max=2000"`
}

// JournalBookmarkRef names one of the user's Bookmarks, by exactly one of
// its server id, from the feed, or the LocalID the installation made it
// under. It's the whole payload of a delete. Naming one that's gone, or
// never was the user's, is taken in and changes nothing: a delete wins
// (ADR 0024).
type JournalBookmarkRef struct {
	BookmarkID *int    `json:"bookmark_id,omitempty" validate:"omitempty,min=1"`
	LocalID    *string `json:"local_id,omitempty" validate:"omitempty,min=1,max=64"`
}

// JournalBookmarkNotePayload replaces a Bookmark's note, unless it changed
// later than the event, on the web or another installation.
type JournalBookmarkNotePayload struct {
	JournalBookmarkRef
	// Note is required, so an event that leaves it out can't clear it by
	// accident; a blank one clears it.
	Note *string `json:"note" validate:"required,max=2000" tstype:"string,required"`
}

// JournalPageFlowPayload saves the Page flow the user chose for a Book,
// chosen at the event's time: a choice made later elsewhere keeps.
type JournalPageFlowPayload struct {
	BookID   int     `json:"book_id" validate:"required,min=1"`
	PageFlow *string `json:"page_flow" validate:"required,oneof=paginated scrolled" tstype:"PageFlow"`
}

// JournalBookPayload names the Book a status, Reference, Rating clear, or
// To Read event changes. Day is the phone's own day at the event, which
// dates a Read-through it starts or ends when the user has set no time
// zone on the server.
type JournalBookPayload struct {
	BookID int    `json:"book_id" validate:"required,min=1"`
	Day    string `json:"day,omitempty" validate:"omitempty,date"`
}

// JournalReadThroughDeletePayload deletes one of the user's Read-throughs:
// ReadThroughID, from the feed, or, with BookID instead, the one underway
// in that Book, as Mark as not started does, unless it began after Day.
// One already gone stays gone.
type JournalReadThroughDeletePayload struct {
	ReadThroughID *int   `json:"read_through_id,omitempty" validate:"omitempty,min=1"`
	BookID        *int   `json:"book_id,omitempty" validate:"omitempty,min=1"`
	Day           string `json:"day,omitempty" validate:"omitempty,date"`
}

// JournalRatingPayload sets a Book's Rating, in quarter stars from 1 to 20,
// unless it changed later elsewhere: the latest instant wins.
type JournalRatingPayload struct {
	BookID int `json:"book_id" validate:"required,min=1"`
	Rating int `json:"rating" validate:"required,min=1,max=20"`
}

// JournalToReadMovePayload moves a Book on To Read to just after
// AfterBookID, or to the top when it's null. The latest move of each Book
// wins.
type JournalToReadMovePayload struct {
	BookID      int  `json:"book_id" validate:"required,min=1"`
	AfterBookID *int `json:"after_book_id,omitempty" validate:"omitempty,min=1"`
}

// JournalListRef names a List by exactly one of its server id, from the
// feed, or the LocalID the installation made it under.
type JournalListRef struct {
	ListID  *int    `json:"list_id,omitempty" validate:"omitempty,min=1"`
	LocalID *string `json:"local_id,omitempty" validate:"omitempty,min=1,max=64"`
}

// JournalListCreatePayload makes a List the user owns, once per LocalID:
// sent again, even after it was deleted, it changes nothing.
type JournalListCreatePayload struct {
	// LocalID is the installation's own id for the List, a UUID, which
	// later events name it by until the feed brings its server id.
	LocalID     string  `json:"local_id" validate:"required,max=64"`
	Name        string  `json:"name" mod:"trim" validate:"required,min=1,max=200"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
	IsOrdered   bool    `json:"is_ordered,omitempty"`
}

// JournalListRenamePayload renames a List the user owns or manages.
type JournalListRenamePayload struct {
	JournalListRef
	Name string `json:"name" mod:"trim" validate:"required,min=1,max=200"`
}

// JournalListBookPayload adds a Book to a List, or removes it, for a user
// who can edit the List. A removal doesn't undo an add made later elsewhere.
type JournalListBookPayload struct {
	JournalListRef
	BookID int `json:"book_id" validate:"required,min=1"`
}

// JournalListMovePayload moves a Book in an ordered List to just after
// AfterBookID, or to the top when it's null. The latest move of each Book
// wins.
type JournalListMovePayload struct {
	JournalListRef
	BookID      int  `json:"book_id" validate:"required,min=1"`
	AfterBookID *int `json:"after_book_id,omitempty" validate:"omitempty,min=1"`
}

// JournalFilePayload names the File whose saved place a clear removes. A
// place saved after the event, such as by another reader, stands.
type JournalFilePayload struct {
	FileID int `json:"file_id" validate:"required,min=1"`
}

// StatsQuery picks the range the user's Reading Stats cover: one calendar
// year, or all time when Year is omitted.
type StatsQuery struct {
	Year *int `query:"year" json:"year,omitempty" validate:"omitempty,min=1,max=9999"`
}

// StatsResponse is the user's Reading Stats for one range, as the web's
// Reading Stats page shows them. The app keeps a copy of each range it
// shows, and adds its own unsent reading on top until the next sync.
type StatsResponse struct {
	// Year is the year covered, or null for all time.
	Year *int `json:"year"`
	// Years are the years with any reading, newest first.
	Years []int `json:"years"`
	// TimeZone is the IANA zone the server groups reading into days in: the
	// user's, or UTC when they set none.
	TimeZone string             `json:"time_zone"`
	Totals   StatsTotals        `json:"totals"`
	Pace     StatsPace          `json:"pace"`
	Periods  []StatsPeriod      `json:"periods"`
	Genres   []StatsCount       `json:"genres"`
	Tags     []StatsCount       `json:"tags"`
	Authors  []StatsCount       `json:"authors"`
	Kinds    []StatsKindCount   `json:"kinds"`
	Ratings  []StatsRatingCount `json:"ratings"`
	Places   []StatsPlace       `json:"places"`
}

// StatsTotals are the range's summary figures.
type StatsTotals struct {
	// Finished counts finished Read-throughs, rereads included, and Rereads
	// how many of those were of a Book finished before.
	Finished int `json:"finished"`
	Rereads  int `json:"rereads"`
	// FinishedBooks is how many distinct Books the breakdowns count from,
	// each finished Book once, so a row's share is its Books over this. Null
	// from a server before it was sent.
	FinishedBooks *int `json:"finished_books"`
	DidNotFinish  int  `json:"did_not_finish"`
	// ReadingSeconds and ListeningSeconds are Reading Session time.
	ReadingSeconds   int `json:"reading_seconds"`
	ListeningSeconds int `json:"listening_seconds"`
	// WordsRead is how far the user read through Reflowable Files with a
	// word count, in words.
	WordsRead int `json:"words_read"`
	// AverageRating is the mean Rating, in stars, of the finished Books the
	// user rated, or null with none.
	AverageRating *float64 `json:"average_rating"`
}

// StatsPace is how fast the user reads and listens, per kind of File, each
// null without enough timed reading.
type StatsPace struct {
	WordsPerMinute    *float64 `json:"words_per_minute"`
	ComicPagesPerHour *float64 `json:"comic_pages_per_hour"`
	PDFPagesPerHour   *float64 `json:"pdf_pages_per_hour"`
	// ListeningSpeed is audiobook time covered per second spent.
	ListeningSpeed *float64 `json:"listening_speed"`
}

// StatsPeriod is one month of a year, or one year of all time.
type StatsPeriod struct {
	Year int `json:"year"`
	// Month is 1 to 12, or null when the period is the whole year.
	Month            *int `json:"month"`
	Finished         int  `json:"finished"`
	ReadingSeconds   int  `json:"reading_seconds"`
	ListeningSeconds int  `json:"listening_seconds"`
}

// StatsCount is how many distinct finished Books fall under one name.
type StatsCount struct {
	Name  string `json:"name"`
	Books int    `json:"books"`
}

// StatsKindCount is how many distinct finished Books were read as one kind:
// "ebook", "comic", "audiobook", or "unknown".
type StatsKindCount struct {
	Kind  string `json:"kind"`
	Books int    `json:"books"`
}

// StatsRatingCount is how many finished Books have one Rating, in stars.
type StatsRatingCount struct {
	Rating float64 `json:"rating"`
	Books  int     `json:"books"`
}

// StatsPlace is the time spent reading in one place: the web reader, one of
// the user's Devices (DeviceID and DeviceName), or a device that's no
// longer one of them.
type StatsPlace struct {
	Source           string  `json:"source" tstype:"ReadingSource"`
	DeviceID         *string `json:"device_id"`
	DeviceName       *string `json:"device_name"`
	ReadingSeconds   int     `json:"reading_seconds"`
	ListeningSeconds int     `json:"listening_seconds"`
}

// StatsHeatmapQuery picks the heatmap's year, or the last 12 months when
// Year is omitted.
type StatsHeatmapQuery struct {
	Year *int `query:"year" json:"year,omitempty" validate:"omitempty,min=1,max=9999"`
}

// StatsHeatmapResponse is the user's reading a day at a time, for the
// heatmap on Reading Stats' Calendar tab, as the web's. Days are in the
// user's time zone, a Session past midnight split between both. The app
// keeps a copy of each it shows (CapabilityReadingCalendar).
type StatsHeatmapResponse struct {
	// Today is the user's today.
	Today string `json:"today"`
	// From and To are the heatmap's first and last days: the last 12 months
	// up to Today, or the whole year asked for.
	From string `json:"from"`
	To   string `json:"to"`
	// FirstDay is the earliest day the user read or began or ended a
	// Read-through, which the calendar goes back to. Null with no reading
	// at all.
	FirstDay *string `json:"first_day"`
	// Days are the days from From to To with Session time, in order.
	Days []StatsDay `json:"days"`
}

// StatsDay is the Session time that fell on one day.
type StatsDay struct {
	Day              string `json:"day"`
	ReadingSeconds   int    `json:"reading_seconds"`
	ListeningSeconds int    `json:"listening_seconds"`
}

// StatsCalendarQuery picks the month the calendar shows.
type StatsCalendarQuery struct {
	Year  int `query:"year" json:"year" validate:"required,min=1,max=9999"`
	Month int `query:"month" json:"month" validate:"required,min=1,max=12"`
}

// StatsCalendarResponse is one month of the Reading Stats calendar: each
// day with reading or a finish, and the Books on them.
type StatsCalendarResponse struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	// Books are every Book read or finished in the month, by title.
	Books []StatsBookRef `json:"books"`
	// Days are the days with anything on them, in order.
	Days []StatsCalendarDay `json:"days"`
}

// StatsCalendarDay is one day of the calendar and the Books on it.
type StatsCalendarDay struct {
	Day   string                 `json:"day"`
	Books []StatsCalendarDayBook `json:"books"`
}

// StatsCalendarDayBook is one Book on one calendar day: the Session time
// spent on it that day, and whether a Read-through of it finished that day.
type StatsCalendarDayBook struct {
	Key      string `json:"key"`
	Seconds  int    `json:"seconds"`
	Finished bool   `json:"finished"`
}

// StatsBookRef names a Book on the calendar and in a day's detail.
type StatsBookRef struct {
	// Key stays the same for a Book in every month, so it picks the Book's
	// color the way the web's calendar does.
	Key   string `json:"key"`
	Title string `json:"title"`
	// Removed is true when the Book was deleted.
	Removed bool `json:"removed"`
	// BookID is the Book when the user can still reach it, which the app
	// finds in its own catalog: null when it was deleted or sits in a
	// library they no longer have access to.
	BookID *int `json:"book_id"`
}

// StatsDayQuery picks the day whose Sessions the calendar lists.
type StatsDayQuery struct {
	Day string `query:"day" json:"day" validate:"required,date"`
}

// StatsDayResponse is one day's Reading Sessions, in the user's time zone,
// and the Read-throughs finished that day.
type StatsDayResponse struct {
	Day string `json:"day"`
	// TimeZone is the IANA zone the day is in: the user's, or UTC when they
	// set none.
	TimeZone string            `json:"time_zone"`
	Sessions []StatsDaySession `json:"sessions"`
	Finished []StatsBookRef    `json:"finished"`
}

// StatsDaySession is one Reading Session that touched the day, in the order
// they started. StartedAt and EndedAt are the whole Session, which may begin
// the day before or end the day after; Seconds is its time on this day.
type StatsDaySession struct {
	ID              int          `json:"id"`
	Book            StatsBookRef `json:"book"`
	StartedAt       time.Time    `json:"started_at"`
	EndedAt         time.Time    `json:"ended_at"`
	Seconds         int          `json:"seconds"`
	StartPercentage float64      `json:"start_percentage"`
	EndPercentage   float64      `json:"end_percentage"`
	Kind            string       `json:"kind" tstype:"ReadingKind"`
	Source          string       `json:"source" tstype:"ReadingSource"`
	// Measured is false for a Session its source estimated.
	Measured bool `json:"measured"`
	// DeviceID and DeviceName are the user's Device it came from, null for
	// the web reader or a Device since deleted.
	DeviceID   *string `json:"device_id"`
	DeviceName *string `json:"device_name"`
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
