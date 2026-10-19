package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/fixedlayout"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manyPages is enough pages that the generator's lookahead, which grows
// with GOMAXPROCS, is a small part of the book.
func manyPages() int {
	return 8*runtime.GOMAXPROCS(0) + 40
}

func TestCBZGenerator_Conformance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := testgen.GenerateCBZ(t, dir, "source.cbz", testgen.CBZOptions{
		Title:        "Original",
		HasComicInfo: true,
		PageCount:    manyPages(),
	})
	broken := filepath.Join(dir, "broken.cbz")
	require.NoError(t, os.WriteFile(broken, []byte("not a zip"), 0o600))

	book := &models.Book{
		Title: "Streamed Comic",
		BookSeries: []*models.BookSeries{
			{SeriesNumber: pointerutil.Float64(2), Series: &models.Series{Name: "Series"}},
		},
	}
	file := &models.File{FileType: models.FileTypeCBZ, CoverPage: pointerutil.Int(1)}

	filegentest.Run(t, filegentest.Case{
		Generator: &filegen.CBZGenerator{},
		Source:    source,
		Book:      book,
		File:      file,
		Check: func(t *testing.T, output []byte) {
			src, err := zip.OpenReader(source)
			require.NoError(t, err)
			defer src.Close()
			out, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
			require.NoError(t, err)

			require.Len(t, out.File, len(src.File))
			require.Equal(t, "ComicInfo.xml", out.File[0].Name)
			var info struct {
				Title  string `xml:"Title"`
				Series string `xml:"Series"`
				Number string `xml:"Number"`
				Pages  struct {
					Page []struct {
						Image string `xml:"Image,attr"`
						Type  string `xml:"Type,attr"`
					} `xml:"Page"`
				} `xml:"Pages"`
			}
			require.NoError(t, xml.Unmarshal(readEntry(t, out.File[0]), &info))
			assert.Equal(t, "Streamed Comic", info.Title)
			assert.Equal(t, "Series", info.Series)
			assert.Equal(t, "2", info.Number)
			require.NotEmpty(t, info.Pages.Page)
			assert.Equal(t, "1", info.Pages.Page[0].Image)
			assert.Equal(t, "FrontCover", info.Pages.Page[0].Type)

			// Every page is processed for e-readers and kept in order.
			for i, srcPage := range src.File[1:] {
				ext := strings.ToLower(filepath.Ext(srcPage.Name))
				want := fixedlayout.ProcessImageForEreader(readEntry(t, srcPage), ext)
				got := out.File[i+1]
				assert.Equal(t, strings.TrimSuffix(srcPage.Name, filepath.Ext(srcPage.Name))+want.Ext, got.Name)
				assert.Equal(t, want.Data, readEntry(t, got), "page %d", i)
			}
		},
		Broken: broken,
	})
}

func readEntry(t *testing.T, f *zip.File) []byte {
	t.Helper()
	r, err := f.Open()
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return data
}

// pageSource records how far into the pages the generator has read.
type pageSource struct {
	filegen.Source
	starts []int64 // data offset of each page, in order
	end    int64   // end of the last page's data
	mu     sync.Mutex
	read   int // pages whose data has been read from
	closed atomic.Bool
	late   atomic.Bool
}

func newPageSource(t *testing.T, path string) *pageSource {
	t.Helper()
	src, err := filegen.OpenSource(path)
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	zr, err := zip.NewReader(src, src.Size())
	require.NoError(t, err)
	s := &pageSource{Source: src}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".png") {
			off, err := f.DataOffset()
			require.NoError(t, err)
			s.starts = append(s.starts, off)
			s.end = off + int64(f.CompressedSize64)
		}
	}
	return s
}

func (s *pageSource) ReadAt(p []byte, off int64) (int, error) {
	if s.closed.Load() {
		s.late.Store(true)
	}
	s.mu.Lock()
	for off < s.end && s.read < len(s.starts) && s.starts[s.read] <= off {
		s.read++
	}
	s.mu.Unlock()
	return s.Source.ReadAt(p, off)
}

func (s *pageSource) pagesRead() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read
}

// pageCounter counts the entries written to the output so far by their
// local file headers.
type pageCounter struct {
	out    bytes.Buffer
	onPage func(written int)
}

func (w *pageCounter) Write(p []byte) (int, error) {
	w.out.Write(p)
	w.onPage(strings.Count(w.out.String(), "PK\x03\x04"))
	return len(p), nil
}

// However many pages a CBZ has, the generator reads only a bounded number of
// pages ahead of what it has written, so it never holds every processed page
// in memory at once.
func TestCBZGenerator_ReadsBoundedPagesAhead(t *testing.T) {
	t.Parallel()
	pages := manyPages()
	path := testgen.GenerateCBZ(t, t.TempDir(), "source.cbz", testgen.CBZOptions{PageCount: pages})
	src := newPageSource(t, path)
	bound := 2*runtime.GOMAXPROCS(0) + 2

	prepared, err := (&filegen.CBZGenerator{}).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	maxAhead := 0
	w := &pageCounter{onPage: func(written int) {
		maxAhead = max(maxAhead, src.pagesRead()-written)
	}}
	require.NoError(t, prepared.Write(context.Background(), w))
	assert.Equal(t, pages, src.pagesRead())
	assert.LessOrEqual(t, maxAhead, bound, "pages read ahead of the writer")
}

// Canceling the context stops generation: Write returns the cancellation
// and no page is read from the source after it returns.
func TestCBZGenerator_CancelStopsWorkers(t *testing.T) {
	t.Parallel()
	path := testgen.GenerateCBZ(t, t.TempDir(), "source.cbz", testgen.CBZOptions{PageCount: manyPages()})
	src := newPageSource(t, path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prepared, err := (&filegen.CBZGenerator{}).Prepare(ctx, src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	w := &pageCounter{onPage: func(int) { cancel() }}
	err = prepared.Write(ctx, w)
	src.closed.Store(true)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, src.pagesRead(), manyPages(), "every page was read despite the cancellation")
	assert.False(t, src.late.Load(), "a page was read after Write returned")
}

// A page that cannot be read fails the build after earlier pages have been
// written, rather than in preparation.
func TestCBZGenerator_CorruptPageFailsWhileWriting(t *testing.T) {
	t.Parallel()
	pages := manyPages()
	path := testgen.GenerateCorruptCBZ(t, t.TempDir(), "corrupt.cbz", pages, pages/2)
	src, err := filegen.OpenSource(path)
	require.NoError(t, err)
	defer src.Close()

	prepared, err := (&filegen.CBZGenerator{}).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	var out bytes.Buffer
	err = prepared.Write(context.Background(), &out)
	require.ErrorIs(t, err, zip.ErrChecksum)
	assert.Positive(t, out.Len(), "nothing was written before the corrupt page")
}

// A CBZ with an entry the zip reader cannot open is rejected in preparation,
// so the download falls back before any byte is sent instead of failing
// partway.
func TestCBZGenerator_UnreadableEntryFailsPreparation(t *testing.T) {
	t.Parallel()
	for name, header := range map[string]zip.FileHeader{
		"unsupported compression": {Name: "001.png", Method: 99},
		"encrypted":               {Name: "001.png", Method: zip.Store, Flags: 0x1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "unreadable.cbz")
			f, err := os.Create(path)
			require.NoError(t, err)
			zw := zip.NewWriter(f)
			h := header
			h.CompressedSize64, h.UncompressedSize64 = 4, 4
			w, err := zw.CreateRaw(&h)
			require.NoError(t, err)
			_, err = w.Write([]byte("data"))
			require.NoError(t, err)
			require.NoError(t, zw.Close())
			require.NoError(t, f.Close())

			src, err := filegen.OpenSource(path)
			require.NoError(t, err)
			defer src.Close()
			_, err = (&filegen.CBZGenerator{}).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
			require.Error(t, err)
		})
	}
}
