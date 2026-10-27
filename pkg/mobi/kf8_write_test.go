package mobi

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodedTestImage(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 60))
	for y := range 60 {
		for x := range 40 {
			img.Set(x, y, color.RGBA{uint8(x * 6), uint8(y * 4), 120, 255}) //nolint:gosec // small test values
		}
	}
	var buf bytes.Buffer
	require.NoError(t, encode(&buf, img))
	return buf.Bytes()
}

func testPNG(t *testing.T) []byte {
	return encodedTestImage(t, func(b *bytes.Buffer, img image.Image) error { return png.Encode(b, img) })
}

func testJPEG(t *testing.T) []byte {
	return encodedTestImage(t, func(b *bytes.Buffer, img image.Image) error { return jpeg.Encode(b, img, nil) })
}

const testCSS = `p { margin: 0; text-indent: 1em; }
.boxed { border: 2px solid black; background: url("../images/pic.png"); }
`

// testChapterOne is long enough to be cut into several fragments: a section
// holding more than a fragment's worth of paragraphs, and many paragraphs
// after it, some with text that is not ASCII, so record boundaries fall
// inside characters.
func testChapterOne() string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en">
<head>
  <title>Chapter One</title>
  <link href="../css/style.css" rel="stylesheet" type="text/css"/>
</head>
<body class="book">
<h1 id="top">Chapter One</h1>
<p class="boxed">A picture: <img src="../images/pic.png" alt="pic"/></p>
<div id="sec" class="section">
`)
	for i := range 120 {
		fmt.Fprintf(&b, "<p>Section paragraph %d, with some words to fill the line out.</p>\n", i)
	}
	b.WriteString("</div>\n")
	for i := range 200 {
		fmt.Fprintf(&b, "<p>Paragraph %d: café, naïve, 日本語のテキスト, and <em>emphasis</em>.<br/></p>\n", i)
	}
	b.WriteString("<!-- a comment with <p>markup</p> in it -->\n</body>\n</html>\n")
	return b.String()
}

const testChapterTwo = `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter Two</title><link rel="stylesheet" href="../css/style.css"/></head>
<body>
<section id="c2"><h2>Chapter Two</h2><p>The second chapter.</p><p><img src="../images/cover.jpg" alt=""/></p></section>
</body>
</html>
`

func testKF8Book(t *testing.T) *KF8Book {
	t.Helper()
	pic, cover := testPNG(t), testJPEG(t)
	released := time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)
	return &KF8Book{
		Metadata: Metadata{
			Title:       "The Writer Test",
			Authors:     []string{"Ada Writer", "Bo Second"},
			Description: "A book written by the KF8 writer.",
			Publisher:   "Pub House",
			ReleaseDate: &released,
			Language:    "fr",
			Genres:      []string{"Fantasy", "Adventure"},
			Identifiers: &Identifiers{ISBNs: []string{"9780306406157"}},
		},
		Identifier: "9a1d2c3e-0000-5000-8000-000000000001",
		Documents: []Document{
			{Name: "OEBPS/text/ch1.xhtml", Content: []byte(testChapterOne())},
			{Name: "OEBPS/text/ch2.xhtml", Content: []byte(testChapterTwo)},
		},
		Styles: []Document{{Name: "OEBPS/css/style.css", Content: []byte(testCSS)}},
		Images: []KF8Image{
			{Name: "OEBPS/images/pic.png", MediaType: "image/png", Load: func() ([]byte, error) { return pic, nil }},
			{Name: "OEBPS/images/cover.jpg", MediaType: "image/jpeg", Load: func() ([]byte, error) { return cover, nil }},
		},
		Cover: "OEBPS/images/cover.jpg",
		TOC: []*NavPoint{
			{Title: "Chapter One", Href: "OEBPS/text/ch1.xhtml", Children: []*NavPoint{
				{Title: "Section", Href: "OEBPS/text/ch1.xhtml#sec"},
			}},
			{Title: "Chapter Two", Href: "OEBPS/text/ch2.xhtml#c2"},
		},
	}
}

// writeKF8File writes b to a file and returns its path.
func writeKF8File(t *testing.T, b *KF8Book) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "book.azw3")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, WriteKF8(context.Background(), f, b))
	require.NoError(t, f.Close())
	return path
}

func openContent(t *testing.T, path string) *Content {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	info, err := f.Stat()
	require.NoError(t, err)
	content, err := ReadContent(context.Background(), f, info.Size())
	require.NoError(t, err)
	return content
}

func TestWriteKF8_ReadsBackMetadataAndCover(t *testing.T) {
	t.Parallel()
	b := testKF8Book(t)
	path := writeKF8File(t, b)

	meta, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "The Writer Test", meta.Title)
	require.Len(t, meta.Authors, 2)
	assert.Equal(t, "Ada Writer", meta.Authors[0].Name)
	assert.Equal(t, "Bo Second", meta.Authors[1].Name)
	assert.Equal(t, "A book written by the KF8 writer.", meta.Description)
	assert.Equal(t, "Pub House", meta.Publisher)
	require.NotNil(t, meta.ReleaseDate)
	assert.Equal(t, "2021-03-04", meta.ReleaseDate.Format("2006-01-02"))
	require.NotNil(t, meta.Language)
	assert.Equal(t, "fr", *meta.Language)
	assert.Equal(t, []string{"Fantasy", "Adventure"}, meta.Genres)
	require.Len(t, meta.Identifiers, 1)
	assert.Equal(t, "9780306406157", meta.Identifiers[0].Value)

	cover, err := b.Images[1].Load()
	require.NoError(t, err)
	assert.Equal(t, cover, meta.CoverData)
	assert.Equal(t, "image/jpeg", meta.CoverMimeType)

	require.Len(t, meta.Chapters, 2)
	assert.Equal(t, "Chapter One", meta.Chapters[0].Title)
	require.Len(t, meta.Chapters[0].Children, 1)
	assert.Equal(t, "Section", meta.Chapters[0].Children[0].Title)
	assert.Equal(t, "Chapter Two", meta.Chapters[1].Title)
}

func TestWriteKF8_ReadsBackContent(t *testing.T) {
	t.Parallel()
	b := testKF8Book(t)
	content := openContent(t, writeKF8File(t, b))

	// The reader drops the aid attributes and turns kindle: references
	// into its own names, so with those names the documents come back as
	// they were written.
	require.Len(t, content.Documents, 2)
	want := strings.NewReplacer(`"../images/pic.png"`, `"image0001.png"`, `"../css/style.css"`, `"flow0001.css"`)
	assert.Equal(t, want.Replace(testChapterOne()), string(content.Documents[0].Content))
	assert.Equal(t, strings.NewReplacer(`"../images/cover.jpg"`, `"image0002.jpg"`, `"../css/style.css"`, `"flow0001.css"`).Replace(testChapterTwo), string(content.Documents[1].Content))

	require.Len(t, content.Styles, 1)
	assert.Equal(t, strings.Replace(testCSS, `url("../images/pic.png")`, `url(image0001.png)`, 1), string(content.Styles[0].Content))

	require.Len(t, content.Resources, 2)
	pic, err := content.ReadResource(content.Resources[0])
	require.NoError(t, err)
	wantPic, err := b.Images[0].Load()
	require.NoError(t, err)
	assert.Equal(t, wantPic, pic)
	require.NotNil(t, content.Cover)
	assert.Equal(t, "image0002.jpg", content.Cover.Name)

	require.Len(t, content.TOC, 2)
	assert.Equal(t, "part0000.xhtml#top", content.TOC[0].Href)
	require.Len(t, content.TOC[0].Children, 1)
	assert.Equal(t, "part0000.xhtml#sec", content.TOC[0].Children[0].Href)
	assert.Equal(t, "part0001.xhtml#c2", content.TOC[1].Href)
}

// A long document is cut into fragments of about a fragment's size, and
// every element in a body carries an aid, which the fragment selectors name.
func TestWriteKF8_CutsDocumentsIntoFragments(t *testing.T) {
	t.Parallel()
	path := writeKF8File(t, testKF8Book(t))
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	db, err := readPDB(f, info.Size())
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)

	frags, err := readNCX(db, int(headerField(rec, offFragment)))
	require.NoError(t, err)
	assert.Greater(t, len(frags.entries), 4)
	for _, e := range frags.entries {
		geometry := e.values[6]
		require.Len(t, geometry, 2)
		assert.LessOrEqual(t, int(geometry[1]), 2*fragmentSize, "fragment %s", e.ident)
	}

	text, err := readText(context.Background(), db, 0, rec, compressionPalmDOC, true)
	require.NoError(t, err)
	assert.Contains(t, string(text), `<body aid="`)
	assert.Contains(t, string(text), `<h1 aid="`)
}

func TestWriteKF8_CarriesTheIdentifier(t *testing.T) {
	t.Parallel()
	b := testKF8Book(t)
	path := writeKF8File(t, b)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	h, err := parseHeader(rec)
	require.NoError(t, err)
	assert.Equal(t, b.Identifier, h.text(exthASIN))
	assert.Equal(t, b.Identifier, h.text(exthASINAlt))
	assert.Equal(t, "EBOK", h.text(exthDocType))
	assert.True(t, h.isKF8())
}

// A generated file can have its metadata rewritten like any other AZW3, as
// the AZW3 generator does on a later download of a file added to a library.
func TestWriteKF8_CanBeRewritten(t *testing.T) {
	t.Parallel()
	path := writeKF8File(t, testKF8Book(t))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, Rewrite(bytes.NewReader(data), int64(len(data)), &out, &Metadata{Title: "Renamed"}))
	rewritten := filepath.Join(t.TempDir(), "renamed.azw3")
	require.NoError(t, os.WriteFile(rewritten, out.Bytes(), 0o600))
	meta, err := Parse(rewritten)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", meta.Title)
}

// An image a Kindle can't show, such as a WebP, is written as a JPEG, and
// one that is not a bitmap at all, such as an SVG file, is left out.
func TestWriteKF8_ConvertsImages(t *testing.T) {
	t.Parallel()
	webp, err := os.ReadFile("../covers/testdata/lossy-alpha.webp")
	require.NoError(t, err)
	b := &KF8Book{
		Metadata:  Metadata{Title: "Images"},
		Documents: []Document{{Name: "a.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>A</title></head><body><p><img src="pic.webp"/><img src="logo.svg"/></p></body></html>`)}},
		Images: []KF8Image{
			{Name: "pic.webp", MediaType: "image/webp", Load: func() ([]byte, error) { return webp, nil }},
			{Name: "logo.svg", MediaType: "image/svg+xml", Load: func() ([]byte, error) { return []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), nil }},
		},
		Cover: "pic.webp",
	}
	content := openContent(t, writeKF8File(t, b))
	require.Len(t, content.Resources, 1)
	assert.Equal(t, "image/jpeg", content.Resources[0].MediaType)
	assert.Contains(t, string(content.Documents[0].Content), `<img src="image0001.jpg"/><img src="logo.svg"/>`)
	require.NotNil(t, content.Cover)
}

// A book with no documents has nothing to read.
func TestWriteKF8_RefusesABookWithNoDocuments(t *testing.T) {
	t.Parallel()
	f, err := os.Create(filepath.Join(t.TempDir(), "empty.azw3"))
	require.NoError(t, err)
	defer f.Close()
	assert.ErrorIs(t, WriteKF8(context.Background(), f, &KF8Book{Metadata: Metadata{Title: "Empty"}}), ErrNoDocuments)
}

// A Kindle reads documents as HTML, where "<a/>" opens an element that
// takes in the text after it, so a self-closed element that isn't void is
// written with an end tag. Malformed markup still comes back as written.
func TestWriteKF8_WritesDocumentsAKindleParses(t *testing.T) {
	t.Parallel()
	in := `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title/><script src="x.js"/></head>
<BODY><p><a id="page1"/>Text after an anchor.<br/></p><div/>
<p>Unclosed <b>bold <i>italic</p><![CDATA[ <p>not markup</p> ]]>
<style>p > i { color: red } /* </div> */</style><p>Last < bare</p></BODY></html>`
	want := strings.NewReplacer(`<title/>`, `<title></title>`, `<script src="x.js"/>`, `<script src="x.js"></script>`,
		`<a id="page1"/>`, `<a id="page1"></a>`, `<div/>`, `<div></div>`).Replace(in)
	content := openContent(t, writeKF8File(t, &KF8Book{Metadata: Metadata{Title: "Markup"}, Documents: []Document{{Name: "a.xhtml", Content: []byte(in)}}}))
	assert.Equal(t, want, string(content.Documents[0].Content))
}

// A table of contents entry naming a document the book doesn't have is left
// out, its children kept, and a title too long for an index record is cut.
func TestWriteKF8_CleansTheTableOfContents(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", 40000)
	b := &KF8Book{
		Metadata:  Metadata{Title: "TOC"},
		Documents: []Document{{Name: "a.xhtml", Content: []byte(`<html><body><h1 id="ü">One</h1></body></html>`)}},
		TOC: []*NavPoint{
			{Title: "Missing", Href: "notes.xhtml", Children: []*NavPoint{{Title: "Kept", Href: "a.xhtml#%C3%BC"}}},
			{Title: long, Href: "a.xhtml"},
		},
	}
	content := openContent(t, writeKF8File(t, b))
	require.Len(t, content.TOC, 2)
	assert.Equal(t, "Kept", content.TOC[0].Title)
	assert.Equal(t, "part0000.xhtml#ü", content.TOC[0].Href)
	assert.True(t, strings.HasPrefix(long, content.TOC[1].Title))
	assert.Less(t, len(content.TOC[1].Title), len(long))
}

// A fixed-layout book, such as a comic, says so in its EXTH block with the
// size its pages were made for; a reflowable one has neither record.
func TestWriteKF8_MarksFixedLayout(t *testing.T) {
	t.Parallel()
	header := func(b *KF8Book) *header {
		data, err := os.ReadFile(writeKF8File(t, b))
		require.NoError(t, err)
		db, err := readPDB(bytes.NewReader(data), int64(len(data)))
		require.NoError(t, err)
		rec, err := db.record(0, maxHeaderRecord)
		require.NoError(t, err)
		h, err := parseHeader(rec)
		require.NoError(t, err)
		return h
	}
	doc := []Document{{Name: "p.xhtml", Content: []byte(`<html><body><img src="p.jpg"/></body></html>`)}}

	h := header(&KF8Book{Metadata: Metadata{Title: "Comic"}, Documents: doc, FixedLayout: true, OriginalResolution: "1072x1448"})
	assert.Equal(t, "true", h.text(exthFixedLayout))
	assert.Equal(t, "1072x1448", h.text(exthOriginalResolution))

	h = header(&KF8Book{Metadata: Metadata{Title: "Novel"}, Documents: doc})
	assert.Empty(t, h.text(exthFixedLayout))
	assert.Empty(t, h.text(exthOriginalResolution))
}

// testFont is a TrueType font as far as a sniff of its first bytes goes,
// with bytes after the header that don't compress.
func testFont() []byte {
	font := []byte("\x00\x01\x00\x00")
	for i := range 3000 {
		font = append(font, byte(i*7919>>3))
	}
	return font
}

// linkedBook has links of every kind: a footnote reference into a notes
// document and the note's link back, a link to a place later in the same
// long document, which falls in another fragment, a link to a whole
// document, a link to a document the book doesn't have, and a link out of
// the book.
func linkedBook() *KF8Book {
	var one strings.Builder
	one.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>One</title></head><body>
<h1 id="top">One</h1>
<p>A claim.<a id="ref1" epub:type="noteref" href="../notes/notes.xhtml#n1">1</a> Skip <a href="#later">ahead</a>, read <a href="two.xhtml">two</a>, <a href="missing.xhtml#x">nowhere</a>, or <a href="https://example.com/#a">the web</a>.</p>
`)
	for i := range 300 {
		fmt.Fprintf(&one, "<p>Filler paragraph %d keeps the chapter long enough for several fragments.</p>\n", i)
	}
	one.WriteString(`<p id="later">Later on.</p></body></html>`)
	return &KF8Book{
		Metadata: Metadata{Title: "Links"},
		Documents: []Document{
			{Name: "OEBPS/text/one.xhtml", Content: []byte(one.String())},
			{Name: "OEBPS/text/two.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Two</title></head><body><h1>Two</h1><p>Back to <a href="one.xhtml#top">the start</a>.</p></body></html>`)},
			{Name: "OEBPS/notes/notes.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Notes</title></head><body><aside id="n1" epub:type="footnote"><p><a href="../text/one.xhtml#ref1">1.</a> The note.</p></aside></body></html>`)},
		},
		TOC: []*NavPoint{{Title: "One", Href: "OEBPS/text/one.xhtml"}, {Title: "Two", Href: "OEBPS/text/two.xhtml"}},
	}
}

// Links between the book's documents become kindle:pos references to the
// place they name, which a reader resolves back to the element there; links
// out of the book, and to documents it doesn't have, are kept as written.
func TestWriteKF8_RewritesInternalLinks(t *testing.T) {
	t.Parallel()
	path := writeKF8File(t, linkedBook())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	text, err := readText(context.Background(), db, 0, rec, compressionPalmDOC, true)
	require.NoError(t, err)
	assert.NotContains(t, string(text), `notes.xhtml#n1"`)
	assert.Regexp(t, `href="kindle:pos:fid:[0-9A-V]{4}:off:[0-9A-V]{10}"`, string(text))

	content := openContent(t, path)
	require.Len(t, content.Documents, 3)
	one, two, notes := string(content.Documents[0].Content), string(content.Documents[1].Content), string(content.Documents[2].Content)
	assert.Contains(t, one, `epub:type="noteref" href="part0002.xhtml#n1">1</a>`)
	assert.Contains(t, one, `<a href="part0000.xhtml#later">ahead</a>`)
	assert.Regexp(t, `<a href="part0001\.xhtml#[^"]+">two</a>`, one, "a link to a whole document names its first element")
	assert.Contains(t, one, `<a href="missing.xhtml#x">nowhere</a>`)
	assert.Contains(t, one, `<a href="https://example.com/#a">the web</a>`)
	assert.Contains(t, two, `<a href="part0000.xhtml#top">the start</a>`)
	assert.Contains(t, notes, `<a href="part0000.xhtml#ref1">1.</a>`)
	assert.Contains(t, one, `<p id="later">Later on.</p>`, "the target keeps its id")
}

// A font the stylesheets name is stored as a FONT resource, and a url()
// naming it becomes a kindle:embed reference; data that is no font is left
// out, its url() kept as written.
func TestWriteKF8_WritesFonts(t *testing.T) {
	t.Parallel()
	font := testFont()
	b := &KF8Book{
		Metadata:  Metadata{Title: "Fonts"},
		Documents: []Document{{Name: "OEBPS/text/a.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>A</title><link rel="stylesheet" href="../css/a.css"/></head><body><h1>Heading</h1><p><img src="../images/pic.png"/></p></body></html>`)}},
		Styles: []Document{{Name: "OEBPS/css/a.css", Content: []byte(`@font-face { font-family: "Body"; src: url("../fonts/body.ttf") format("truetype"); }
@font-face { font-family: "Broken"; src: url(../fonts/broken.otf); }
h1 { font-family: "Body", serif; }`)}},
		Images: []KF8Image{{Name: "OEBPS/images/pic.png", MediaType: "image/png", Load: func() ([]byte, error) { return testPNG(t), nil }}},
		Fonts: []Document{
			{Name: "OEBPS/fonts/body.ttf", MediaType: "font/ttf", Content: font},
			{Name: "OEBPS/fonts/broken.otf", MediaType: "font/otf", Content: []byte("not a font at all")},
		},
	}
	content := openContent(t, writeKF8File(t, b))
	require.Len(t, content.Fonts, 1)
	assert.Equal(t, font, content.Fonts[0].Content)
	assert.Equal(t, "font/ttf", content.Fonts[0].MediaType)
	require.Len(t, content.Styles, 1)
	css := string(content.Styles[0].Content)
	assert.Contains(t, css, `src: url(`+content.Fonts[0].Name+`) format("truetype");`)
	assert.Contains(t, css, `src: url(../fonts/broken.otf);`)
	require.Len(t, content.Resources, 1, "the image is still read")
	assert.Contains(t, string(content.Documents[0].Content), `<img src="`+content.Resources[0].Name+`"/>`)
}

// A stylesheet imported by another becomes a kindle:flow reference, and a
// large stylesheet, spanning text records and holding text that is not
// ASCII, comes back as written.
func TestWriteKF8_CarriesStylesheets(t *testing.T) {
	t.Parallel()
	var heavy strings.Builder
	heavy.WriteString("@import url(\"base.css\");\n@import 'extra.css';\n")
	for i := range 400 {
		fmt.Fprintf(&heavy, ".c%d > p:first-child::before { content: \"→ %d «»\"; margin: %dpx 0 0 1em; }\n", i, i, i%9)
	}
	heavy.WriteString("@media (orientation: landscape) { p { text-indent: 2em; } }\n")
	b := &KF8Book{
		Metadata:  Metadata{Title: "Styles"},
		Documents: []Document{{Name: "s/a.xhtml", Content: []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>A</title><link rel="stylesheet" href="heavy.css"/></head><body><p class="c1">Styled.</p></body></html>`)}},
		Styles: []Document{
			{Name: "s/heavy.css", Content: []byte(heavy.String())},
			{Name: "s/base.css", Content: []byte("p { margin: 0; }")},
			{Name: "s/extra.css", Content: []byte("p { color: #333; }")},
		},
	}
	content := openContent(t, writeKF8File(t, b))
	require.Len(t, content.Styles, 3)
	want := strings.NewReplacer(`url("base.css")`, `url(flow0002.css)`, `'extra.css'`, `url(flow0003.css)`).Replace(heavy.String())
	assert.Equal(t, want, string(content.Styles[0].Content))
	assert.Equal(t, "p { margin: 0; }", string(content.Styles[1].Content))
	assert.Contains(t, string(content.Documents[0].Content), `href="flow0001.css"`)
}

// The guide index names the places the book's guide gives, as a fragment
// and an offset, with each title in its CNCX.
func TestWriteKF8_WritesTheGuide(t *testing.T) {
	t.Parallel()
	b := linkedBook()
	b.Guide = []GuideRef{
		{Type: "toc", Title: "Contents", Href: "OEBPS/text/two.xhtml"},
		{Type: "text", Title: "Beginning", Href: "OEBPS/text/one.xhtml#later"},
		{Type: "notes", Title: "Gone", Href: "OEBPS/missing.xhtml"},
	}
	path := writeKF8File(t, b)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	at := headerField(rec, offGuide)
	require.NotEqual(t, uint32(nullIndex), at)
	guide, err := readNCX(db, int(at))
	require.NoError(t, err)
	require.Len(t, guide.entries, 2, "a reference to a document the book doesn't have is left out")
	assert.Equal(t, "text", string(guide.entries[0].ident))
	assert.Equal(t, "toc", string(guide.entries[1].ident))
	for i, want := range []string{"Beginning", "Contents"} {
		offset, ok := guide.entries[i].value(guideTagTitle)
		require.True(t, ok)
		label, ok := guide.label(offset)
		require.True(t, ok)
		assert.Equal(t, want, string(label))
	}

	// A Kindle opens the book where the guide's text starts: the element
	// with id "later", at its place in the assembled documents.
	h, err := parseHeader(rec)
	require.NoError(t, err)
	start, ok := h.number(exthStartReading)
	require.True(t, ok)
	text, err := readText(context.Background(), db, 0, rec, compressionPalmDOC, true)
	require.NoError(t, err)
	parts, err := readParts(context.Background(), db, 0, rec, text)
	require.NoError(t, err)
	doc := parts.docs[0]
	pos := int(start) - parts.starts[0]
	require.Less(t, pos, len(doc))
	assert.True(t, bytes.HasPrefix(doc[pos:], []byte(`<p aid="`)), "%q", doc[pos:min(len(doc), pos+40)])
	assert.Contains(t, string(doc[pos:min(len(doc), pos+40)]), `id="later"`)

	// The guide's places resolve like links do.
	content := openContent(t, path)
	for i, want := range []string{"part0000.xhtml#later", "part0001.xhtml#"} {
		pos := guide.entries[i].values[ncxTagPosFID]
		require.Len(t, pos, 2)
		loc := posURI(int(pos[0]), int(pos[1]))
		assert.True(t, strings.HasPrefix(content.hrefs[strings.ToUpper(loc)], want), "guide entry %d names %q", i, content.hrefs[strings.ToUpper(loc)])
	}
}

// A link written with an unquoted value right before "/>" still gets its
// end tag, and its value ends before the slash.
func TestWriteKF8_LinksWithUnquotedValues(t *testing.T) {
	t.Parallel()
	b := &KF8Book{
		Metadata: Metadata{Title: "Unquoted"},
		Documents: []Document{
			{Name: "one.xhtml", Content: []byte(`<html><body><p>See <a href=two.xhtml/>x</p></body></html>`)},
			{Name: "two.xhtml", Content: []byte(`<html><body><p id="t">Two</p></body></html>`)},
		},
	}
	content := openContent(t, writeKF8File(t, b))
	assert.Contains(t, string(content.Documents[0].Content), `<a href=part0001.xhtml#t></a>x</p>`)
}

// A book whose guide has no "text" entry has no start reading record.
func TestWriteKF8_NoGuideNoStartReading(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(writeKF8File(t, linkedBook()))
	require.NoError(t, err)
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	h, err := parseHeader(rec)
	require.NoError(t, err)
	_, ok := h.number(exthStartReading)
	assert.False(t, ok)
	assert.Equal(t, uint32(nullIndex), headerField(rec, offGuide))
}

// A Kindle reads a kindle: reference in CSS only unquoted, as KindleGen
// writes it, so the quotes around a rewritten url() are dropped, and an
// @import naming its stylesheet with a string becomes an unquoted url().
// A reference left as written keeps its quotes.
func TestWriteKF8_WritesCSSReferencesUnquoted(t *testing.T) {
	t.Parallel()
	b := &KF8Book{
		Metadata:  Metadata{Title: "Quotes"},
		Documents: []Document{{Name: "a.xhtml", Content: []byte(`<html><head><style>p { background: url( 'pic.png' ); }</style></head><body><p style="background: url(&quot;pic.png&quot;)">A</p><p style="background: url(&#34;pic.png&#34;)">B</p><p style='background: url(&#39;pic.png&#39;)'>C</p></body></html>`)}},
		Styles: []Document{
			{Name: "a.css", Content: []byte(`@import "b.css"; @import url('b.css') screen; @font-face { src: url("f.ttf") format("truetype"); } p { background: url("missing.png"); }`)},
			{Name: "b.css", Content: []byte("p { margin: 0; }")},
		},
		Images: []KF8Image{{Name: "pic.png", MediaType: "image/png", Load: func() ([]byte, error) { return testPNG(t), nil }}},
		Fonts:  []Document{{Name: "f.ttf", MediaType: "font/ttf", Content: testFont()}},
	}
	data, err := os.ReadFile(writeKF8File(t, b))
	require.NoError(t, err)
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	text, err := readText(context.Background(), db, 0, rec, compressionPalmDOC, true)
	require.NoError(t, err)
	assert.NotRegexp(t, `url\(\s*(["']|&quot;)kindle:`, string(text))
	assert.Contains(t, string(text), `@import url(kindle:flow:0002?mime=text/css); @import url(kindle:flow:0002?mime=text/css) screen;`)
	assert.Contains(t, string(text), `src: url(kindle:embed:0002) format("truetype");`)
	assert.Contains(t, string(text), `background: url(kindle:embed:0001?mime=image/png); }</style>`)
	assert.Equal(t, 2, strings.Count(string(text), `style="background: url(kindle:embed:0001?mime=image/png)"`))
	assert.Contains(t, string(text), `style='background: url(kindle:embed:0001?mime=image/png)'`)
	assert.Contains(t, string(text), `url("missing.png")`)
}
