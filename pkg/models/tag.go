package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Tag struct {
	bun.BaseModel `bun:"table:tags,alias:t" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Name      string    `bun:",nullzero" json:"name"`
	BookCount int       `bun:",scanonly" json:"book_count"`
}

type BookTag struct {
	bun.BaseModel `bun:"table:book_tags,alias:bt" tstype:"-"`

	ID     int  `bun:",pk,nullzero" json:"id"`
	BookID int  `bun:",nullzero" json:"book_id"`
	TagID  int  `bun:",nullzero" json:"tag_id"`
	Tag    *Tag `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty" tstype:"Tag"`
}
