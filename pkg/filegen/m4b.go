package filegen

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/mp4"
)

// M4BGenerator generates M4B audiobook files with modified metadata.
type M4BGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *M4BGenerator) SupportedType() string {
	return models.FileTypeM4B
}

// Generate creates a modified M4B at destPath with updated metadata.
func (g *M4BGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypeM4B, err, "context cancelled")
	}

	// Parse source file to preserve existing metadata (description, genre, chapters, etc.)
	srcMeta, err := mp4.ParseFull(srcPath)
	if err != nil {
		return NewGenerationError(models.FileTypeM4B, err, "failed to parse source file")
	}

	// Build new metadata from book/file models
	newMeta := g.buildMetadata(book, file, srcMeta)

	// Handle cover replacement
	if err := g.loadCover(book, file, newMeta); err != nil {
		return NewGenerationError(models.FileTypeM4B, err, "failed to load cover image")
	}

	// Check context cancellation before write
	if err := ctx.Err(); err != nil {
		return NewGenerationError(models.FileTypeM4B, err, "context cancelled")
	}

	// Write to destination using atomic pattern
	if err := mp4.WriteToFile(srcPath, destPath, newMeta); err != nil {
		return NewGenerationError(models.FileTypeM4B, err, "failed to write file")
	}

	return nil
}

// buildMetadata constructs new Metadata from book/file models while preserving source metadata.
func (g *M4BGenerator) buildMetadata(book *models.Book, file *models.File, src *mp4.Metadata) *mp4.Metadata {
	// Use file.Name for title if set, otherwise fall back to book.Title
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}

	meta := &mp4.Metadata{
		// From book/file model
		Title:    title,
		Subtitle: "",

		// Preserve from source (will be overwritten if we have new values)
		Description: src.Description,
		Comment:     src.Comment,
		Year:        src.Year,
		Copyright:   src.Copyright,
		Encoder:     src.Encoder,
		Duration:    src.Duration,
		Bitrate:     src.Bitrate,
		Chapters:    src.Chapters,
		MediaType:   src.MediaType,
		Freeform:    src.Freeform,
		Publisher:   src.Publisher,
		Imprint:     src.Imprint,
		URL:         src.URL,
		ReleaseDate: src.ReleaseDate,

		// Preserve cover from source initially (may be replaced below)
		CoverData:     src.CoverData,
		CoverMimeType: src.CoverMimeType,

		// Preserve unknown atoms for complete tag preservation
		UnknownAtoms: src.UnknownAtoms,
	}

	// Set description from book if available
	if book.Description != nil && *book.Description != "" {
		meta.Description = *book.Description
	}

	// Set publisher from file if available
	if file.Publisher != nil {
		meta.Publisher = file.Publisher.Name
	}

	// Set imprint from file if available
	if file.Imprint != nil {
		meta.Imprint = file.Imprint.Name
	}

	// Set URL from file if available
	if file.URL != nil && *file.URL != "" {
		meta.URL = *file.URL
	}

	// Set release date from file if available
	if file.ReleaseDate != nil {
		meta.ReleaseDate = file.ReleaseDate
	}

	// Build genres from book model, or preserve source genres if none in book
	if len(book.BookGenres) > 0 {
		for _, bg := range book.BookGenres {
			if bg.Genre != nil {
				meta.Genres = append(meta.Genres, bg.Genre.Name)
			}
		}
		// Set Genre field as comma-separated for compatibility
		meta.Genre = strings.Join(meta.Genres, ", ")
	} else {
		// Preserve source genres
		meta.Genre = src.Genre
		meta.Genres = src.Genres
	}

	// Build tags from book model, or preserve source tags if none in book
	if len(book.BookTags) > 0 {
		for _, bt := range book.BookTags {
			if bt.Tag != nil {
				meta.Tags = append(meta.Tags, bt.Tag.Name)
			}
		}
	} else {
		// Preserve source tags
		meta.Tags = src.Tags
	}

	// Set subtitle from book model
	if book.Subtitle != nil && *book.Subtitle != "" {
		meta.Subtitle = *book.Subtitle
	}

	// Authors sorted by SortOrder
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		for _, a := range authors {
			if a.Person != nil {
				// M4B doesn't support author roles, so we just store the name
				meta.Authors = append(meta.Authors, mediafile.ParsedAuthor{
					Name: a.Person.Name,
					Role: "",
				})
			}
		}
	}

	// Narrators sorted by SortOrder (from file, not book)
	if len(file.Narrators) > 0 {
		narrators := make([]*models.Narrator, len(file.Narrators))
		copy(narrators, file.Narrators)
		sort.Slice(narrators, func(i, j int) bool {
			return narrators[i].SortOrder < narrators[j].SortOrder
		})
		for _, n := range narrators {
			if n.Person != nil {
				meta.Narrators = append(meta.Narrators, n.Person.Name)
			}
		}
	}

	// Series from book (first by SortOrder)
	if len(book.BookSeries) > 0 {
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})
		if series[0].Series != nil {
			meta.Series = series[0].Series.Name
			meta.SeriesNumber = series[0].SeriesNumber
		}
	}

	// Convert file identifiers to parsed identifiers for writing
	if len(file.Identifiers) > 0 {
		for _, id := range file.Identifiers {
			meta.Identifiers = append(meta.Identifiers, mediafile.ParsedIdentifier{
				Type:  id.Type,
				Value: id.Value,
			})
		}
	}

	return meta
}

// loadCover reads the cover image from the file system and sets it on the metadata.
func (g *M4BGenerator) loadCover(book *models.Book, file *models.File, meta *mp4.Metadata) error {
	coverPath := resolveCoverPath(book, file)
	if coverPath == "" {
		return nil
	}

	data, err := os.ReadFile(coverPath)
	if err != nil {
		return err
	}

	meta.CoverData = data

	// Determine MIME type
	if file.CoverMimeType != nil && *file.CoverMimeType != "" {
		meta.CoverMimeType = *file.CoverMimeType
	} else {
		// Detect from extension
		ext := filepath.Ext(coverPath)
		switch ext {
		case ".jpg", ".jpeg":
			meta.CoverMimeType = "image/jpeg"
		case ".png":
			meta.CoverMimeType = "image/png"
		default:
			meta.CoverMimeType = "image/jpeg" // default
		}
	}

	return nil
}
