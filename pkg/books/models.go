package books

import (
	"time"

	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/uptrace/bun"
)

const (
	//tygo:emit export type DataSource = typeof DataSourceManual | typeof DataSourceEPUBMetadata | typeof DataSourceM4BMetadata | typeof DataSourceFilepath;
	DataSourceManual       = "manual"
	DataSourceEPUBMetadata = "epub_metadata"
	DataSourceM4BMetadata  = "m4b_metadata"
	DataSourceFilepath     = "filepath"
)

// Lower priority means that we respect it more than higher priority.
const (
	DataSourceManualPriority = iota
	DataSourceEPUBMetadataPriority
	DataSourceM4BMetadataPriority
	DataSourceFilepathPriority
)

var DataSourcePriority = map[string]int{
	DataSourceManual:       DataSourceManualPriority,
	DataSourceEPUBMetadata: DataSourceEPUBMetadataPriority,
	DataSourceM4BMetadata:  DataSourceM4BMetadataPriority,
	DataSourceFilepath:     DataSourceFilepathPriority,
}

const (
	//tygo:emit export type FileType = typeof FileTypeCBZ | typeof FileTypeEPUB | typeof FileTypeM4B;
	FileTypeCBZ  = "cbz"
	FileTypeEPUB = "epub"
	FileTypeM4B  = "m4b"
)

type Book struct {
	bun.BaseModel `bun:"table:books,alias:b" tstype:"-"`

	ID             string             `bun:",pk,nullzero" json:"id"`
	LibraryID      string             `bun:",nullzero" json:"library_id"`
	Library        *libraries.Library `bun:"rel:belongs-to" json:"library" tstype:"Library"`
	Filepath       string             `bun:",nullzero" json:"filepath"`
	Title          string             `bun:",nullzero" json:"title"`
	TitleSource    string             `bun:",nullzero" json:"title_source" tstype:"DataSource"`
	Subtitle       *string            `json:"subtitle"`
	SubtitleSource *string            `json:"subtitle_source" tstype:"DataSource"`
	Authors        []*Author          `bun:"rel:has-many" json:"authors,omitempty" tstype:"Author[]"`
	AuthorSource   string             `bun:",nullzero" json:"author_source" tstype:"DataSource"`
	Series         *string            `json:"series,omitempty"`
	SeriesNumber   *float64           `json:"series_number,omitempty"`
	Files          []*File            `bun:"rel:has-many" json:"files" tstype:"File[]"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

type File struct {
	bun.BaseModel `bun:"table:files,alias:f" tstype:"-"`

	ID                string      `bun:",pk,nullzero" json:"id"`
	LibraryID         string      `bun:",nullzero" json:"library_id"`
	BookID            string      `bun:",nullzero" json:"book_id"`
	Book              *Book       `bun:"rel:belongs-to" json:"book" tstype:"Book"`
	Filepath          string      `bun:",nullzero" json:"filepath"`
	FileType          string      `bun:",nullzero" json:"file_type" tstype:"FileType"`
	FilesizeBytes     int64       `bun:",nullzero" json:"filesize_bytes"`
	CoverMimeType     *string     `json:"cover_mime_type"`
	AudiobookDuration *float64    `json:"audiobook_duration"`
	AudiobookBitrate  *float64    `json:"audiobook_bitrate"`
	Narrators         []*Narrator `bun:"rel:has-many" json:"narrators,omitempty" tstype:"Narrator[]"`
	NarratorSource    *string     `json:"narrator_source" tstype:"DataSource"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
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

type Author struct {
	bun.BaseModel `bun:"table:authors,alias:a" tstype:"-"`

	ID        string    `bun:",pk,nullzero" json:"id"`
	BookID    string    `bun:",nullzero" json:"book_id"`
	Name      string    `bun:",nullzero" json:"name"`
	Sequence  int       `bun:",nullzero" json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Narrator struct {
	bun.BaseModel `bun:"table:narrators,alias:n" tstype:"-"`

	ID        string    `bun:",pk,nullzero" json:"id"`
	FileID    string    `bun:",nullzero" json:"file_id"`
	Name      string    `bun:",nullzero" json:"name"`
	Sequence  int       `bun:",nullzero" json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
