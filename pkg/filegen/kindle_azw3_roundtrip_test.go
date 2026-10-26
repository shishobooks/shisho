package filegen_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // the IDPF font obfuscation key is a SHA-1
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/filegen"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const roundTripID = "urn:uuid:7b1c6f4e-2d1a-4c3b-9e8f-0a1b2c3d4e5f"

// roundTripFont returns font data starting with magic, with bytes after it
// that don't compress.
func roundTripFont(magic string, seed int) []byte {
	font := []byte(magic)
	for i := range 3000 {
		font = append(font, byte((i+seed)*7919>>3))
	}
	return font
}

// idpfObfuscate XORs a font's first 1040 bytes with the SHA-1 of the
// book's unique identifier, as the IDPF font obfuscation does.
func idpfObfuscate(font []byte, id string) []byte {
	key := sha1.Sum([]byte(strings.Join(strings.Fields(id), ""))) //nolint:gosec // the algorithm's key
	out := bytes.Clone(font)
	for i := range min(1040, len(out)) {
		out[i] ^= key[i%len(key)]
	}
	return out
}

// roundTripChapterOne is long enough to span several fragments, with a
// footnote reference near its start and a link target near its end.
func roundTripChapterOne() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>One</title><link rel="stylesheet" href="../styles/main.css"/></head>
<body><h1 id="one">Chapter One</h1>
<p>A claim that needs a note.<a id="ref1" epub:type="noteref" href="notes.xhtml#note1">1</a> Jump <a href="#end">to the end</a> or on to <a href="two.xhtml#two">chapter two</a>.</p>
`)
	for i := range 250 {
		fmt.Fprintf(&b, "<p class=\"body\">Paragraph %d of chapter one, with café and naïve words.</p>\n", i)
	}
	b.WriteString(`<p id="end">The end of chapter one.</p></body></html>`)
	return b.String()
}

// writeRoundTripEPUB writes an EPUB 3 with internal links, a footnote and
// its back link, a plain font and an obfuscated one, a stylesheet that
// imports another and is longer than a text record, a guide, and landmarks.
// It returns the two fonts.
func writeRoundTripEPUB(t *testing.T, path string) (plain, hidden []byte) {
	t.Helper()
	plain, hidden = roundTripFont("\x00\x01\x00\x00", 1), roundTripFont("OTTO", 2)
	var css strings.Builder
	css.WriteString(`@import url("base.css");
@font-face { font-family: "Plain"; src: url(../fonts/plain.ttf); }
@font-face { font-family: "Hidden"; src: url("../fonts/hidden.otf") format("opentype"); }
h1 { font-family: "Hidden", serif; }
p.body { font-family: "Plain", serif; }
`)
	for i := range 300 {
		fmt.Fprintf(&css, ".rule%d::before { content: \"§%d\"; margin-left: %dem; }\n", i, i, i%5)
	}
	entries := []struct {
		name string
		data []byte
	}{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{"META-INF/encryption.xml", []byte(`<?xml version="1.0"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container" xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
<enc:EncryptedData><enc:EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/><enc:CipherData><enc:CipherReference URI="OEBPS/fonts/hidden.otf"/></enc:CipherData></enc:EncryptedData>
</encryption>`)},
		{"OEBPS/content.opf", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>Title In File</dc:title>
    <dc:creator>Author In File</dc:creator>
    <dc:identifier id="bookid">` + roundTripID + `</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>Round Trip Press</dc:publisher>
    <dc:description>A book that goes there and back.</dc:description>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="main" href="styles/main.css" media-type="text/css"/>
    <item id="base" href="styles/base.css" media-type="text/css"/>
    <item id="plain" href="fonts/plain.ttf" media-type="font/ttf"/>
    <item id="hidden" href="fonts/hidden.otf" media-type="application/vnd.ms-opentype"/>
    <item id="c1" href="text/one.xhtml" media-type="application/xhtml+xml"/>
    <item id="c2" href="text/two.xhtml" media-type="application/xhtml+xml"/>
    <item id="notes" href="text/notes.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="c1"/><itemref idref="c2"/><itemref idref="notes" linear="no"/></spine>
  <guide><reference type="text" title="Start" href="text/one.xhtml#one"/></guide>
</package>`)},
		{"OEBPS/nav.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head>
<body><nav epub:type="toc"><ol>
<li><a href="text/one.xhtml#one">Chapter One</a><ol><li><a href="text/one.xhtml#end">The End</a></li></ol></li>
<li><a href="text/two.xhtml#two">Chapter Two</a></li>
<li><a href="text/notes.xhtml">Notes</a></li>
</ol></nav></body></html>`)},
		{"OEBPS/styles/main.css", []byte(css.String())},
		{"OEBPS/styles/base.css", []byte(`p { margin: 0; text-indent: 1.2em; }`)},
		{"OEBPS/fonts/plain.ttf", plain},
		{"OEBPS/fonts/hidden.otf", idpfObfuscate(hidden, roundTripID)},
		{"OEBPS/text/one.xhtml", []byte(roundTripChapterOne())},
		{"OEBPS/text/two.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Two</title><link rel="stylesheet" href="../styles/main.css"/></head>
<body><section id="two"><h1>Chapter Two</h1><p class="rule3">Back to <a href="one.xhtml">chapter one</a>.</p></section></body></html>`)},
		{"OEBPS/text/notes.xhtml", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Notes</title></head>
<body><aside id="note1" epub:type="footnote"><p><a href="one.xhtml#ref1">1.</a> The note itself.</p></aside></body></html>`)},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		method := zip.Deflate
		if e.name == "mimetype" {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		require.NoError(t, err)
		_, err = w.Write(e.data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return plain, hidden
}

// An EPUB converted to AZW3 and back keeps its text, its chapter
// boundaries, its links, its fonts and stylesheets, and its metadata.
func TestEPUBToAZW3_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "book.epub")
	plain, hidden := writeRoundTripEPUB(t, src)
	book := &models.Book{Title: "Round Trip", Authors: []*models.Author{{Person: &models.Person{Name: "Trip Author"}}}}
	azw3 := generateAZW3(t, models.FileTypeEPUB, src, book, &models.File{ID: 3, FileType: models.FileTypeEPUB})

	gen, err := filegen.GeneratorFor(models.FileTypeAZW3, models.FileTypeEPUB)
	require.NoError(t, err)
	back := filepath.Join(dir, "back.epub")
	require.NoError(t, gen.Generate(context.Background(), azw3, back, book, &models.File{ID: 4, FileType: models.FileTypeAZW3, Filepath: azw3}))

	parsed, err := epub.Parse(back)
	require.NoError(t, err)
	assert.Equal(t, "Round Trip", parsed.Title)
	require.Len(t, parsed.Authors, 1)
	assert.Equal(t, "Trip Author", parsed.Authors[0].Name)
	assert.Equal(t, "Round Trip Press", parsed.Publisher)
	assert.Equal(t, "A book that goes there and back.", parsed.Description)
	require.NotNil(t, parsed.Language)
	assert.Equal(t, "en", *parsed.Language)
	require.Len(t, parsed.Chapters, 3)
	assert.Equal(t, "Chapter One", parsed.Chapters[0].Title)
	require.Len(t, parsed.Chapters[0].Children, 1)
	assert.Equal(t, "The End", parsed.Chapters[0].Children[0].Title)
	assert.Equal(t, "Notes", parsed.Chapters[2].Title)

	data, err := os.ReadFile(back)
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, f := range zr.File {
		content := readZipEntry(t, f)
		assert.NotRegexp(t, `["(]kindle:`, string(content), f.Name)
		if strings.HasSuffix(f.Name, ".xhtml") || strings.HasSuffix(f.Name, ".opf") || strings.HasSuffix(f.Name, ".ncx") {
			assertWellFormedXML(t, f.Name, content)
		}
	}
	assertEPUBLinksResolve(t, zr)

	// Each document comes back whole, as its own document.
	srcData, err := os.ReadFile(src)
	require.NoError(t, err)
	srcZip, err := zip.NewReader(bytes.NewReader(srcData), int64(len(srcData)))
	require.NoError(t, err)
	for i, name := range []string{"one", "two", "notes"} {
		want := docText(t, readZipEntry(t, findZipEntry(t, srcZip, "OEBPS/text/"+name+".xhtml")))
		got := docText(t, readZipEntry(t, findZipEntry(t, zr, fmt.Sprintf("OEBPS/part%04d.xhtml", i))))
		assert.Equal(t, want, got, name)
	}

	// Every link between documents still names its target.
	one := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0000.xhtml")))
	two := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0001.xhtml")))
	notes := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/part0002.xhtml")))
	assert.Contains(t, one, `epub:type="noteref" href="part0002.xhtml#note1">1</a>`)
	assert.Contains(t, one, `<a href="part0000.xhtml#end">to the end</a>`)
	assert.Contains(t, one, `<a href="part0001.xhtml#two">chapter two</a>`)
	assert.Regexp(t, `<a href="part0000\.xhtml#[^"]+">chapter one</a>`, two)
	assert.Contains(t, notes, `<a href="part0000.xhtml#ref1">1.</a>`)

	// The fonts come back as they were, the obfuscated one in the clear,
	// and the stylesheets still name them and each other.
	var fonts [][]byte
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".ttf") || strings.HasSuffix(f.Name, ".otf") {
			fonts = append(fonts, readZipEntry(t, f))
		}
	}
	assert.ElementsMatch(t, [][]byte{plain, hidden}, fonts)
	main := string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/flow0001.css")))
	assert.Regexp(t, `@import url\("flow0002\.css"\);`, main)
	assert.Regexp(t, `src: url\(font\d{4}\.ttf\);`, main)
	assert.Regexp(t, `src: url\("font\d{4}\.otf"\) format\("opentype"\);`, main)
	assert.Contains(t, main, `.rule299::before { content: "§299"; margin-left: 4em; }`)
	assert.Equal(t, `p { margin: 0; text-indent: 1.2em; }`, string(readZipEntry(t, findZipEntry(t, zr, "OEBPS/flow0002.css"))))
}

// Shisho's identifiers can replace the EPUB's unique identifier, which keys
// its obfuscated fonts, so the source's identifier still unlocks them.
func TestEPUBToAZW3_FontsSurviveANewIdentifier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "book.epub")
	plain, hidden := writeRoundTripEPUB(t, src)
	file := &models.File{ID: 3, FileType: models.FileTypeEPUB, Identifiers: []*models.FileIdentifier{
		{Type: models.IdentifierTypeUUID, Value: "11111111-2222-4333-8444-555555555555"},
	}}
	azw3 := generateAZW3(t, models.FileTypeEPUB, src, &models.Book{Title: "New ID"}, file)

	f, err := os.Open(azw3)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	content, err := mobi.ReadContent(context.Background(), f, info.Size())
	require.NoError(t, err)
	var fonts [][]byte
	for _, font := range content.Fonts {
		fonts = append(fonts, font.Content)
	}
	assert.ElementsMatch(t, [][]byte{plain, hidden}, fonts)
}
