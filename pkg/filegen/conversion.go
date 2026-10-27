package filegen

import (
	"archive/zip"
	"bufio"
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
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
	{From: models.FileTypeAZW3, To: models.FileTypeEPUB}:  kindleToEPUB{fileType: models.FileTypeAZW3},
	{From: models.FileTypeMOBI, To: models.FileTypeEPUB}:  kindleToEPUB{fileType: models.FileTypeMOBI},
	{From: models.FileTypeEPUB, To: models.FileTypeAZW3}:  epubToAZW3{},
}

// builtStep is a Conversion step whose output can't stream, because the
// file lists every part's offset before the parts. Its Prepared is a
// fileBuilder.
type builtStep interface {
	conversionStep
	builtInFull()
}

// fileBuilder is the output of a builtStep: it is built in full into a
// file before any of it is sent.
type fileBuilder interface {
	build(ctx context.Context, f *os.File) error
}

// GeneratorFor returns the generator that delivers a File of type fileType as
// type deliveredType: the File Type's own generator when they are the same,
// and otherwise the chain of Conversions between them. It returns
// ErrNoConversion when no chain reaches deliveredType. A chain whose last
// step is a builtStep gets a generator that does not stream.
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
	if _, built := gen.steps[len(gen.steps)-1].(builtStep); built {
		return &builtConversion{chain: gen}, nil
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
	return g.prepare(ctx, src, book, file, nil)
}

// spoolFunc writes a step's output somewhere the next step can read it as
// entries, for a step whose Prepared is not an entryLister.
type spoolFunc func(ctx context.Context, prepared Prepared) ([]kepub.Entry, error)

// prepare is Prepare, with spool, when set, feeding a step the output of one
// that only writes it.
func (g *conversionGenerator) prepare(ctx context.Context, src Source, book *models.Book, file *models.File, spool spoolFunc) (Prepared, error) {
	in := stepInput{src: src, book: book, file: file}
	var prepared Prepared
	for i, step := range g.steps {
		if i > 0 {
			lister, ok := prepared.(entryLister)
			switch {
			case ok:
				in.entries = lister.entries()
			case spool != nil:
				entries, err := spool(ctx, prepared)
				if err != nil {
					return nil, relabelError(err, g.to)
				}
				in.entries = entries
			default:
				return nil, NewGenerationError(g.to, nil, "a Conversion step's output cannot feed the next step")
			}
		}
		var err error
		prepared, err = step.prepare(ctx, in)
		if err != nil {
			// A step before the last names the File Type it makes; the
			// error is about the File Type delivered.
			return nil, relabelError(err, g.to)
		}
	}
	return prepared, nil
}

// builtConversion delivers a File through a chain of Conversions ending in
// a builtStep. It has no Prepare, so the download cache builds the file in
// its own directory before sending it, sends it with its exact size, and
// keeps it for the next download.
type builtConversion struct {
	chain *conversionGenerator
}

// SupportedType returns the File Type delivered.
func (g *builtConversion) SupportedType() string {
	return g.chain.to
}

// Generate builds the delivered file at destPath. A step before the last
// whose output is not a list of entries, such as a CBZ made into an EPUB,
// is written to a file beside destPath and read back from it.
func (g *builtConversion) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	to := g.chain.to
	src, err := OpenSource(srcPath)
	if err != nil {
		return NewGenerationError(to, err, "failed to open source file")
	}
	defer src.Close()

	dir, base := filepath.Dir(destPath), filepath.Base(destPath)
	var spooled []*os.File
	defer func() {
		for _, f := range spooled {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	spool := func(ctx context.Context, prepared Prepared) ([]kepub.Entry, error) {
		f, err := fileutils.CreateTemp(dir, "."+base+".step-*", 0o600)
		if err != nil {
			return nil, NewGenerationError(to, err, "failed to create a file for a Conversion step")
		}
		spooled = append(spooled, f)
		w := bufio.NewWriter(f)
		if err := prepared.Write(ctx, w); err != nil {
			return nil, err
		}
		if err := w.Flush(); err != nil {
			return nil, NewGenerationError(to, err, "failed to write a Conversion step")
		}
		info, err := f.Stat()
		if err != nil {
			return nil, NewGenerationError(to, err, "failed to write a Conversion step")
		}
		zr, err := zip.NewReader(f, info.Size())
		if err != nil {
			return nil, NewGenerationError(to, err, "failed to read a Conversion step's output")
		}
		entries := make([]kepub.Entry, 0, len(zr.File))
		for _, zf := range zr.File {
			entries = append(entries, kepub.Entry{Name: zf.Name, File: zf})
		}
		return entries, nil
	}
	prepared, err := g.chain.prepare(ctx, src, book, file, spool)
	if err != nil {
		return err
	}
	builder, ok := prepared.(fileBuilder)
	if !ok {
		return NewGenerationError(to, nil, "a Conversion step that builds a file returned no builder")
	}

	tmp, err := fileutils.CreateTemp(dir, "."+base+".tmp-*", 0o644)
	if err != nil {
		return NewGenerationError(to, err, "failed to create destination file")
	}
	defer os.Remove(tmp.Name())
	err = builder.build(ctx, tmp)
	if err == nil {
		err = src.Unchanged()
	}
	if closeErr := tmp.Close(); err == nil {
		err = errors.WithStack(closeErr)
	}
	if err != nil {
		var genErr *GenerationError
		if errors.As(err, &genErr) {
			return err
		}
		return NewGenerationError(to, err, "failed to write file")
	}
	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return NewGenerationError(to, err, "failed to write file")
	}
	return nil
}
