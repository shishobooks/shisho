package kobo

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncPoint tracks the state of the library at a sync, per Device.
type SyncPoint struct {
	bun.BaseModel `bun:"table:kobo_sync_points,alias:ksp"`

	ID          string     `bun:"id,pk"`
	DeviceID    string     `bun:"device_id,notnull"`
	CreatedAt   time.Time  `bun:"created_at,notnull"`
	CompletedAt *time.Time `bun:"completed_at"`

	Books []*SyncPointBook `bun:"rel:has-many,join:id=sync_point_id"`
}

// SyncPointBook is a snapshot of a file's state at a sync point.
type SyncPointBook struct {
	bun.BaseModel `bun:"table:kobo_sync_point_books,alias:kspb"`

	ID           string `bun:"id,pk"`
	SyncPointID  string `bun:"sync_point_id,notnull"`
	FileID       int    `bun:"file_id,notnull"`
	FileHash     string `bun:"file_hash,notnull"`
	FileSize     int64  `bun:"file_size,notnull"`
	MetadataHash string `bun:"metadata_hash,notnull"`
	Synced       bool   `bun:"synced,notnull,default:false"`
	// CoverModifiedAt is the cover image's mtime, so a cover replaced under
	// the same filename counts as a change. Nil on rows stored before it was
	// tracked, and for files without a cover on disk.
	CoverModifiedAt *time.Time `bun:"cover_modified_at"`
	// EntryID, DeliveredFileType, and RevisionID are the entry the file went
	// out under (SyncEntry). Rows stored before entries had ids hold the
	// file's Shisho id and KePub, and an empty RevisionID when the file had
	// no entry.
	EntryID           string `bun:"entry_id,notnull"`
	DeliveredFileType string `bun:"delivered_file_type,notnull"`
	RevisionID        string `bun:"revision_id,notnull"`
}

// SyncEntry is one book Shisho has put on a Kobo, per Device (ADR 0012):
// the book and delivered File Type it belongs to, the File it holds now,
// and the id and RevisionId it was first sent with, which never change. A
// Kobo treats every new RevisionId as a new book, and ignores a change sent
// under a RevisionId it does not have. An entry stays with its File while
// the Device's Delivery Rules still deliver that File as DeliveredFileType,
// and is then handed to another File of the book delivered as the same type,
// or removed. Entries outlive sync points: clearing sync history forces a
// fresh sync, but the books already on the device keep their ids.
type SyncEntry struct {
	bun.BaseModel `bun:"table:kobo_sync_entries,alias:kse"`

	DeviceID string `bun:"device_id,pk"`
	// EntryID is the id the Kobo knows the book by. Entries recorded before
	// they had ids of their own are "shisho-{fileID}"; newer ones can never
	// take that form (newEntryID).
	EntryID string `bun:"entry_id,pk"`
	// BookID and FileID are nil once their row is deleted. An entry without
	// a File is handed over or removed by the next sync.
	BookID            *int      `bun:"book_id"`
	FileID            *int      `bun:"file_id"`
	DeliveredFileType string    `bun:"delivered_file_type,notnull"`
	RevisionID        string    `bun:"revision_id,notnull"`
	CreatedAt         time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
	// ReadingMinutes is the reading time the Kobo last reported for the book,
	// which it counts from when the book reached it, and ReadingMinutesAt when
	// it did. An entry from before Kobo reading state synced starts without
	// one: its first report is only where counting starts.
	ReadingMinutes   *int       `bun:"reading_minutes"`
	ReadingMinutesAt *time.Time `bun:"reading_minutes_at"`
	// ReadingStatusAt is the latest status time the Kobo has seen: when it
	// last changed the status it reports, or the time of a status a sync sent
	// it. A report whose status changed no later is the Kobo repeating itself
	// or echoing Shisho.
	ReadingStatusAt *time.Time `bun:"reading_status_at"`
	// ReadingStateVersion is what the Kobo last heard of the reading state
	// (stateInputs.version), so a sync sends it again only once it changes.
	// ReadingStateSentVersion is what a sync sent it since, in the sync point
	// ReadingStateSentIn, on the page starting at ReadingStateSentCursor: the
	// Kobo has heard it once it asks for a later page or syncs from that
	// point, and until then a retry sends it again.
	ReadingStateVersion     *string `bun:"reading_state_version"`
	ReadingStateSentVersion *string `bun:"reading_state_sent_version"`
	ReadingStateSentIn      *string `bun:"reading_state_sent_in"`
	ReadingStateSentCursor  *int    `bun:"reading_state_sent_cursor"`
}
