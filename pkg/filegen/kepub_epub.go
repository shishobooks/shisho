package filegen

import (
	"context"
	"io"

	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// epubToKepub is the Conversion step from EPUB to KePub. It converts the
// entries of a Generated EPUB in a single pass, so no Generated EPUB is
// written first, and the size is not known up front.
type epubToKepub struct {
	converter *kepub.Converter
}

// prepare does the EPUB generator's preparation when it is a chain's first
// step, and converts the entries of the step before it otherwise.
func (s epubToKepub) prepare(ctx context.Context, in stepInput) (Prepared, error) {
	entries := in.entries
	if entries == nil {
		var err error
		entries, err = plannedEntries(ctx, in, models.FileTypeKePub)
		if err != nil {
			return nil, err
		}
	}
	return &preparedKepubEPUB{converter: s.converter, entries: entries}, nil
}

// plannedEntries runs the EPUB generator's preparation on a chain's Original
// File and returns the Generated EPUB's entries, with errors labeled as
// fileType's.
func plannedEntries(ctx context.Context, in stepInput, fileType string) ([]kepub.Entry, error) {
	plan, err := planEPUB(ctx, in.src, in.book, in.file)
	if err != nil {
		return nil, relabelError(err, fileType)
	}
	entries := make([]kepub.Entry, 0, len(plan.entries))
	for _, e := range plan.entries {
		entries = append(entries, kepub.Entry{Name: e.name, File: e.source, Content: e.content})
	}
	return entries, nil
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
		return NewGenerationError(models.FileTypeKePub, err, "failed to convert EPUB: "+err.Error())
	}
	return nil
}

// kepubToEPUB is the Conversion step from KePub to EPUB. It generates the
// KePub as the EPUB generator does, so the book's metadata is applied, and
// strips Kobo's markup from the entries as it writes them, so no Generated
// KePub is written first, and the size is not known up front.
type kepubToEPUB struct{}

// prepare does the EPUB generator's preparation when it is a chain's first
// step, and then reads what stripping needs to know about the whole book.
func (kepubToEPUB) prepare(ctx context.Context, in stepInput) (Prepared, error) {
	entries := in.entries
	if entries == nil {
		var err error
		entries, err = plannedEntries(ctx, in, models.FileTypeEPUB)
		if err != nil {
			return nil, err
		}
	}
	strip, err := kepub.PrepareStrip(ctx, entries)
	if err != nil {
		return nil, NewGenerationError(models.FileTypeEPUB, err, "failed to convert KePub: "+err.Error())
	}
	return &preparedStrippedEPUB{strip: strip}, nil
}

// preparedStrippedEPUB is an EPUB ready to be stripped from a KePub.
type preparedStrippedEPUB struct {
	strip *kepub.Strip
}

func (p *preparedStrippedEPUB) Size() (int64, bool) {
	return 0, false
}

func (p *preparedStrippedEPUB) Write(ctx context.Context, w io.Writer) error {
	if err := p.strip.Write(ctx, w); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to convert KePub: "+err.Error())
	}
	return nil
}
