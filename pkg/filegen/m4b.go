package filegen

import (
	"context"

	"github.com/shishobooks/shisho/pkg/models"
)

// M4BGenerator generates M4B audiobook files with modified metadata.
type M4BGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *M4BGenerator) SupportedType() string {
	return models.FileTypeM4B
}

// Generate creates a modified M4B at destPath with updated metadata.
// Currently not implemented.
func (g *M4BGenerator) Generate(_ context.Context, _, _ string, _ *models.Book, _ *models.File) error {
	return NewGenerationError(models.FileTypeM4B, ErrNotImplemented, "M4B file generation is not yet implemented")
}
