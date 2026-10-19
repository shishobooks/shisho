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
}

// SyncEntry records, per Device, the RevisionId a File was first sent to
// the Kobo with. A Kobo treats every new RevisionId as a new book, and
// ignores a change sent under a RevisionId it does not have, so the
// RevisionId never changes once recorded (ADR 0012). Entries outlive sync
// points: clearing sync history forces a fresh sync, but the books already
// on the device keep their RevisionIds.
type SyncEntry struct {
	bun.BaseModel `bun:"table:kobo_sync_entries,alias:kse"`

	DeviceID   string    `bun:"device_id,pk"`
	FileID     int       `bun:"file_id,pk"`
	RevisionID string    `bun:"revision_id,notnull"`
	CreatedAt  time.Time `bun:"created_at,nullzero,notnull,default:current_timestamp"`
}
