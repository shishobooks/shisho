package filegentest

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
)

// CopyMagic starts every source the Copy generator accepts.
const CopyMagic = "SRC:"

// CopyHeader starts every output of the Copy generator.
const CopyHeader = "generated:"

// Copy is a stand-in StreamGenerator whose output is CopyHeader followed by
// the source. Preparation reads only the source's first bytes and fails
// unless they are CopyMagic.
type Copy struct {
	// UnknownSize makes the generator report no size up front.
	UnknownSize bool
}

// Prepare implements filegen.StreamGenerator.
func (g Copy) Prepare(_ context.Context, src filegen.Source, _ *models.Book, _ *models.File) (filegen.Prepared, error) {
	magic := make([]byte, len(CopyMagic))
	if _, err := src.ReadAt(magic, 0); err != nil || string(magic) != CopyMagic {
		return nil, errors.New("source does not start with CopyMagic")
	}
	return &copyPrepared{src: src, unknownSize: g.UnknownSize}, nil
}

type copyPrepared struct {
	src         filegen.Source
	unknownSize bool
}

func (p *copyPrepared) Size() (int64, bool) {
	if p.unknownSize {
		return 0, false
	}
	return int64(len(CopyHeader)) + p.src.Size(), true
}

func (p *copyPrepared) Write(ctx context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, CopyHeader); err != nil {
		return errors.WithStack(err)
	}
	buf := make([]byte, 4096)
	r := io.NewSectionReader(p.src, 0, p.src.Size())
	for {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return errors.WithStack(werr)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
}

// Pausing is a stand-in StreamGenerator for tests of the download cache and
// the routes that serve it. Its output is First followed by Rest, whatever
// the source. Write writes First, then waits for Release (or for its
// context to end) before writing Rest, so a test can observe a download
// while generation is still running.
type Pausing struct {
	First, Rest []byte
	// UnknownSize makes the generator report no size up front.
	UnknownSize bool
	// PrepareErr, when set, is returned from every Prepare.
	PrepareErr error

	started     chan struct{}
	startOnce   sync.Once
	release     chan struct{}
	releaseOnce sync.Once
	prepares    atomic.Int32
	writes      atomic.Int32
}

// NewPausing returns a Pausing generator whose output is first then rest.
func NewPausing(first, rest string) *Pausing {
	return &Pausing{
		First:   []byte(first),
		Rest:    []byte(rest),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

// Output is the complete output.
func (g *Pausing) Output() []byte {
	return append(bytes.Clone(g.First), g.Rest...)
}

// Started is closed once a Write has written First.
func (g *Pausing) Started() <-chan struct{} {
	return g.started
}

// Release lets every Write, current and future, finish.
func (g *Pausing) Release() {
	g.releaseOnce.Do(func() { close(g.release) })
}

// Prepares counts the calls to Prepare.
func (g *Pausing) Prepares() int {
	return int(g.prepares.Load())
}

// Writes counts the calls to Write.
func (g *Pausing) Writes() int {
	return int(g.writes.Load())
}

// Prepare implements filegen.StreamGenerator.
func (g *Pausing) Prepare(_ context.Context, _ filegen.Source, _ *models.Book, _ *models.File) (filegen.Prepared, error) {
	g.prepares.Add(1)
	if g.PrepareErr != nil {
		return nil, g.PrepareErr
	}
	return pausingPrepared{g}, nil
}

type pausingPrepared struct{ g *Pausing }

func (p pausingPrepared) Size() (int64, bool) {
	if p.g.UnknownSize {
		return 0, false
	}
	return int64(len(p.g.First) + len(p.g.Rest)), true
}

func (p pausingPrepared) Write(ctx context.Context, w io.Writer) error {
	g := p.g
	g.writes.Add(1)
	if _, err := w.Write(g.First); err != nil {
		return errors.WithStack(err)
	}
	g.startOnce.Do(func() { close(g.started) })
	select {
	case <-g.release:
	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	}
	_, err := w.Write(g.Rest)
	return errors.WithStack(err)
}
