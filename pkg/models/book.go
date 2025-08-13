package models

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

type Book struct {
	bun.BaseModel `bun:"table:books,alias:b" tstype:"-"`

	ID             int       `bun:",pk,nullzero" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LibraryID      int       `bun:",nullzero" json:"library_id"`
	Library        *Library  `bun:"rel:belongs-to" json:"library" tstype:"Library"`
	Filepath       string    `bun:",nullzero" json:"filepath"`
	Title          string    `bun:",nullzero" json:"title"`
	TitleSource    string    `bun:",nullzero" json:"title_source" tstype:"DataSource"`
	Subtitle       *string   `json:"subtitle"`
	SubtitleSource *string   `json:"subtitle_source" tstype:"DataSource"`
	Authors        []*Author `bun:"rel:has-many" json:"authors,omitempty" tstype:"Author[]"`
	AuthorSource   string    `bun:",nullzero" json:"author_source" tstype:"DataSource"`
	Series         *string   `json:"series,omitempty"`
	SeriesNumber   *float64  `json:"series_number,omitempty"`
	Files          []*File   `bun:"rel:has-many" json:"files" tstype:"File[]"`
	CoverImagePath *string   `json:"cover_image_path"`
}

func (b *Book) ResolveCoverImage() string {
	if b.CoverImagePath != nil && *b.CoverImagePath != "" {
		coverPath := filepath.Join(b.Filepath, *b.CoverImagePath)
		if _, err := os.Stat(coverPath); err == nil {
			return *b.CoverImagePath
		}
		b.CoverImagePath = nil
	}

	extensions := []string{".jpg", ".jpeg", ".png", ".webp"}

	for _, ext := range extensions {
		coverPath := filepath.Join(b.Filepath, "cover"+ext)
		if _, err := os.Stat(coverPath); err == nil {
			filename := "cover" + ext
			b.CoverImagePath = &filename
			return filename
		}
	}

	for _, ext := range extensions {
		coverPath := filepath.Join(b.Filepath, "audiobook_cover"+ext)
		if _, err := os.Stat(coverPath); err == nil {
			filename := "audiobook_cover" + ext
			b.CoverImagePath = &filename
			return filename
		}
	}

	return ""
}

func (b *Book) CoverMimeType() string {
	coverImage := b.ResolveCoverImage()
	if coverImage == "" {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(coverImage))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}
