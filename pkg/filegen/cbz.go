package filegen

import (
	"context"

	"github.com/shishobooks/shisho/pkg/models"
)

// CBZGenerator generates CBZ comic book files with modified metadata.
type CBZGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *CBZGenerator) SupportedType() string {
	return models.FileTypeCBZ
}

// Generate creates a modified CBZ at destPath with updated metadata.
// Currently not implemented.
func (g *CBZGenerator) Generate(_ context.Context, _, _ string, _ *models.Book, _ *models.File) error {
	return NewGenerationError(models.FileTypeCBZ, ErrNotImplemented, "CBZ file generation is not yet implemented")
}
