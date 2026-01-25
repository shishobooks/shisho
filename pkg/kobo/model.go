package kobo

import (
	"time"

	"github.com/uptrace/bun"
)

// SyncPoint tracks the state of the library at a sync, per API key.
type SyncPoint struct {
	bun.BaseModel `bun:"table:kobo_sync_points,alias:ksp"`

	ID          string     `bun:"id,pk"`
	APIKeyID    string     `bun:"api_key_id,notnull"`
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
}

// SyncScope represents the scope of books to sync (parsed from URL).
type SyncScope struct {
	Type      string // "all", "library", "list"
	LibraryID *int
	ListID    *int
}
