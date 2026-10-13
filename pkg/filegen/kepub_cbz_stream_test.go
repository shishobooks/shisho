package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/pointerutil"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/cbz"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/filegen/filegentest"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newKepubCBZGenerator(t *testing.T) filegen.StreamGenerator {
	t.Helper()
	gen, err := filegen.GetKepubGenerator(models.FileTypeCBZ)
	require.NoError(t, err)
	sg, ok := gen.(filegen.StreamGenerator)
	require.True(t, ok, "the KePub-from-CBZ generator does not stream")
	return sg
}

type kepubManifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type kepubOPF struct {
	Title    string              `xml:"metadata>title"`
	Manifest []kepubManifestItem `xml:"manifest>item"`
	Spine    []struct {
		IDRef string `xml:"idref,attr"`
	} `xml:"spine>itemref"`
}

var viewportRE = regexp.MustCompile(`<meta name="viewport" content="width=(\d+), height=(\d+)"/>`)

func TestKepubCBZGenerator_Conformance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pages := manyPages()
	source := testgen.GenerateCBZ(t, dir, "source.cbz", testgen.CBZOptions{
		Title:        "Original",
		HasComicInfo: true,
		PageCount:    pages,
	})
	broken := filepath.Join(dir, "broken.cbz")
	require.NoError(t, os.WriteFile(broken, []byte("not a zip"), 0o600))

	book := &models.Book{Title: "Streamed Comic"}
	file := &models.File{
		FileType:  models.FileTypeCBZ,
		CoverPage: pointerutil.Int(1),
		Chapters: []*models.Chapter{
			{Title: "Chapter One", StartPage: pointerutil.Int(0)},
			{Title: "Chapter Two", StartPage: pointerutil.Int(5)},
		},
	}

	filegentest.Run(t, filegentest.Case{
		Generator: newKepubCBZGenerator(t),
		Source:    source,
		Book:      book,
		File:      file,
		Check: func(t *testing.T, output []byte) {
			src, err := zip.OpenReader(source)
			require.NoError(t, err)
			defer src.Close()
			srcPages := cbz.PageImages(&src.Reader)
			require.Len(t, srcPages, pages)

			out, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
			require.NoError(t, err)
			entries := map[string]*zip.File{}
			index := map[string]int{}
			for i, f := range out.File {
				entries[f.Name] = f
				index[f.Name] = i
			}

			require.Equal(t, "mimetype", out.File[0].Name)
			assert.Equal(t, zip.Store, out.File[0].Method)
			assert.Equal(t, "application/epub+zip", string(readEntry(t, out.File[0])))

			// The navigation documents list every page, so they come after
			// the last page.
			lastPage := index[fmt.Sprintf("OEBPS/page%04d.xhtml", pages)]
			require.Positive(t, lastPage)
			for _, name := range []string{"OEBPS/content.opf", "OEBPS/toc.ncx", "OEBPS/nav.xhtml"} {
				require.Contains(t, index, name)
				assert.Greater(t, index[name], lastPage, "%s comes after the pages", name)
			}

			var opf kepubOPF
			require.NoError(t, xml.Unmarshal(readEntry(t, entries["OEBPS/content.opf"]), &opf))
			assert.Equal(t, "Streamed Comic", opf.Title)
			images := map[string]kepubManifestItem{}
			for _, item := range opf.Manifest {
				if strings.HasPrefix(item.Href, "images/") {
					images[item.ID] = item
				}
			}
			require.Len(t, images, pages)
			require.Len(t, opf.Spine, pages)

			ncx := string(readEntry(t, entries["OEBPS/toc.ncx"]))
			assert.Contains(t, ncx, "Chapter Two")
			assert.Contains(t, ncx, `<content src="page0006.xhtml"/>`)
			nav := string(readEntry(t, entries["OEBPS/nav.xhtml"]))
			assert.Contains(t, nav, `<a href="page0006.xhtml">Chapter Two</a>`)

			for i, srcPage := range srcPages {
				want := kepub.ProcessImageForEreader(readEntry(t, srcPage), strings.ToLower(filepath.Ext(srcPage.Name)))
				item, ok := images[fmt.Sprintf("img%04d", i+1)]
				require.True(t, ok, "page %d is in the manifest", i+1)
				imgName := "OEBPS/" + item.Href
				assert.Equal(t, fmt.Sprintf("images/page%04d%s", i+1, want.Ext), item.Href)
				assert.Equal(t, want.MediaType, item.MediaType)
				require.Contains(t, entries, imgName, "the manifest names a written image")
				got := readEntry(t, entries[imgName])
				assert.Equal(t, want.Data, got, "page %d is processed as before", i+1)
				cfg, _, err := image.DecodeConfig(bytes.NewReader(got))
				require.NoError(t, err)

				xhtmlName := fmt.Sprintf("OEBPS/page%04d.xhtml", i+1)
				xhtml := string(readEntry(t, entries[xhtmlName]))
				assert.Contains(t, xhtml, `src="images/`+path.Base(item.Href)+`"`)
				m := viewportRE.FindStringSubmatch(xhtml)
				require.NotNil(t, m, "page %d has a viewport", i+1)
				assert.Equal(t, []string{strconv.Itoa(cfg.Width), strconv.Itoa(cfg.Height)}, m[1:], "page %d's dimensions are the image's", i+1)
				assert.Less(t, index[xhtmlName], index[imgName], "page %d's XHTML comes before its image", i+1)
				if i > 0 {
					assert.Greater(t, index[xhtmlName], index[fmt.Sprintf("OEBPS/page%04d.xhtml", i)], "pages are in order")
				}
			}
			assert.Equal(t, "cover-image", images["img0002"].Properties)
		},
		Broken: broken,
	})
}

// However many pages a CBZ has, the converter reads only a bounded number of
// pages ahead of what it has written, so it never holds every processed page
// in memory at once.
func TestKepubCBZGenerator_ReadsBoundedPagesAhead(t *testing.T) {
	t.Parallel()
	pages := manyPages()
	path := testgen.GenerateCBZ(t, t.TempDir(), "source.cbz", testgen.CBZOptions{PageCount: pages})
	src := newPageSource(t, path)
	bound := 2*runtime.GOMAXPROCS(0) + 2

	prepared, err := newKepubCBZGenerator(t).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	maxAhead := 0
	w := &pageCounter{onPage: func(entries int) {
		// Each page is two entries, its XHTML and its image, after the
		// mimetype, container, and stylesheet.
		written := max(entries-3, 0) / 2
		maxAhead = max(maxAhead, src.pagesRead()-written)
	}}
	require.NoError(t, prepared.Write(context.Background(), w))
	assert.Equal(t, pages, src.pagesRead())
	assert.LessOrEqual(t, maxAhead, bound, "pages read ahead of the writer")
}

// Canceling the context stops the conversion: Write returns the
// cancellation and no page is read from the source after it returns.
func TestKepubCBZGenerator_CancelStopsWorkers(t *testing.T) {
	t.Parallel()
	path := testgen.GenerateCBZ(t, t.TempDir(), "source.cbz", testgen.CBZOptions{PageCount: manyPages()})
	src := newPageSource(t, path)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prepared, err := newKepubCBZGenerator(t).Prepare(ctx, src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	w := &pageCounter{onPage: func(entries int) {
		// Cancel once the first page's XHTML follows the mimetype,
		// container, and stylesheet.
		if entries > 3 {
			cancel()
		}
	}}
	err = prepared.Write(ctx, w)
	src.closed.Store(true)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, src.pagesRead(), manyPages(), "every page was read despite the cancellation")
	assert.False(t, src.late.Load(), "a page was read after Write returned")
}

// A page that cannot be read fails the conversion after earlier pages have
// been written, rather than in preparation.
func TestKepubCBZGenerator_CorruptPageFailsWhileWriting(t *testing.T) {
	t.Parallel()
	pages := manyPages()
	path := testgen.GenerateCorruptCBZ(t, t.TempDir(), "corrupt.cbz", pages, pages/2)
	src, err := filegen.OpenSource(path)
	require.NoError(t, err)
	defer src.Close()

	prepared, err := newKepubCBZGenerator(t).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
	require.NoError(t, err)
	var out bytes.Buffer
	err = prepared.Write(context.Background(), &out)
	require.ErrorIs(t, err, zip.ErrChecksum)
	assert.Contains(t, out.String(), "OEBPS/images/page0001", "nothing was written before the corrupt page")
}

// A CBZ the converter cannot use is rejected in preparation, so the download
// falls back before any byte is sent instead of failing partway.
func TestKepubCBZGenerator_PrepareRejects(t *testing.T) {
	t.Parallel()
	for name, header := range map[string]*zip.FileHeader{
		"unsupported compression": {Name: "001.png", Method: 99},
		"encrypted":               {Name: "001.png", Method: zip.Store, Flags: 0x1},
		"no pages":                {Name: "notes.txt", Method: zip.Store},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "unusable.cbz")
			f, err := os.Create(path)
			require.NoError(t, err)
			zw := zip.NewWriter(f)
			header.CompressedSize64, header.UncompressedSize64 = 4, 4
			w, err := zw.CreateRaw(header)
			require.NoError(t, err)
			_, err = w.Write([]byte("data"))
			require.NoError(t, err)
			require.NoError(t, zw.Close())
			require.NoError(t, f.Close())

			src, err := filegen.OpenSource(path)
			require.NoError(t, err)
			defer src.Close()
			_, err = newKepubCBZGenerator(t).Prepare(context.Background(), src, &models.Book{Title: "Book"}, &models.File{FileType: models.FileTypeCBZ})
			require.Error(t, err)
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("disk on fire")
}

// A KePub generator's errors name the KePub being generated, not the source
// format, and keep the cause, which the logs and the remembered failure
// show only through the message.
func TestKepubGenerators_ErrorsNameKepub(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.bin")
	require.NoError(t, os.WriteFile(broken, []byte("not a zip"), 0o600))
	for name, tc := range map[string]struct {
		fileType string
		source   string
	}{
		"cbz":  {models.FileTypeCBZ, testgen.GenerateCBZ(t, dir, "source.cbz", testgen.CBZOptions{})},
		"epub": {models.FileTypeEPUB, testgen.GenerateEPUB(t, dir, "source.epub", testgen.EPUBOptions{Title: "Book"})},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gen, err := filegen.GetKepubGenerator(tc.fileType)
			require.NoError(t, err)
			sg, ok := gen.(filegen.StreamGenerator)
			require.True(t, ok)
			book := &models.Book{Title: "Book"}
			file := &models.File{FileType: tc.fileType}

			src, err := filegen.OpenSource(broken)
			require.NoError(t, err)
			defer src.Close()
			_, err = sg.Prepare(context.Background(), src, book, file)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "failed to generate kepub file: "), err.Error())

			src, err = filegen.OpenSource(tc.source)
			require.NoError(t, err)
			defer src.Close()
			prepared, err := sg.Prepare(context.Background(), src, book, file)
			require.NoError(t, err)
			err = prepared.Write(context.Background(), failingWriter{})
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "failed to generate kepub file: "), err.Error())
			assert.Contains(t, err.Error(), "disk on fire")
		})
	}
}
