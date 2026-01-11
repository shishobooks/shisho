package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Genre struct {
	bun.BaseModel `bun:"table:genres,alias:g" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Name      string    `bun:",nullzero" json:"name"`
	BookCount int       `bun:",scanonly" json:"book_count"`
}

type BookGenre struct {
	bun.BaseModel `bun:"table:book_genres,alias:bg" tstype:"-"`

	ID      int    `bun:",pk,nullzero" json:"id"`
	BookID  int    `bun:",nullzero" json:"book_id"`
	GenreID int    `bun:",nullzero" json:"genre_id"`
	Genre   *Genre `bun:"rel:belongs-to,join:genre_id=id" json:"genre,omitempty" tstype:"Genre"`
}
