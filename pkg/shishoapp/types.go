package shishoapp

// The Shisho app's Device surface (ADR 0020). These types are a contract
// with app releases that can't be forced to update: within a protocol,
// responses only grow, and request fields stay strict. Removing a response
// field or changing its type means a new protocol. The app decodes them
// leniently, ignoring fields it doesn't know and mapping unknown enum values
// to an unknown case (ADR 0021).

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
}

// The kinds of record the feed sends. An app ignores a kind it doesn't
// know, and its cursor moves past it anyway (ADR 0023).
const (
	//tygo:emit export type FeedKind = typeof FeedKindBook | typeof FeedKindLibrary;
	FeedKindBook    = "book"
	FeedKindLibrary = "library"
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
}

// FeedEntry is one record that changed. Exactly one of the record fields
// is set, the one Kind names, unless Deleted.
type FeedEntry struct {
	Kind    string `json:"kind" tstype:"FeedKind"`
	ID      int    `json:"id"`
	Deleted bool   `json:"deleted"`
	// LibraryID is the Library the record belongs to.
	LibraryID int          `json:"library_id"`
	Book      *FeedBook    `json:"book,omitempty"`
	Library   *FeedLibrary `json:"library,omitempty"`
}

// FeedLibrary is a Library the user can access.
type FeedLibrary struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	CoverAspectRatio string `json:"cover_aspect_ratio" tstype:"CoverAspectRatio"`
}

// FeedBook is a Book as the app keeps it.
type FeedBook struct {
	ID        int     `json:"id"`
	LibraryID int     `json:"library_id"`
	Title     string  `json:"title"`
	Subtitle  *string `json:"subtitle"`
	SortTitle string  `json:"sort_title"`
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
}
