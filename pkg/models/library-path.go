package models

import (
	"time"

	"github.com/uptrace/bun"
)

type LibraryPath struct {
	bun.BaseModel `bun:"table:library_paths,alias:lp" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Filepath  string    `bun:",nullzero" json:"filepath"`
}
