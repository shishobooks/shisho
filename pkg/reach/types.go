package reach

const (
	//tygo:emit export type NotDeliveredReason = typeof NotDeliveredOutsideScope | typeof NotDeliveredNoRule;
	// NotDeliveredOutsideScope is a book outside the Device's scope.
	NotDeliveredOutsideScope = "outside_scope"
	// NotDeliveredNoRule is a book in scope none of whose main Files the
	// Device's Delivery Rules deliver.
	NotDeliveredNoRule = "no_rule"
)

// BookDevice is what one of the current user's Devices gets of a book, for
// the book page's "On your Devices" panel, which lists every Device.
type BookDevice struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	DeviceType string `json:"device_type" tstype:"DeviceType"`
	// Deliveries are the book's Files the Device gets, each with the File
	// Type it gets it as. Empty when it gets none, and NotDeliveredReason
	// says why.
	Deliveries         []BookDelivery `json:"deliveries"`
	NotDeliveredReason string         `json:"not_delivered_reason,omitempty" tstype:"NotDeliveredReason"`
	// EditionMoves are, on a Kobo, the books on it that its next sync moves
	// to another of this book's Files. The Kobo may reopen the book at the
	// start.
	EditionMoves []EditionMove `json:"edition_moves"`
}

// BookDelivery is one File a Device gets, and the File Type it gets it as.
type BookDelivery struct {
	FileID            int    `json:"file_id"`
	SourceFileType    string `json:"source_file_type" tstype:"FileType"`
	DeliveredFileType string `json:"delivered_file_type" tstype:"DeliveredFileType"`
}

// EditionMove is a book on a Kobo that its next sync moves from one File to
// another. FromFileID is null when the File it held was deleted.
type EditionMove struct {
	FromFileID *int `json:"from_file_id" tstype:"number | null"`
	ToFileID   int  `json:"to_file_id"`
}

// DeliveryPreviewQuery is the query for GET
// /user/devices/:id/delivery-preview. With no Mode, it previews the rules
// the Device follows now; with one, the rules it would follow in that mode,
// and Rules, read only for a custom list, are "source:delivered" pairs
// separated by commas, in order ("epub:kepub,cbz:epub"). It is a GET so
// it stays read-only in Demo Mode.
type DeliveryPreviewQuery struct {
	Mode  string `query:"mode" json:"mode,omitempty" tstype:"DeliveryRulesMode"`
	Rules string `query:"rules" json:"rules,omitempty"`
}

// DeliveryPreviewResponse is what a Device would receive within its scope.
type DeliveryPreviewResponse struct {
	// ReceivedBooks counts the books it gets something of.
	ReceivedBooks int `json:"received_books"`
	// NotReceived counts the books it gets nothing of, by the File Types of
	// their main Files, in the built-in rules' order. A book with Files of
	// two types counts under both.
	NotReceived []NotReceivedCount `json:"not_received"`
	// MovedBooks counts, on a Kobo, the books on it the change moves to
	// another edition, which may reopen at the start.
	MovedBooks int `json:"moved_books"`
	// RetypedBooks counts, on a Kobo, the books on it the change switches
	// to another File Type, such as KePub to EPUB. They lose their reading
	// position and highlights.
	RetypedBooks int `json:"retyped_books"`
}

// NotReceivedCount is how many books a Device gets nothing of that have a
// main File of SourceFileType.
type NotReceivedCount struct {
	SourceFileType string `json:"source_file_type" tstype:"FileType"`
	Books          int    `json:"books"`
}
