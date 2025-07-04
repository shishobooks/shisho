package models

import (
	"time"

	"github.com/uptrace/bun"
)

type BookIdentifier struct {
	bun.BaseModel `bun:"table:book_identifiers,alias:bi" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	BookID    int       `bun:",nullzero" json:"book_id"`
	Type      string    `bun:",nullzero" json:"type"` // TODO: make enum
	Value     string    `bun:",nullzero" json:"value"`
}
