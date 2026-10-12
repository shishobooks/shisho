// Package filegentest holds the conformance suite every
// filegen.StreamGenerator must pass, and stand-in generators for tests of
// the download cache and the routes that serve it.
package filegentest

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/require"
)

// Case is a StreamGenerator and the sources it is checked against.
type Case struct {
	Generator filegen.StreamGenerator
	// Source is the path of an Original File the generator accepts. It must
	// be large enough that preparation reads well under half of it, or the
	// starts-early check cannot tell streaming from building in full.
	Source string
	Book   *models.Book
	File   *models.File
	// Check validates the output, typically by reading it back through the
	// format's parser and comparing it with what a download produced before
	// the generator streamed.
	Check func(t *testing.T, output []byte)
	// Broken is the path of a file whose preparation must fail.
	Broken string
}

// Run checks the generator against c:
//
//   - the output passes c.Check
//   - a size reported up front equals the bytes written
//   - the first output byte is written before half the source has been read
//   - preparing a broken source fails, so the failure comes before any byte
//     could be sent
func Run(t *testing.T, c Case) {
	t.Helper()
	require.NotNil(t, c.Generator, "Case.Generator")
	require.NotEmpty(t, c.Source, "Case.Source")
	require.NotNil(t, c.Check, "Case.Check")
	require.NotEmpty(t, c.Broken, "Case.Broken")

	t.Run("output", func(t *testing.T) {
		out, _, _, err := generate(c)
		require.NoError(t, err)
		c.Check(t, out)
	})
	t.Run("reported size equals bytes written", func(t *testing.T) {
		require.NoError(t, checkSize(c))
	})
	t.Run("starts early", func(t *testing.T) {
		require.NoError(t, checkStartsEarly(c))
	})
	t.Run("preparation failure comes first", func(t *testing.T) {
		require.NoError(t, checkPrepareFails(c))
	})
}

func generate(c Case) (out []byte, size int64, known bool, err error) {
	src, err := filegen.OpenSource(c.Source)
	if err != nil {
		return nil, 0, false, err
	}
	defer src.Close()
	prepared, err := c.Generator.Prepare(context.Background(), src, c.Book, c.File)
	if err != nil {
		return nil, 0, false, errors.Wrap(err, "prepare")
	}
	size, known = prepared.Size()
	var buf bytes.Buffer
	if err := prepared.Write(context.Background(), &buf); err != nil {
		return nil, 0, false, errors.Wrap(err, "write")
	}
	return buf.Bytes(), size, known, nil
}

func checkSize(c Case) error {
	out, size, known, err := generate(c)
	if err != nil {
		return err
	}
	if known && size != int64(len(out)) {
		return errors.Errorf("reported size %d, wrote %d bytes", size, len(out))
	}
	return nil
}

func checkStartsEarly(c Case) error {
	src, err := filegen.OpenSource(c.Source)
	if err != nil {
		return err
	}
	defer src.Close()
	counted := &countingSource{Source: src}
	prepared, err := c.Generator.Prepare(context.Background(), counted, c.Book, c.File)
	if err != nil {
		return errors.Wrap(err, "prepare")
	}
	w := &firstWriteWriter{source: counted, readAtFirst: -1}
	if err := prepared.Write(context.Background(), w); err != nil {
		return errors.Wrap(err, "write")
	}
	if w.readAtFirst < 0 {
		return errors.New("the generator wrote nothing")
	}
	if w.readAtFirst >= src.Size()/2 {
		return errors.Errorf("%d of %d source bytes were read before the first output byte", w.readAtFirst, src.Size())
	}
	return nil
}

func checkPrepareFails(c Case) error {
	src, err := filegen.OpenSource(c.Broken)
	if err != nil {
		return err
	}
	defer src.Close()
	if _, err := c.Generator.Prepare(context.Background(), src, c.Book, c.File); err == nil {
		return errors.New("preparing a broken source succeeded, so the failure would only surface after bytes were sent")
	}
	return nil
}

// countingSource counts the bytes read from a Source. Generators may read
// from several goroutines, so the count is atomic.
type countingSource struct {
	filegen.Source
	read atomic.Int64
}

func (s *countingSource) ReadAt(p []byte, off int64) (int, error) {
	n, err := s.Source.ReadAt(p, off)
	s.read.Add(int64(n))
	return n, err
}

// firstWriteWriter discards output and records how much of the source had
// been read when the first output byte arrived.
type firstWriteWriter struct {
	source      *countingSource
	readAtFirst int64
}

func (w *firstWriteWriter) Write(p []byte) (int, error) {
	if len(p) > 0 && w.readAtFirst < 0 {
		w.readAtFirst = w.source.read.Load()
	}
	return len(p), nil
}
