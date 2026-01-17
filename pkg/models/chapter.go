package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Chapter struct {
	bun.BaseModel `bun:"table:chapters,alias:ch" tstype:"-"`

	ID        int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",notnull" json:"file_id"`
	ParentID  *int      `json:"parent_id"`
	SortOrder int       `bun:",notnull" json:"sort_order"`
	Title     string    `bun:",notnull" json:"title"`

	// Position data (mutually exclusive based on file type)
	StartPage        *int    `json:"start_page"`         // CBZ: 0-indexed page number
	StartTimestampMs *int64  `json:"start_timestamp_ms"` // M4B: milliseconds from start
	Href             *string `json:"href"`               // EPUB: content document href

	// Relations
	File     *File      `bun:"rel:belongs-to,join:file_id=id" json:"-"`
	Parent   *Chapter   `bun:"rel:belongs-to,join:parent_id=id" json:"-"`
	Children []*Chapter `bun:"rel:has-many,join:id=parent_id" json:"children,omitempty"`
}
