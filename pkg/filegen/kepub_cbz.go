package filegen

import (
	"context"
	"sort"

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
func (g *KepubCBZGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// Build metadata from book model
	metadata := buildCBZMetadata(book, file)

	if err := g.converter.ConvertCBZWithMetadata(ctx, srcPath, destPath, metadata); err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to convert CBZ to KePub format")
	}

	return nil
}

// buildCBZMetadata creates CBZMetadata from a book and file model.
func buildCBZMetadata(book *models.Book, file *models.File) *kepub.CBZMetadata {
	if book == nil {
		return nil
	}

	metadata := &kepub.CBZMetadata{
		Title:    book.Title,
		Subtitle: book.Subtitle,
	}

	// Set Name from file if available (takes precedence over Title in the converter)
	if file != nil && file.Name != nil {
		metadata.Name = file.Name
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

	// Add genres
	for _, bg := range book.BookGenres {
		if bg.Genre != nil {
			metadata.Genres = append(metadata.Genres, bg.Genre.Name)
		}
	}

	// Add tags
	for _, bt := range book.BookTags {
		if bt.Tag != nil {
			metadata.Tags = append(metadata.Tags, bt.Tag.Name)
		}
	}

	// Add chapters (top-level only, no children)
	if file != nil && len(file.Chapters) > 0 {
		metadata.Chapters = convertModelChaptersToCBZ(file.Chapters)
	}

	// Set cover page if available
	if file != nil && file.CoverPage != nil {
		metadata.CoverPage = file.CoverPage
	}

	return metadata
}

// convertModelChaptersToCBZ converts model chapters to CBZ chapters.
// Only top-level chapters are included (children are flattened/ignored).
// Chapters are sorted by SortOrder.
func convertModelChaptersToCBZ(chapters []*models.Chapter) []kepub.CBZChapter {
	// Copy to avoid modifying the original slice
	sorted := make([]*models.Chapter, len(chapters))
	copy(sorted, chapters)

	// Sort by SortOrder
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	result := make([]kepub.CBZChapter, 0, len(sorted))
	for _, ch := range sorted {
		startPage := 0
		if ch.StartPage != nil {
			startPage = *ch.StartPage
		}
		result = append(result, kepub.CBZChapter{
			Title:     ch.Title,
			StartPage: startPage,
		})
	}
	return result
}
