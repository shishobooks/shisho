package libraries

import (
	"time"

	"github.com/uptrace/bun"
)

type Library struct {
	bun.BaseModel `bun:"table:libraries,alias:l" tstype:"-"`

	ID           string         `bun:",pk,nullzero" json:"id"`
	Name         string         `bun:",nullzero" json:"name"`
	LibraryPaths []*LibraryPath `bun:"rel:has-many" json:"library_paths,omitempty" tstype:"LibraryPath[]"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type LibraryPath struct {
	bun.BaseModel `bun:"table:library_paths,alias:lp" tstype:"-"`

	ID        string    `bun:",pk,nullzero" json:"id"`
	LibraryID string    `bun:",nullzero" json:"library_id"`
	Filepath  string    `bun:",nullzero" json:"filepath"`
	CreatedAt time.Time `json:"created_at"`
}
