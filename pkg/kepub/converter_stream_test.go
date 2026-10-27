package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/ziputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter accepts limit bytes, then fails every write.
type failingWriter struct {
	limit int
}

var errWriterFull = errors.New("writer full")

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		n := w.limit
		w.limit = 0
		return n, errWriterFull
	}
	w.limit -= len(p)
	return len(p), nil
}

// manyChapters is an EPUB with more content documents than the lookahead,
// each large enough to fill the zip writer's buffer.
func manyChapters() []Entry {
	var items strings.Builder
	entries := []Entry{}
	for i := range 3 * lookahead {
		name := fmt.Sprintf("ch%d.xhtml", i)
		fmt.Fprintf(&items, `<item id="c%d" href="%s" media-type="application/xhtml+xml"/>`, i, name)
		entries = append(entries, Entry{Name: name, Content: []byte(`<html><body><p>` + strings.Repeat(fmt.Sprintf("Sentence %d. ", i), 2000) + `</p></body></html>`)})
	}
	opf := Entry{Name: "content.opf", Content: []byte(`<package><manifest>` + items.String() + `</manifest></package>`)}
	return append([]Entry{opf}, entries...)
}

// A writer that fails partway ends the conversion with its error, and the
// conversions running ahead of it stop rather than leak. Not parallel: it
// counts the process's goroutines.
func TestWriteEPUB_WriterFailsPartway(t *testing.T) {
	before := runtime.NumGoroutine()
	err := NewConverter().WriteEPUB(context.Background(), &failingWriter{limit: 20_000}, manyChapters())
	require.ErrorIs(t, err, errWriterFull)
	// Polled here rather than with assert.Eventually, whose condition runs
	// on a goroutine of its own.
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.LessOrEqual(t, runtime.NumGoroutine(), before, "conversions still running after WriteEPUB returned")
}

// The output of a conversion that succeeds lists every entry in order.
func TestWriteEPUB_KeepsEntryOrder(t *testing.T) {
	t.Parallel()
	entries := manyChapters()
	var buf bytes.Buffer
	require.NoError(t, NewConverter().WriteEPUB(context.Background(), &buf, entries))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	want := []string{"mimetype", "kobo.js"}
	for _, e := range entries {
		want = append(want, e.Name)
	}
	assert.Equal(t, want, names)
}

// A conversion that panics fails with an error instead of crashing.
func TestConvertAhead_RecoversPanic(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	results := convertAhead(ctx, []Entry{{Name: "bad.xhtml"}}, func(context.Context, Entry) (*ziputil.Entry, error) {
		panic("malformed")
	})
	_, err := results.next(ctx, 0)
	cancel()
	results.wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad.xhtml")
}

// An entry with Load is read when it is converted, and stored as it is. A
// failing Load ends the conversion with its error.
func TestWriteEPUB_LoadsEntries(t *testing.T) {
	t.Parallel()
	image := []byte("\x89PNG not really")
	loads := 0
	entries := append(manyChapters(), Entry{Name: "image.png", Load: func() ([]byte, error) {
		loads++
		return image, nil
	}})
	var buf bytes.Buffer
	require.NoError(t, NewConverter().WriteEPUB(context.Background(), &buf, entries))
	assert.Equal(t, 1, loads)
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	last := zr.File[len(zr.File)-1]
	assert.Equal(t, "image.png", last.Name)
	assert.Equal(t, zip.Store, last.Method)
	r, err := last.Open()
	require.NoError(t, err)
	defer r.Close()
	var got bytes.Buffer
	_, err = got.ReadFrom(r)
	require.NoError(t, err)
	assert.Equal(t, image, got.Bytes())

	failing := append(manyChapters(), Entry{Name: "image.png", Load: func() ([]byte, error) { return nil, errWriterFull }})
	require.ErrorIs(t, NewConverter().WriteEPUB(context.Background(), &bytes.Buffer{}, failing), errWriterFull)
}
