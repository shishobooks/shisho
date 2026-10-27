package filegen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/mobi"
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

// The guide is the OPF's when it has one, and an EPUB 3's landmarks
// otherwise, with their types turned into the guide's.
func TestKF8Book_Guide(t *testing.T) {
	t.Parallel()
	entries := func(guide string) []kepub.Entry {
		return []kepub.Entry{
			{Name: "OEBPS/content.opf", Content: []byte(`<package version="3.0" xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Guided</dc:title></metadata>
<manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="c1" href="text/c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine>` + guide + `</package>`)},
			{Name: "OEBPS/nav.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>
<nav epub:type="toc"><ol><li><a href="text/c1.xhtml">One</a></li></ol></nav>
<nav epub:type="landmarks"><ol><li><a epub:type="bodymatter" href="text/c1.xhtml#start">Begin Reading</a></li><li><a epub:type="toc" href="nav.xhtml">Contents</a></li><li><a epub:type="unknown-type" href="text/c1.xhtml">Odd</a></li></ol></nav>
</body></html>`)},
			{Name: "OEBPS/text/c1.xhtml", Content: []byte(`<html><body><p id="start">One</p></body></html>`)},
		}
	}
	book, err := kf8Book(entries(`<guide><reference type="text" title="Start" href="text/c1.xhtml#start"/></guide>`))
	require.NoError(t, err)
	assert.Equal(t, []mobi.GuideRef{{Type: "text", Title: "Start", Href: "OEBPS/text/c1.xhtml#start"}}, book.Guide)

	book, err = kf8Book(entries(""))
	require.NoError(t, err)
	assert.Equal(t, []mobi.GuideRef{
		{Type: "text", Title: "Begin Reading", Href: "OEBPS/text/c1.xhtml#start"},
		{Type: "toc", Title: "Contents", Href: "OEBPS/nav.xhtml"},
	}, book.Guide)
}

// A font obfuscated with Adobe's algorithm is read back in the clear, its
// key made from the book's UUID; one the encryption.xml doesn't name is
// read as it is.
func TestKF8Book_DeobfuscatesAdobeFonts(t *testing.T) {
	t.Parallel()
	font := append([]byte("OTTO"), bytes.Repeat([]byte{0x5A}, 2000)...)
	key := []byte{0x7b, 0x1c, 0x6f, 0x4e, 0x2d, 0x1a, 0x4c, 0x3b, 0x9e, 0x8f, 0x0a, 0x1b, 0x2c, 0x3d, 0x4e, 0x5f}
	hidden := bytes.Clone(font)
	for i := range 1024 {
		hidden[i] ^= key[i%len(key)]
	}
	book, err := kf8Book([]kepub.Entry{
		{Name: "META-INF/encryption.xml", Content: []byte(`<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#"><enc:EncryptedData><enc:EncryptionMethod Algorithm="http://ns.adobe.com/pdf/enc#RC"/><enc:CipherData><enc:CipherReference URI="OEBPS/fonts/hidden%20face.otf"/></enc:CipherData></enc:EncryptedData></encryption>`)},
		{Name: "OEBPS/content.opf", Content: []byte(`<package version="2.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="uid"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Fonts</dc:title><dc:identifier id="uid">urn:uuid:7B1C6F4E-2D1A-4C3B-9E8F-0A1B2C3D4E5F</dc:identifier></metadata>
<manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/><item id="f1" href="fonts/hidden%20face.otf" media-type="application/x-font-otf"/><item id="f2" href="fonts/plain.otf" media-type="application/octet-stream"/></manifest><spine><itemref idref="c1"/></spine></package>`)},
		{Name: "OEBPS/c1.xhtml", Content: []byte(`<html><body><p>One</p></body></html>`)},
		{Name: "OEBPS/fonts/hidden face.otf", Content: hidden},
		{Name: "OEBPS/fonts/plain.otf", Content: font},
	})
	require.NoError(t, err)
	require.Len(t, book.Fonts, 2)
	assert.Equal(t, "OEBPS/fonts/hidden face.otf", book.Fonts[0].Name)
	assert.Equal(t, font, book.Fonts[0].Content)
	assert.Equal(t, font, book.Fonts[1].Content)
}
