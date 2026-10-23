package filegen

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// ErrNoConversion is returned for a File Type no chain of Conversions
// reaches from the File's own. Such a File Type is not offered, so asking for
// it is the caller's mistake, not a generation failure.
var ErrNoConversion = errors.New("no Conversion reaches this File Type")

// conversionStep runs one Conversion step (models.ConversionStep).
type conversionStep interface {
	// prepare does the step's cheap up-front work, as StreamGenerator's
	// Prepare does, and returns its output ready to write.
	prepare(ctx context.Context, in stepInput) (Prepared, error)
}

// stepInput is what a Conversion step converts.
type stepInput struct {
	// src is the Original File. Only a chain's first step reads it.
	src  Source
	book *models.Book
	file *models.File
	// entries is the output of the step before, or nil for a chain's first
	// step.
	entries []kepub.Entry
}

// entryLister is a Prepared whose output a following step reads as an
// in-memory list of entries instead of written bytes, so a chain streams
// without building any intermediate file (ADR 0010).
type entryLister interface {
	entries() []kepub.Entry
}

// conversionSteps holds the code that runs each step in
// models.ConversionSteps.
var conversionSteps = map[models.ConversionStep]conversionStep{
	{From: models.FileTypeEPUB, To: models.FileTypeKePub}: epubToKepub{converter: kepub.NewConverter()},
	{From: models.FileTypeCBZ, To: models.FileTypeKePub}:  cbzFixedLayout{fileType: models.FileTypeKePub},
	{From: models.FileTypeCBZ, To: models.FileTypeEPUB}:   cbzFixedLayout{fileType: models.FileTypeEPUB},
	{From: models.FileTypeKePub, To: models.FileTypeEPUB}: kepubToEPUB{},
}

// GeneratorFor returns the generator that delivers a File of type fileType as
// type deliveredType: the File Type's own generator when they are the same,
// and otherwise the chain of Conversions between them. It returns
// ErrNoConversion when no chain reaches deliveredType.
func GeneratorFor(fileType, deliveredType string) (Generator, error) {
	if fileType == deliveredType {
		return GetGenerator(fileType)
	}
	chain, ok := models.ConversionChain(fileType, deliveredType)
	if !ok {
		return nil, errors.Wrapf(ErrNoConversion, "%s to %s", fileType, deliveredType)
	}
	gen := &conversionGenerator{to: deliveredType}
	for _, step := range chain {
		code, ok := conversionSteps[step]
		if !ok {
			return nil, errors.Errorf("no code runs the %s to %s Conversion", step.From, step.To)
		}
		gen.steps = append(gen.steps, code)
	}
	return gen, nil
}

// conversionGenerator delivers a File through a chain of Conversion steps.
// A real failure of any step is an error: it never falls back to another
// File Type.
type conversionGenerator struct {
	to    string
	steps []conversionStep
}

// SupportedType returns the File Type delivered.
func (g *conversionGenerator) SupportedType() string {
	return g.to
}

// Generate writes the delivered file to destPath.
func (g *conversionGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	return generateFile(ctx, g, g.to, srcPath, destPath, book, file)
}

// Prepare prepares each step in turn, handing every step after the first the
// entries of the one before. Only the last step's output is written.
func (g *conversionGenerator) Prepare(ctx context.Context, src Source, book *models.Book, file *models.File) (Prepared, error) {
	in := stepInput{src: src, book: book, file: file}
	var prepared Prepared
	for i, step := range g.steps {
		if i > 0 {
			lister, ok := prepared.(entryLister)
			if !ok {
				return nil, NewGenerationError(g.to, nil, "a Conversion step's output cannot feed the next step")
			}
			in.entries = lister.entries()
		}
		var err error
		prepared, err = step.prepare(ctx, in)
		if err != nil {
			return nil, err
		}
	}
	return prepared, nil
}
