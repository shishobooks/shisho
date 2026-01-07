package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Person struct {
	bun.BaseModel `bun:"table:persons,alias:p" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LibraryID      int       `bun:",nullzero" json:"library_id"`
	Name           string    `bun:",nullzero" json:"name"`
	SortName       string    `bun:",notnull" json:"sort_name"`
	SortNameSource string    `bun:",notnull" json:"sort_name_source" tstype:"DataSource"`
}

// Author role constants for CBZ ComicInfo.xml creator types.
const (
	AuthorRoleWriter      = "writer"
	AuthorRolePenciller   = "penciller"
	AuthorRoleInker       = "inker"
	AuthorRoleColorist    = "colorist"
	AuthorRoleLetterer    = "letterer"
	AuthorRoleCoverArtist = "cover_artist"
	AuthorRoleEditor      = "editor"
	AuthorRoleTranslator  = "translator"
)

type Author struct {
	bun.BaseModel `bun:"table:authors,alias:a" tstype:"-"`

	ID        int     `bun:",pk,nullzero" json:"id"`
	BookID    int     `bun:",nullzero" json:"book_id"`
	PersonID  int     `bun:",nullzero" json:"person_id"`
	Person    *Person `bun:"rel:belongs-to,join:person_id=id" json:"person,omitempty" tstype:"Person"`
	SortOrder int     `bun:",nullzero" json:"sort_order"`
	Role      *string `json:"role"` // CBZ creator role: writer, penciller, inker, etc. NULL for generic author
}

type Narrator struct {
	bun.BaseModel `bun:"table:narrators,alias:n" tstype:"-"`

	ID        int     `bun:",pk,nullzero" json:"id"`
	FileID    int     `bun:",nullzero" json:"file_id"`
	PersonID  int     `bun:",nullzero" json:"person_id"`
	Person    *Person `bun:"rel:belongs-to,join:person_id=id" json:"person,omitempty" tstype:"Person"`
	SortOrder int     `bun:",nullzero" json:"sort_order"`
}
