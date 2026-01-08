package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Download format preference constants.
const (
	DownloadFormatOriginal = "original"
	DownloadFormatKepub    = "kepub"
	DownloadFormatAsk      = "ask"
)

type Library struct {
	bun.BaseModel `bun:"table:libraries,alias:l" tstype:"-"`

	ID                       int            `bun:",pk,nullzero" json:"id"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	Name                     string         `bun:",nullzero" json:"name"`
	OrganizeFileStructure    bool           `json:"organize_file_structure"`
	CoverAspectRatio         string         `bun:",nullzero" json:"cover_aspect_ratio"`
	DownloadFormatPreference string         `bun:",nullzero,default:'original'" json:"download_format_preference"`
	LibraryPaths             []*LibraryPath `bun:"rel:has-many" json:"library_paths,omitempty" tstype:"LibraryPath[]"`
	DeletedAt                *time.Time     `json:"deleted_at,omitempty"`
}
