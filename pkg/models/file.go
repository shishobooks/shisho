package models

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type FileType = typeof FileTypeCBZ | typeof FileTypeEPUB | typeof FileTypeM4B;
	FileTypeCBZ  = "cbz"
	FileTypeEPUB = "epub"
	FileTypeM4B  = "m4b"
)

const (
	//tygo:emit export type FileRole = typeof FileRoleMain | typeof FileRoleSupplement;
	FileRoleMain       = "main"
	FileRoleSupplement = "supplement"
)

type File struct {
	bun.BaseModel `bun:"table:files,alias:f" tstype:"-"`

	ID                       int               `bun:",pk,nullzero" json:"id"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
	LibraryID                int               `bun:",nullzero" json:"library_id"`
	BookID                   int               `bun:",nullzero" json:"book_id"`
	Book                     *Book             `bun:"rel:belongs-to" json:"book" tstype:"Book"`
	Filepath                 string            `bun:",nullzero" json:"filepath"`
	FileType                 string            `bun:",nullzero" json:"file_type" tstype:"FileType"`
	FileRole                 string            `bun:",nullzero,default:'main'" json:"file_role" tstype:"FileRole"`
	FilesizeBytes            int64             `bun:",nullzero" json:"filesize_bytes"`
	CoverImagePath           *string           `json:"cover_image_path"`
	CoverMimeType            *string           `json:"cover_mime_type"`
	CoverSource              *string           `json:"cover_source" tstype:"DataSource"`
	CoverPage                *int              `json:"cover_page"` // 0-indexed page number for CBZ cover, NULL for EPUB/M4B
	PageCount                *int              `json:"page_count"` // Number of pages for CBZ files, NULL for EPUB/M4B
	AudiobookDurationSeconds *float64          `json:"audiobook_duration_seconds"`
	AudiobookBitrateBps      *int              `json:"audiobook_bitrate_bps"`
	Narrators                []*Narrator       `bun:"rel:has-many,join:id=file_id" json:"narrators,omitempty" tstype:"Narrator[]"`
	NarratorSource           *string           `json:"narrator_source" tstype:"DataSource"`
	Identifiers              []*FileIdentifier `bun:"rel:has-many,join:id=file_id" json:"identifiers,omitempty" tstype:"FileIdentifier[]"`
	IdentifierSource         *string           `json:"identifier_source" tstype:"DataSource"`
	URL                      *string           `json:"url"`
	URLSource                *string           `json:"url_source" tstype:"DataSource"`
	ReleaseDate              *time.Time        `json:"release_date"`
	ReleaseDateSource        *string           `json:"release_date_source" tstype:"DataSource"`
	PublisherID              *int              `json:"publisher_id"`
	PublisherSource          *string           `json:"publisher_source" tstype:"DataSource"`
	Publisher                *Publisher        `bun:"rel:belongs-to,join:publisher_id=id" json:"publisher,omitempty" tstype:"Publisher"`
	ImprintID                *int              `json:"imprint_id"`
	ImprintSource            *string           `json:"imprint_source" tstype:"DataSource"`
	Imprint                  *Imprint          `bun:"rel:belongs-to,join:imprint_id=id" json:"imprint,omitempty" tstype:"Imprint"`
}

func (f *File) CoverExtension() string {
	if f.CoverMimeType == nil {
		return ""
	}
	ext := ""
	switch *f.CoverMimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	}
	return ext
}
