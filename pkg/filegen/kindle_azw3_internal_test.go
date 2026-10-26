package filegen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Kindle keeps a reading position per identifier, so a file regenerated
// after a metadata edit carries the same one, and another file its own.
func TestEPUBToAZW3_IdentifierStaysWithTheFile(t *testing.T) {
	t.Parallel()
	src := testgen.GenerateEPUB(t, t.TempDir(), "book.epub", testgen.EPUBOptions{Title: "Book"})
	gen, err := GeneratorFor(models.FileTypeEPUB, models.FileTypeAZW3)
	require.NoError(t, err)
	generate := func(title string, fileID int) []byte {
		dest := filepath.Join(t.TempDir(), "out.azw3")
		require.NoError(t, gen.Generate(context.Background(), src, dest, &models.Book{ID: 3, Title: title}, &models.File{ID: fileID, BookID: 3, FileType: models.FileTypeEPUB}))
		data, err := os.ReadFile(dest)
		require.NoError(t, err)
		return data
	}

	first, renamed, other := generate("Before", 7), generate("After", 7), generate("Before", 8)
	id := []byte(kindleFileIdentifier(&models.File{ID: 7}))
	assert.True(t, bytes.Contains(first, id))
	assert.True(t, bytes.Contains(renamed, id), "a regenerated file keeps its identifier")
	assert.False(t, bytes.Contains(other, id))
	assert.True(t, bytes.Contains(other, []byte(kindleFileIdentifier(&models.File{ID: 8}))))
	assert.NotEqual(t, kindleFileIdentifier(&models.File{ID: 7}), kindleFileIdentifier(&models.File{ID: 8}))
}

// A pre-paginated EPUB, such as one made from a CBZ, becomes a fixed-layout
// AZW3 the size of its first page; a reflowable one stays reflowable.
func TestKF8Book_FixedLayout(t *testing.T) {
	t.Parallel()
	entries := func(layout string) []kepub.Entry {
		return []kepub.Entry{
			{Name: "OEBPS/content.opf", Content: []byte(`<package version="3.0" xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Comic</dc:title>` + layout + `</metadata>
<manifest><item id="p1" href="page1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="p1"/></spine></package>`)},
			{Name: "OEBPS/page1.xhtml", Content: []byte(`<html><head><meta name="viewport" content="width=800, height=1200"/></head><body><img src="p.jpg"/></body></html>`)},
		}
	}
	book, err := kf8Book(entries(`<meta property="rendition:layout">pre-paginated</meta>`))
	require.NoError(t, err)
	assert.True(t, book.FixedLayout)
	assert.Equal(t, "800x1200", book.OriginalResolution)

	book, err = kf8Book(entries(""))
	require.NoError(t, err)
	assert.False(t, book.FixedLayout)
	assert.Empty(t, book.OriginalResolution)
}

// Pages of a fixed-layout EPUB can differ in size, such as a landscape
// spread, so the resolution is the largest any page gives, with the
// viewport's attributes in either order; one with no viewport is still
// fixed-layout.
func TestKF8Book_FixedLayoutResolution(t *testing.T) {
	t.Parallel()
	opf := `<package version="3.0" xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Comic</dc:title><meta property="rendition:layout">pre-paginated</meta></metadata>
<manifest><item id="p1" href="p1.xhtml" media-type="application/xhtml+xml"/><item id="p2" href="p2.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="p1"/><itemref idref="p2"/></spine></package>`
	page := func(head string) []byte {
		return []byte(`<html><head>` + head + `</head><body><p>page</p></body></html>`)
	}
	book, err := kf8Book([]kepub.Entry{
		{Name: "content.opf", Content: []byte(opf)},
		{Name: "p1.xhtml", Content: page(`<meta name="viewport" content="width=800, height=1200"/>`)},
		{Name: "p2.xhtml", Content: page(`<meta content="height=900,width=1600" name="viewport"/>`)},
	})
	require.NoError(t, err)
	assert.Equal(t, "1600x1200", book.OriginalResolution)

	book, err = kf8Book([]kepub.Entry{
		{Name: "content.opf", Content: []byte(opf)},
		{Name: "p1.xhtml", Content: page("")},
		{Name: "p2.xhtml", Content: page("")},
	})
	require.NoError(t, err)
	assert.True(t, book.FixedLayout)
	assert.Empty(t, book.OriginalResolution)
}

// A comic converted from CBZ becomes a fixed-layout AZW3, through the EPUB
// the CBZ Conversion writes: its EXTH block holds the fixed-layout record
// (type 122, "true") and the resolution record (type 307).
func TestCBZToAZW3_IsFixedLayout(t *testing.T) {
	t.Parallel()
	src := testgen.GenerateCBZ(t, t.TempDir(), "comic.cbz", testgen.CBZOptions{Title: "Comic"})
	gen, err := GeneratorFor(models.FileTypeCBZ, models.FileTypeAZW3)
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "comic.azw3")
	require.NoError(t, gen.Generate(context.Background(), src, dest, &models.Book{Title: "Comic"}, &models.File{ID: 1, FileType: models.FileTypeCBZ}))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.True(t, bytes.Contains(data, []byte("\x00\x00\x00\x7a\x00\x00\x00\x0ctrue")), "fixed-layout record")
	assert.Regexp(t, "\x00\x00\x01\x33\x00\x00\x00.[0-9]+x[0-9]+", string(data), "resolution record")
}
