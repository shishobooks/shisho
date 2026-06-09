package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Cover aspect ratio preference constants. Determine which file's cover a
// hybrid (e.g. EPUB + M4B) book serves.
const (
	//tygo:emit export type CoverAspectRatio = typeof CoverAspectRatioBook | typeof CoverAspectRatioAudiobook | typeof CoverAspectRatioBookFallbackAudiobook | typeof CoverAspectRatioAudiobookFallbackBook;
	CoverAspectRatioBook                  = "book"
	CoverAspectRatioAudiobook             = "audiobook"
	CoverAspectRatioBookFallbackAudiobook = "book_fallback_audiobook"
	CoverAspectRatioAudiobookFallbackBook = "audiobook_fallback_book"
)

// Download format preference constants.
const (
	//tygo:emit export type DownloadFormat = typeof DownloadFormatOriginal | typeof DownloadFormatKepub | typeof DownloadFormatAsk;
	DownloadFormatOriginal = "original"
	DownloadFormatKepub    = "kepub"
	DownloadFormatAsk      = "ask"
)

type Library struct {
	bun.BaseModel `bun:"table:libraries,alias:l" tstype:"-"`

	ID                       int            `bun:",pk,nullzero" json:"id"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
	Name                     string         `bun:",nullzero" json:"name"`
	OrganizeFileStructure    bool           `json:"organize_file_structure"`
	CoverAspectRatio         string         `bun:",nullzero" json:"cover_aspect_ratio" tstype:"CoverAspectRatio"`
	DownloadFormatPreference string         `bun:",nullzero,default:'original'" json:"download_format_preference" tstype:"DownloadFormat"`
	LibraryPaths             []*LibraryPath `bun:"rel:has-many" json:"library_paths,omitempty" tstype:"LibraryPath[]"`
}
