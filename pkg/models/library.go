package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Library struct {
	bun.BaseModel `bun:"table:libraries,alias:l" tstype:"-"`

	ID           int            `bun:",pk,nullzero" json:"id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Name         string         `bun:",nullzero" json:"name"`
	LibraryPaths []*LibraryPath `bun:"rel:has-many" json:"library_paths,omitempty" tstype:"LibraryPath[]"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}
