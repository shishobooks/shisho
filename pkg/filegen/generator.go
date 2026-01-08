package filegen

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// Generator defines the interface for file generation.
type Generator interface {
	// Generate creates a modified file at destPath from the source file.
	Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error

	// SupportedType returns the file type this generator handles.
	SupportedType() string
}

// GenerationError represents an error that occurred during file generation.
type GenerationError struct {
	FileType string
	Err      error
	Message  string
}

func (e *GenerationError) Error() string {
	return fmt.Sprintf("failed to generate %s file: %s", e.FileType, e.Message)
}

func (e *GenerationError) Unwrap() error {
	return e.Err
}

// NewGenerationError creates a new GenerationError.
func NewGenerationError(fileType string, err error, message string) *GenerationError {
	return &GenerationError{
		FileType: fileType,
		Err:      err,
		Message:  message,
	}
}

// ErrNotImplemented is returned when a file type generator is not yet implemented.
var ErrNotImplemented = errors.New("file type generation not yet implemented")

// ErrKepubNotSupported is returned when KePub conversion is not supported for a file type.
var ErrKepubNotSupported = errors.New("KePub conversion not supported for this file type")

// GetGenerator returns the appropriate generator for a file type.
func GetGenerator(fileType string) (Generator, error) {
	switch fileType {
	case models.FileTypeEPUB:
		return &EPUBGenerator{}, nil
	case models.FileTypeM4B:
		return &M4BGenerator{}, nil
	case models.FileTypeCBZ:
		return &CBZGenerator{}, nil
	default:
		return nil, errors.Errorf("unsupported file type: %s", fileType)
	}
}

// GetKepubGenerator returns the appropriate KePub generator for a file type.
// Returns ErrKepubNotSupported for file types that don't support KePub conversion (M4B).
func GetKepubGenerator(fileType string) (Generator, error) {
	switch fileType {
	case models.FileTypeEPUB:
		return NewKepubEPUBGenerator(), nil
	case models.FileTypeCBZ:
		return NewKepubCBZGenerator(), nil
	case models.FileTypeM4B:
		return nil, ErrKepubNotSupported
	default:
		return nil, errors.Errorf("unsupported file type: %s", fileType)
	}
}

// SupportsKepub returns true if the file type can be converted to KePub format.
func SupportsKepub(fileType string) bool {
	switch fileType {
	case models.FileTypeEPUB, models.FileTypeCBZ:
		return true
	default:
		return false
	}
}
