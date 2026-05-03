package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Publisher struct {
	bun.BaseModel `bun:"table:publishers,alias:pub" tstype:"-"`

	ID        int               `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	LibraryID int               `bun:",nullzero" json:"library_id"`
	Name      string            `bun:",nullzero" json:"name"`
	Aliases   []*PublisherAlias `bun:"rel:has-many,join:id=publisher_id" json:"aliases" tstype:"PublisherAlias[]"`
	FileCount int               `bun:",scanonly" json:"file_count"`
}
