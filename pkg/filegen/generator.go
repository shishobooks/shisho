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
// Error prints only Message, so a message that should reach the logs and a
// remembered failure carries its cause's text.
type GenerationError struct {
	// FileType is the File Type being delivered.
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

// relabelError relabels a GenerationError from code a Conversion step shares
// with its source File Type's generator, so it names the File Type
// delivered.
func relabelError(err error, fileType string) error {
	var genErr *GenerationError
	if errors.As(err, &genErr) {
		return NewGenerationError(fileType, genErr.Err, genErr.Message)
	}
	return err
}

// ErrNotImplemented is returned for a file type with no generator, such as a
// format only a plugin parses. The device and share download routes serve
// the original file for it; any other generation failure is a server fault.
var ErrNotImplemented = errors.New("no generator for this file type")

// ErrCannotDecrypt is wrapped by the GenerationError for a source whose
// encryption Shisho cannot open, such as a PDF that needs a password. The
// web UI routes answer it with a message instead of a server error.
var ErrCannotDecrypt = errors.New("file is encrypted in a way Shisho cannot open")

// GetGenerator returns the appropriate generator for a file type.
func GetGenerator(fileType string) (Generator, error) {
	switch fileType {
	case models.FileTypeEPUB:
		return &EPUBGenerator{}, nil
	case models.FileTypeM4B:
		return &M4BGenerator{}, nil
	case models.FileTypeCBZ:
		return &CBZGenerator{}, nil
	case models.FileTypePDF:
		return &PDFGenerator{}, nil
	case models.FileTypeMOBI, models.FileTypeAZW3:
		return &MOBIGenerator{fileType: fileType}, nil
	default:
		return nil, errors.Wrapf(ErrNotImplemented, "unsupported file type: %s", fileType)
	}
}
