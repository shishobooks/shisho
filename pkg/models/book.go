package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Book struct {
	bun.BaseModel `bun:"table:books,alias:b" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LibraryID      int       `bun:",nullzero" json:"library_id"`
	Library        *Library  `bun:"rel:belongs-to" json:"library" tstype:"Library"`
	Filepath       string    `bun:",nullzero" json:"filepath"`
	Title          string    `bun:",nullzero" json:"title"`
	TitleSource    string    `bun:",nullzero" json:"title_source" tstype:"DataSource"`
	Subtitle       *string   `json:"subtitle"`
	SubtitleSource *string   `json:"subtitle_source" tstype:"DataSource"`
	Authors        []*Author `bun:"rel:has-many" json:"authors,omitempty" tstype:"Author[]"`
	AuthorSource   string    `bun:",nullzero" json:"author_source" tstype:"DataSource"`
	Series         *string   `json:"series,omitempty"`
	SeriesNumber   *float64  `json:"series_number,omitempty"`
	Files          []*File   `bun:"rel:has-many" json:"files" tstype:"File[]"`
}
