package models

import (
	"time"

	"github.com/uptrace/bun"
)

// FileIdentifier type constants.
const (
	//tygo:emit export type IdentifierType = typeof IdentifierTypeISBN10 | typeof IdentifierTypeISBN13 | typeof IdentifierTypeASIN | typeof IdentifierTypeUUID | typeof IdentifierTypeGoodreads | typeof IdentifierTypeGoogle | typeof IdentifierTypeOther | (string & {});
	IdentifierTypeISBN10    = "isbn_10"
	IdentifierTypeISBN13    = "isbn_13"
	IdentifierTypeASIN      = "asin"
	IdentifierTypeUUID      = "uuid"
	IdentifierTypeGoodreads = "goodreads"
	IdentifierTypeGoogle    = "google"
	IdentifierTypeOther     = "other"
)

type FileIdentifier struct {
	bun.BaseModel `bun:"table:file_identifiers,alias:fi" tstype:"-"`

	ID        int       `bun:",pk,nullzero" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	FileID    int       `bun:",nullzero" json:"file_id"`
	Type      string    `bun:",nullzero" json:"type" tstype:"IdentifierType"`
	Value     string    `bun:",nullzero" json:"value"`
	Source    string    `bun:",nullzero" json:"source" tstype:"DataSource"`
}
