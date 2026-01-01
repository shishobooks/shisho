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
	SeriesID       *int      `json:"series_id,omitempty"`
	Series         *Series   `bun:"rel:belongs-to" json:"series,omitempty" tstype:"Series"`
	SeriesNumber   *float64  `json:"series_number,omitempty"`
	Files          []*File   `bun:"rel:has-many" json:"files" tstype:"File[]"`
	CoverImagePath *string   `json:"cover_image_path"`
}

func (b *Book) ResolveCoverImage() string {
	// Determine if this is a root-level book by checking if b.Filepath is a file
	isRootLevelBook := false
	if info, err := os.Stat(b.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}

	// Determine the directory where covers should be located
	var coverDir string
	if isRootLevelBook {
		// For root-level books, covers are in the same directory as the file
		coverDir = filepath.Dir(b.Filepath)
	} else {
		// For directory-based books, covers are in the book directory
		coverDir = b.Filepath
	}

	if b.CoverImagePath != nil && *b.CoverImagePath != "" {
		coverPath := filepath.Join(coverDir, *b.CoverImagePath)
		if _, err := os.Stat(coverPath); err == nil {
			return *b.CoverImagePath
		}
		b.CoverImagePath = nil
	}

	extensions := []string{".jpg", ".jpeg", ".png", ".webp"}

	// First, try standard canonical covers
	for _, ext := range extensions {
		coverPath := filepath.Join(coverDir, "cover"+ext)
		if _, err := os.Stat(coverPath); err == nil {
			filename := "cover" + ext
			b.CoverImagePath = &filename
			return filename
		}
	}

	for _, ext := range extensions {
		coverPath := filepath.Join(coverDir, "audiobook_cover"+ext)
		if _, err := os.Stat(coverPath); err == nil {
			filename := "audiobook_cover" + ext
			b.CoverImagePath = &filename
			return filename
		}
	}

	// If no standard covers found, look for individual covers
	if isRootLevelBook {
		// For root-level books, look for covers specific to this book file
		// Cover naming: {filename}.cover.{ext} (e.g., mybook.epub.cover.jpg)
		bookFilename := filepath.Base(b.Filepath)
		for _, ext := range extensions {
			coverFilename := bookFilename + ".cover" + ext
			coverPath := filepath.Join(coverDir, coverFilename)
			if _, err := os.Stat(coverPath); err == nil {
				b.CoverImagePath = &coverFilename
				return coverFilename
			}
		}
	} else {
		// For directory-based books, look for any file matching *.cover.ext pattern
		files, err := os.ReadDir(coverDir)
		if err != nil {
			return ""
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}

			filename := file.Name()
			ext := filepath.Ext(filename)

			// Check if it matches the *.cover.ext pattern
			if strings.HasSuffix(strings.TrimSuffix(filename, ext), ".cover") {
				// Verify it's a supported image extension
				for _, supportedExt := range extensions {
					if strings.ToLower(ext) == supportedExt {
						b.CoverImagePath = &filename
						return filename
					}
				}
			}
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
