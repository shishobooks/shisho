package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Series struct {
	bun.BaseModel `bun:"table:series,alias:s" tstype:"-"`

	ID             int        `bun:",pk,nullzero" json:"id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `bun:",soft_delete" json:"-"`
	LibraryID      int        `bun:",nullzero" json:"library_id"`
	Library        *Library   `bun:"rel:belongs-to" json:"library,omitempty" tstype:"Library"`
	Name           string     `bun:",nullzero" json:"name"`
	NameSource     string     `bun:",nullzero" json:"name_source" tstype:"DataSource"`
	Description    *string    `json:"description,omitempty"`
	CoverImagePath *string    `json:"cover_image_path,omitempty"`
	Books          []*Book    `bun:"rel:has-many" json:"books,omitempty" tstype:"Book[]"`
	BookCount      int        `bun:",scanonly" json:"book_count"`
}
