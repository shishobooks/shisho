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

type File struct {
	bun.BaseModel `bun:"table:files,alias:f" tstype:"-"`

	ID                int         `bun:",pk,nullzero" json:"id"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	LibraryID         int         `bun:",nullzero" json:"library_id"`
	BookID            int         `bun:",nullzero" json:"book_id"`
	Book              *Book       `bun:"rel:belongs-to" json:"book" tstype:"Book"`
	Filepath          string      `bun:",nullzero" json:"filepath"`
	FileType          string      `bun:",nullzero" json:"file_type" tstype:"FileType"`
	FilesizeBytes     int64       `bun:",nullzero" json:"filesize_bytes"`
	CoverMimeType     *string     `json:"cover_mime_type"`
	CoverSource       *string     `json:"cover_source" tstype:"DataSource"`
	AudiobookDuration *float64    `json:"audiobook_duration"`
	AudiobookBitrate  *float64    `json:"audiobook_bitrate"`
	Narrators         []*Narrator `bun:"rel:has-many" json:"narrators,omitempty" tstype:"Narrator[]"`
	NarratorSource    *string     `json:"narrator_source" tstype:"DataSource"`
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
