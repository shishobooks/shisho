package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Author struct {
	bun.BaseModel `bun:"table:authors,alias:a" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	BookID    int       `bun:",nullzero" json:"book_id"`
	Name      string    `bun:",nullzero" json:"name"`
	SortOrder int       `bun:",nullzero" json:"sort_order"`
}
