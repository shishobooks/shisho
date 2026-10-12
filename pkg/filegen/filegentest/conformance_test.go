package filegentest

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSources(t *testing.T) (source, broken, body string) {
	t.Helper()
	dir := t.TempDir()
	body = CopyMagic + strings.Repeat("page ", 20_000)
	source = filepath.Join(dir, "source.bin")
	require.NoError(t, os.WriteFile(source, []byte(body), 0o600))
	broken = filepath.Join(dir, "broken.bin")
	require.NoError(t, os.WriteFile(broken, []byte("not the magic"), 0o600))
	return source, broken, body
}

// The suite passes a generator that streams, with and without a size
// reported up front.
func TestRun_StreamingGenerator(t *testing.T) {
	t.Parallel()
	source, broken, body := writeSources(t)
	for _, unknownSize := range []bool{false, true} {
		Run(t, Case{
			Generator: Copy{UnknownSize: unknownSize},
			Source:    source,
			Book:      &models.Book{Title: "Book"},
			File:      &models.File{FileType: models.FileTypeEPUB},
			Check: func(t *testing.T, output []byte) {
				assert.Equal(t, CopyHeader+body, string(output))
			},
			Broken: broken,
		})
	}
}

// Each check fails for a generator that breaks its rule.
func TestRun_ChecksCatchBrokenGenerators(t *testing.T) {
	t.Parallel()
	source, broken, _ := writeSources(t)
	c := func(gen filegen.StreamGenerator) Case {
		return Case{Generator: gen, Source: source, File: &models.File{}, Broken: broken}
	}

	require.NoError(t, checkSize(c(Copy{})))
	require.NoError(t, checkStartsEarly(c(Copy{})))
	require.NoError(t, checkPrepareFails(c(Copy{})))

	require.Error(t, checkSize(c(misbehaving{sizeOff: 1})), "a wrong size")
	require.Error(t, checkStartsEarly(c(misbehaving{readAllFirst: true})), "building in full before writing")
	require.Error(t, checkPrepareFails(c(misbehaving{acceptAnything: true})), "a broken source accepted by Prepare")
}

// misbehaving wraps Copy and breaks one rule of the suite.
type misbehaving struct {
	sizeOff        int64
	readAllFirst   bool
	acceptAnything bool
}

func (g misbehaving) Prepare(ctx context.Context, src filegen.Source, book *models.Book, file *models.File) (filegen.Prepared, error) {
	if g.acceptAnything {
		return &copyPrepared{src: src}, nil
	}
	p, err := Copy{}.Prepare(ctx, src, book, file)
	if err != nil {
		return nil, err
	}
	return misbehavingPrepared{Prepared: p, g: g, src: src}, nil
}

type misbehavingPrepared struct {
	filegen.Prepared
	g   misbehaving
	src filegen.Source
}

func (p misbehavingPrepared) Size() (int64, bool) {
	size, ok := p.Prepared.Size()
	return size + p.g.sizeOff, ok
}

func (p misbehavingPrepared) Write(ctx context.Context, w io.Writer) error {
	if !p.g.readAllFirst {
		return p.Prepared.Write(ctx, w)
	}
	var buf bytes.Buffer
	if err := p.Prepared.Write(ctx, &buf); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return errors.WithStack(err)
}
