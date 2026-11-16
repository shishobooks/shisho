package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Bookmark is a moment in one audiobook File that one user marked to come
// back to, with an optional note. It's private to the user, and a File can
// have many. See "Bookmark" in CONTEXT.md.
type Bookmark struct {
	bun.BaseModel `bun:"table:bookmarks,alias:bm" tstype:"-"`

	ID        int       `bun:",pk,autoincrement" json:"id"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID    int       `bun:",notnull" json:"-"`
	FileID    int       `bun:",notnull" json:"file_id"`
	// Seconds is the moment, in seconds from the start of the File.
	Seconds float64 `bun:",notnull" json:"seconds"`
	// Note is nil when the user left none.
	Note *string `json:"note" tstype:"string | null,required"`
}
