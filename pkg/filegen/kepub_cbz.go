package filegen

import (
	"context"

	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// KepubCBZGenerator generates KePub files from CBZ sources.
// It converts CBZ comic archives to fixed-layout EPUBs with KePub enhancements.
type KepubCBZGenerator struct {
	converter *kepub.Converter
}

// NewKepubCBZGenerator creates a new KepubCBZGenerator.
func NewKepubCBZGenerator() *KepubCBZGenerator {
	return &KepubCBZGenerator{
		converter: kepub.NewConverter(),
	}
}

// SupportedType returns the file type this generator handles.
func (g *KepubCBZGenerator) SupportedType() string {
	return models.FileTypeCBZ
}

// Generate creates a KePub file at destPath from a CBZ source.
// The CBZ is converted to a fixed-layout EPUB with KePub enhancements.
// Images are copied byte-for-byte without modification (lossless).
func (g *KepubCBZGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, _ *models.File) error {
	// Build metadata from book model
	metadata := buildCBZMetadata(book)

	if err := g.converter.ConvertCBZWithMetadata(ctx, srcPath, destPath, metadata); err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to convert CBZ to KePub format")
	}

	return nil
}

// buildCBZMetadata creates CBZMetadata from a book model.
func buildCBZMetadata(book *models.Book) *kepub.CBZMetadata {
	if book == nil {
		return nil
	}

	metadata := &kepub.CBZMetadata{
		Title:    book.Title,
		Subtitle: book.Subtitle,
	}

	// Add authors with their roles and sort names
	for _, author := range book.Authors {
		if author.Person != nil {
			role := ""
			if author.Role != nil {
				role = *author.Role
			}
			metadata.Authors = append(metadata.Authors, kepub.CBZAuthor{
				Name:     author.Person.Name,
				SortName: author.Person.SortName,
				Role:     role,
			})
		}
	}

	// Add series information
	for _, bs := range book.BookSeries {
		if bs.Series != nil {
			metadata.Series = append(metadata.Series, kepub.CBZSeries{
				Name:   bs.Series.Name,
				Number: bs.SeriesNumber,
			})
		}
	}

	return metadata
}
