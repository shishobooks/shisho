package models

import (
	"time"

	"github.com/uptrace/bun"
)

type GenreAlias struct {
	bun.BaseModel `bun:"table:genre_aliases,alias:ga" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	GenreID   int       `bun:",nullzero" json:"genre_id"`
	Name      string    `bun:",nullzero" json:"name"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
}

type TagAlias struct {
	bun.BaseModel `bun:"table:tag_aliases,alias:ta" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	TagID     int       `bun:",nullzero" json:"tag_id"`
	Name      string    `bun:",nullzero" json:"name"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
}

type SeriesAlias struct {
	bun.BaseModel `bun:"table:series_aliases,alias:sa" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	SeriesID  int       `bun:",nullzero" json:"series_id"`
	Name      string    `bun:",nullzero" json:"name"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
}

type PersonAlias struct {
	bun.BaseModel `bun:"table:person_aliases,alias:pa" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	PersonID  int       `bun:",nullzero" json:"person_id"`
	Name      string    `bun:",nullzero" json:"name"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
}

type PublisherAlias struct {
	bun.BaseModel `bun:"table:publisher_aliases,alias:puba" tstype:"-"`

	ID          int       `bun:",pk,nullzero" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	PublisherID int       `bun:",nullzero" json:"publisher_id"`
	Name        string    `bun:",nullzero" json:"name"`
	LibraryID   int       `bun:",nullzero" json:"library_id"`
}

type ImprintAlias struct {
	bun.BaseModel `bun:"table:imprint_aliases,alias:impa" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ImprintID int       `bun:",nullzero" json:"imprint_id"`
	Name      string    `bun:",nullzero" json:"name"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
}
