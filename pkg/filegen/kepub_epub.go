package filegen

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// KepubEPUBGenerator generates KePub files from EPUB sources.
type KepubEPUBGenerator struct {
	epubGenerator *EPUBGenerator
	converter     *kepub.Converter
}

// NewKepubEPUBGenerator creates a new KepubEPUBGenerator.
func NewKepubEPUBGenerator() *KepubEPUBGenerator {
	return &KepubEPUBGenerator{
		epubGenerator: &EPUBGenerator{},
		converter:     kepub.NewConverter(),
	}
}

// SupportedType returns the file type this generator handles.
func (g *KepubEPUBGenerator) SupportedType() string {
	return models.FileTypeEPUB
}

// Generate creates a KePub file at destPath from an EPUB source.
// It first generates an EPUB with updated metadata, then converts it to KePub.
func (g *KepubEPUBGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// First, generate an EPUB with updated metadata to a temporary location
	tempEpubPath := destPath + ".epub.tmp"
	defer os.Remove(tempEpubPath)

	if err := g.epubGenerator.Generate(ctx, srcPath, tempEpubPath, book, file); err != nil {
		return errors.Wrap(err, "failed to generate EPUB with metadata")
	}

	// Then convert the EPUB to KePub format
	if err := g.converter.ConvertEPUB(ctx, tempEpubPath, destPath); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to convert to KePub format")
	}

	return nil
}
