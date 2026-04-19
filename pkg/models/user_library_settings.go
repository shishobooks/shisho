package models

import (
	"time"

	"github.com/uptrace/bun"
)

type UserLibrarySettings struct {
	bun.BaseModel `bun:"table:user_library_settings,alias:uls" tstype:"-"`

	ID        int       `bun:",pk,autoincrement"                            json:"id"`
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	UserID    int       `bun:",notnull"                                     json:"user_id"`
	LibraryID int       `bun:",notnull"                                     json:"library_id"`
	SortSpec  *string   `bun:",nullzero"                                    json:"sort_spec"`
}
