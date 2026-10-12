package filegen

import (
	"context"
	"io"

	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// KepubEPUBGenerator generates KePub files from EPUB sources.
type KepubEPUBGenerator struct {
	converter *kepub.Converter
}

// NewKepubEPUBGenerator creates a new KepubEPUBGenerator.
func NewKepubEPUBGenerator() *KepubEPUBGenerator {
	return &KepubEPUBGenerator{
		converter: kepub.NewConverter(),
	}
}

// SupportedType returns the file type this generator handles.
func (g *KepubEPUBGenerator) SupportedType() string {
	return models.FileTypeEPUB
}

// Generate creates a KePub file at destPath from an EPUB source.
func (g *KepubEPUBGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	return generateFile(ctx, g, models.FileTypeEPUB, srcPath, destPath, book, file)
}

// Prepare does the EPUB generator's preparation. Writing converts its
// entries to KePub in a single pass over the source, so no Generated EPUB
// is built first, and the size is not known up front.
func (g *KepubEPUBGenerator) Prepare(ctx context.Context, src Source, book *models.Book, file *models.File) (Prepared, error) {
	plan, err := planEPUB(ctx, src, book, file)
	if err != nil {
		return nil, err
	}
	entries := make([]kepub.Entry, 0, len(plan.entries))
	for _, e := range plan.entries {
		entries = append(entries, kepub.Entry{Name: e.name, File: e.source, Content: e.content})
	}
	return &preparedKepubEPUB{converter: g.converter, entries: entries}, nil
}

// preparedKepubEPUB is a KePub ready to be converted from an EPUB.
type preparedKepubEPUB struct {
	converter *kepub.Converter
	entries   []kepub.Entry
}

func (p *preparedKepubEPUB) Size() (int64, bool) {
	return 0, false
}

func (p *preparedKepubEPUB) Write(ctx context.Context, w io.Writer) error {
	if err := p.converter.WriteEPUB(ctx, w, p.entries); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to convert to KePub format")
	}
	return nil
}
