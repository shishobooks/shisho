package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Person struct {
	bun.BaseModel `bun:"table:persons,alias:p" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LibraryID int       `bun:",nullzero" json:"library_id"`
	Name      string    `bun:",nullzero" json:"name"`
	SortName  string    `bun:",nullzero" json:"sort_name"`
}

type Author struct {
	bun.BaseModel `bun:"table:authors,alias:a" tstype:"-"`

	ID        int     `bun:",pk,nullzero" json:"id"`
	BookID    int     `bun:",nullzero" json:"book_id"`
	PersonID  int     `bun:",nullzero" json:"person_id"`
	Person    *Person `bun:"rel:belongs-to,join:person_id=id" json:"person,omitempty" tstype:"Person"`
	SortOrder int     `bun:",nullzero" json:"sort_order"`
}

type Narrator struct {
	bun.BaseModel `bun:"table:narrators,alias:n" tstype:"-"`

	ID        int     `bun:",pk,nullzero" json:"id"`
	FileID    int     `bun:",nullzero" json:"file_id"`
	PersonID  int     `bun:",nullzero" json:"person_id"`
	Person    *Person `bun:"rel:belongs-to,join:person_id=id" json:"person,omitempty" tstype:"Person"`
	SortOrder int     `bun:",nullzero" json:"sort_order"`
}
