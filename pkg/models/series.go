package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Series struct {
	bun.BaseModel `bun:"table:series,alias:s" tstype:"-"`

	ID                 int           `bun:",pk,nullzero" json:"id"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	DeletedAt          *time.Time    `bun:",soft_delete" json:"-"`
	LibraryID          int           `bun:",nullzero" json:"library_id"`
	Library            *Library      `bun:"rel:belongs-to" json:"library,omitempty" tstype:"Library"`
	Name               string        `bun:",nullzero" json:"name"`
	NameSource         string        `bun:",nullzero" json:"name_source" tstype:"DataSource"`
	SortName           string        `bun:",notnull" json:"sort_name"`
	SortNameSource     string        `bun:",notnull" json:"sort_name_source" tstype:"DataSource"`
	Description        *string       `json:"description,omitempty"`
	CoverImageFilename *string       `json:"cover_image_filename,omitempty"`
	BookSeries         []*BookSeries `bun:"rel:has-many" json:"book_series,omitempty" tstype:"BookSeries[]"`
	BookCount          int           `bun:",scanonly" json:"book_count"`
}

type BookSeries struct {
	bun.BaseModel `bun:"table:book_series,alias:bs" tstype:"-"`

	ID           int      `bun:",pk,nullzero" json:"id"`
	BookID       int      `bun:",nullzero" json:"book_id"`
	SeriesID     int      `bun:",nullzero" json:"series_id"`
	Series       *Series  `bun:"rel:belongs-to,join:series_id=id" json:"series,omitempty" tstype:"Series"`
	SeriesNumber *float64 `json:"series_number,omitempty"`
	SortOrder    int      `bun:",nullzero" json:"sort_order"`
}
