package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Imprint struct {
	bun.BaseModel `bun:"table:imprints,alias:imp" tstype:"-"`

	ID        int             `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	LibraryID int             `bun:",nullzero" json:"library_id"`
	Name      string          `bun:",nullzero" json:"name"`
	Aliases   []*ImprintAlias `bun:"rel:has-many,join:id=imprint_id" json:"aliases" tstype:"ImprintAlias[]"`
	FileCount int             `bun:",scanonly" json:"file_count"`
}
