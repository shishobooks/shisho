package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Narrator struct {
	bun.BaseModel `bun:"table:narrators,alias:n" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",nullzero" json:"file_id"`
	Name      string    `bun:",nullzero" json:"name"`
	SortOrder int       `bun:",nullzero" json:"sort_order"`
}
