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
	assert.Equal(t, strings.Replace(testCSS, `"../images/pic.png"`, `"image0001.png"`, 1), string(content.Styles[0].Content))

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
