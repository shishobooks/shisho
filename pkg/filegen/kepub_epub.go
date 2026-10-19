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
		plan, err := planEPUB(ctx, in.src, in.book, in.file)
		if err != nil {
			return nil, relabelError(err, models.FileTypeKePub)
		}
		entries = make([]kepub.Entry, 0, len(plan.entries))
		for _, e := range plan.entries {
			entries = append(entries, kepub.Entry{Name: e.name, File: e.source, Content: e.content})
		}
	}
	return &preparedKepubEPUB{converter: s.converter, entries: entries}, nil
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
