package models

import (
	"time"

	"github.com/uptrace/bun"
)

// UserLibrarySettings is intentionally not exposed over JSON or
// generated as a TS type (`tstype:"-"`). Handlers transform it into
// LibrarySettingsResponse (in pkg/settings) so the wire shape can
// evolve independently of the storage row, and tygo skips it. The
// struct therefore carries no `json:"..."` tags.
type UserLibrarySettings struct {
	bun.BaseModel `bun:"table:user_library_settings,alias:uls" tstype:"-"`

	ID        int       `bun:",pk,autoincrement"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	UserID    int       `bun:",notnull"`
	LibraryID int       `bun:",notnull"`
	SortSpec  *string   `bun:",nullzero"`
}
