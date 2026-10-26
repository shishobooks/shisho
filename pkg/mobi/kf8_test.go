package mobi

import (
	"bytes"
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anchorAt returns where the start tag a position names begins, and its id
// or "" when it has none, or -1 when there is no such tag.
func anchorAt(doc []byte, pos int) (int, string) {
	t, ok := anchorTag(doc, startTags(doc), pos)
	if !ok {
		return -1, ""
	}
	return t.start, t.id
}

func readBuiltContent(t *testing.T, opts testgen.MOBIOptions) (*Content, error) {
	t.Helper()
	data := testgen.BuildMOBI(t, opts)
	return ReadContent(context.Background(), bytes.NewReader(data), int64(len(data)))
}

func TestReadKF8_ReassemblesChapters(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{"AZW3": testgen.MOBIKindKF8, "combo": testgen.MOBIKindCombo} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			book, err := readBuiltContent(t, testgen.KF8Sample(kind))
			require.NoError(t, err)

			require.Len(t, book.Documents, 2)
			one, two := string(book.Documents[0].Content), string(book.Documents[1].Content)
			assert.Equal(t, "part0000.xhtml", book.Documents[0].Name)
			assert.Equal(t, "part0001.xhtml", book.Documents[1].Name)
			assert.True(t, strings.HasPrefix(one, `<?xml version="1.0" encoding="utf-8"?>`), one[:60])
			assert.True(t, strings.HasSuffix(one, `</body></html>`))
			// Both fragments of chapter one are back in its body, in order,
			// and multibyte text split across text records is whole.
			assert.Contains(t, one, `<h1 id="ch1">Chapter One</h1>`)
			assert.Less(t, strings.Index(one, "Chapter One"), strings.Index(one, "Later in chapter one."))
			assert.Equal(t, 120, strings.Count(one, "Ünïcödé filler"))
			assert.Contains(t, two, "Chapter Two")

			for _, doc := range book.Documents {
				assert.NotRegexp(t, `["(]kindle:`, string(doc.Content), doc.Name)
				assert.NotRegexp(t, `\said=`, string(doc.Content), doc.Name)
			}
			assert.Contains(t, two, "like kindle:embed:0001 inside", "a kindle: URI in the text is not a reference")
			assert.Contains(t, two, `<a href="https://example.com/">`)
		})
	}
}

func TestReadKF8_ResolvesLinks(t *testing.T) {
	t.Parallel()
	book, err := readBuiltContent(t, testgen.KF8Sample(testgen.MOBIKindKF8))
	require.NoError(t, err)
	one, two := string(book.Documents[0].Content), string(book.Documents[1].Content)

	assert.Contains(t, one, `<a href="part0001.xhtml#ch2">the next chapter</a>`)
	assert.Contains(t, one, `<a href="part0000.xhtml#later">later on</a>`)
	assert.Contains(t, two, `<a href="part0000.xhtml#ch1">the start</a>`)
	// An element with no id gets one for the link to it.
	assert.Regexp(t, `<a href="part0001.xhtml#(kindle-pos-1-\d+)">the end</a>`, two)
	assert.Regexp(t, `<p id="kindle-pos-1-\d+" data-anchor="end">The end.</p>`, two)
	assert.Contains(t, one, `<link href="flow0001.css" rel="stylesheet" type="text/css"/>`)

	// Every link names a document of the book and an id in it.
	ids := map[string]bool{}
	for _, doc := range book.Documents {
		for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllStringSubmatch(string(doc.Content), -1) {
			ids[doc.Name+"#"+m[1]] = true
		}
	}
	for _, doc := range book.Documents {
		for _, m := range regexp.MustCompile(`href="(part[^"]+)"`).FindAllStringSubmatch(string(doc.Content), -1) {
			assert.True(t, ids[m[1]], "%s links to %s", doc.Name, m[1])
		}
	}
}

func TestReadKF8_ImagesAndStyles(t *testing.T) {
	t.Parallel()
	data := testgen.BuildMOBI(t, testgen.KF8Sample(testgen.MOBIKindKF8))
	book, err := ReadContent(context.Background(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	require.Len(t, book.Styles, 1)
	assert.Equal(t, "flow0001.css", book.Styles[0].Name)
	assert.Equal(t, "text/css", book.Styles[0].MediaType)
	assert.Equal(t, `h1 { color: red; } body { background: url(image0003.png); }`, string(book.Styles[0].Content))
	assert.Contains(t, string(book.Documents[0].Content), `<img src="image0003.png" alt="plate"/>`)

	// The cover and the plate are resources; the thumbnail nothing names is
	// left out.
	require.NotNil(t, book.Cover)
	assert.Equal(t, "image0001.jpg", book.Cover.Name)
	assert.Equal(t, "image/jpeg", book.Cover.MediaType)
	var names []string
	for _, r := range book.Resources {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"image0001.jpg", "image0003.png"}, names)
	plate, err := book.ReadResource(book.Resources[1])
	require.NoError(t, err)
	assert.Equal(t, testgen.KF8SamplePlate, plate)
	assert.Equal(t, "image/png", book.Resources[1].MediaType)
	assert.Equal(t, int64(len(testgen.KF8SamplePlate)), book.Resources[1].Size)
}

func TestReadKF8_TOCAndMetadata(t *testing.T) {
	t.Parallel()
	book, err := readBuiltContent(t, testgen.KF8Sample(testgen.MOBIKindKF8))
	require.NoError(t, err)

	require.Len(t, book.TOC, 2)
	assert.Equal(t, "Chapter One", book.TOC[0].Title)
	assert.Equal(t, "part0000.xhtml#ch1", book.TOC[0].Href)
	require.Len(t, book.TOC[0].Children, 1)
	assert.Equal(t, "part0000.xhtml#later", book.TOC[0].Children[0].Href)
	assert.Equal(t, "part0001.xhtml#ch2", book.TOC[1].Href)

	// A stored chapter's location finds the same place.
	data := testgen.BuildMOBI(t, testgen.KF8Sample(testgen.MOBIKindKF8))
	tocs, err := ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	href, ok := book.Href(tocs[0].Entries[1].Locator)
	require.True(t, ok)
	assert.Equal(t, "part0001.xhtml#ch2", href)
	_, ok = book.Href("filepos:0000000001")
	assert.False(t, ok)

	assert.Equal(t, "Kindle Book", book.Metadata.Title)
	require.Len(t, book.Metadata.Authors, 1)
	assert.Equal(t, "Kay Author", book.Metadata.Authors[0].Name)
}

func TestReadKF8_LinkToElementWithoutID(t *testing.T) {
	t.Parallel()
	doc := []byte(`<body><p>One</p><p class="x">Two</p></body>`)
	first := bytes.Index(doc, []byte(`<p>`))
	second := bytes.Index(doc, []byte(`<p class`))

	// A position names the start tag it is in or at, or else the start tag
	// of the text it is in.
	for name, c := range map[string]struct{ pos, want int }{
		"at a start tag":     {second, second},
		"inside a start tag": {second + 3, second},
		"in text":            {first + 4, first},
		"in an end tag":      {second - 2, first},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tag, id := anchorAt(doc, c.pos)
			assert.Equal(t, c.want, tag)
			assert.Empty(t, id)
		})
	}

	withID := []byte(`<body><h2 class="t" id="here">Two</h2></body>`)
	tag, id := anchorAt(withID, bytes.Index(withID, []byte("<h2")))
	assert.Equal(t, bytes.Index(withID, []byte("<h2")), tag)
	assert.Equal(t, "here", id)

	// The first element inside one with an id names the same place.
	nested := []byte("<body><section id=\"ch\">\n\t<div class=\"h\"><h2>Two</h2></div></section></body>")
	tag, id = anchorAt(nested, bytes.Index(nested, []byte("<h2")))
	assert.Equal(t, bytes.Index(nested, []byte("<section")), tag)
	assert.Equal(t, "ch", id)
	// Text, or an element closed before it, breaks that.
	for _, doc := range [][]byte{
		[]byte(`<section id="ch">Intro <h2>Two</h2></section>`),
		[]byte(`<section id="ch"><br/><h2>Two</h2></section>`),
	} {
		tag, id = anchorAt(doc, bytes.Index(doc, []byte("<h2")))
		assert.Equal(t, bytes.Index(doc, []byte("<h2")), tag, string(doc))
		assert.Empty(t, id, string(doc))
	}
}

func TestReadKF8_DeclinesWhatItCannotConvert(t *testing.T) {
	t.Parallel()

	encrypted := testgen.KF8Sample(testgen.MOBIKindKF8)
	encrypted.Encrypted = true
	_, err := readBuiltContent(t, encrypted)
	require.ErrorIs(t, err, ErrDRMProtected)

	for name, opts := range map[string]testgen.MOBIOptions{
		"no skeleton index": {Kind: testgen.MOBIKindKF8, Title: "Bare"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := readBuiltContent(t, opts)
			var unsupported *UnsupportedError
			require.ErrorAs(t, err, &unsupported)
			assert.NotEmpty(t, unsupported.Reason)
		})
	}
}

func TestPalmDOCDecompress(t *testing.T) {
	t.Parallel()
	// "abc", a run of one literal byte (0x01), a back-reference copying
	// three bytes from three back, and a space pair (" x").
	in := []byte{'a', 'b', 'c', 0x01, 0x01, 0x80, 3 << 3, 0x80 ^ 'x'}
	out, err := palmDOCDecompress(in)
	require.NoError(t, err)
	assert.Equal(t, "abc\x01bc\x01 x", string(out))

	_, err = palmDOCDecompress([]byte{0x80, 0x08})
	require.Error(t, err, "a back-reference before the start")
	_, err = palmDOCDecompress([]byte{0x03, 'a'})
	require.Error(t, err, "a literal run past the end")
}

func TestReadKF8_FromFile(t *testing.T) {
	t.Parallel()
	path := testgen.GenerateMOBI(t, t.TempDir(), "book.azw3", testgen.KF8Sample(testgen.MOBIKindKF8))
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	_, err = ReadContent(context.Background(), f, info.Size())
	require.NoError(t, err)
}

// A link to an element whose id is empty replaces the empty id, so the tag
// never has two, and a new id never repeats one the document has.
func TestAnchor_EmptyAndTakenIDs(t *testing.T) {
	t.Parallel()
	// The id the empty one would get, kindle-pos-0-39, is taken.
	doc := []byte(`<body><p id="kindle-pos-0-39">Taken</p><p id="">Empty</p></body>`)
	empty := bytes.Index(doc, []byte(`<p id=""`))
	require.Equal(t, 39, empty)
	p := &kf8Parts{docs: [][]byte{doc}, starts: []int{0}, inserts: []int{0}}
	loc := "KINDLE:POS:FID:0000:OFF:" + strings.ToUpper(strconv.FormatInt(int64(empty), 32))
	hrefs := p.anchor(map[string]bool{loc: true})

	assert.Equal(t, "part0000.xhtml#kindle-pos-0-39-2", hrefs[loc])
	assert.Equal(t, `<body><p id="kindle-pos-0-39">Taken</p><p id="kindle-pos-0-39-2">Empty</p></body>`, string(p.docs[0]))
}

func TestAssembleParts_RejectsCraftedIndexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	flow := []byte("<a></a><b></b>XY")

	p, err := assembleParts(ctx, []kf8Skeleton{{fragments: 1, start: 0, length: 7}, {fragments: 1, start: 8, length: 7}},
		[]kf8Fragment{{insert: 3, length: 1}, {insert: 11, length: 1}}, []byte("<a></a>X<b></b>Y"))
	require.NoError(t, err)
	assert.Equal(t, "<a>X</a>", string(p.docs[0]))
	assert.Equal(t, "<b>Y</b>", string(p.docs[1]))

	// Skeletons that overlap would let a small file assemble huge documents.
	_, err = assembleParts(ctx, []kf8Skeleton{{start: 0, length: 7}, {start: 0, length: 7}}, nil, flow)
	require.Error(t, err)
	_, err = assembleParts(ctx, []kf8Skeleton{{start: 3, length: 4}}, nil, flow)
	require.Error(t, err)

	// Fragments that each go in at the start of a large document move it
	// every time.
	big := bytes.Repeat([]byte("x"), 1<<20)
	frags := make([]kf8Fragment, 300)
	for i := range frags {
		frags[i] = kf8Fragment{insert: 0, length: 1}
	}
	_, err = assembleParts(ctx, []kf8Skeleton{{fragments: len(frags), start: 0, length: len(big)}}, frags, append(big, bytes.Repeat([]byte("y"), len(frags))...))
	require.ErrorContains(t, err, "out of order")
}

func TestEditDocument_StyleElements(t *testing.T) {
	t.Parallel()
	upper := bytes.ToUpper
	mark := func(b []byte) []byte { return append([]byte("[css]"), b...) }

	for name, c := range map[string]struct{ in, want string }{
		// Lowercasing "\xe9" (Windows-1252 é, invalid UTF-8) changes its
		// length, so offsets must come from the bytes as they are.
		"text that isn't UTF-8":  {"<style>a\xe9\xe9\xe9</style><p>x</p>", "<STYLE>[css]a\xe9\xe9\xe9</style><P>x</p>"},
		"an end tag in capitals": {"<style>a</STYLE><p>x</p>", "<STYLE>[css]a</STYLE><P>x</p>"},
		"a self-closed style":    {"<style/><p>x</p><style>b</style>", "<STYLE/><P>x</p><STYLE>[css]b</style>"},
		"a style never closed":   {"<style>a<p>x</p>", "<STYLE>a<P>x</p>"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, string(editDocument([]byte(c.in), upper, mark)))
		})
	}
}

// A document full of style elements is edited in linear time.
func TestEditDocument_ManyStyles(t *testing.T) {
	t.Parallel()
	doc := append(bytes.Repeat([]byte("<style>a</style>"), 20_000), bytes.Repeat([]byte("text "), 1<<20)...)
	start := time.Now()
	out := editDocument(doc, func(b []byte) []byte { return b }, func(b []byte) []byte { return b })
	assert.Equal(t, doc, out)
	assert.Less(t, time.Since(start), 5*time.Second)
}

// KindleGen, building a book from HTML, writes documents without the
// XHTML namespace and with its own data-AmznPageBreak attributes: the
// documents get the namespace, and the attributes go, as aid does.
func TestReadKF8_KindleGenHTMLMarkup(t *testing.T) {
	t.Parallel()
	book, err := readBuiltContent(t, testgen.MOBIOptions{
		Kind:  testgen.MOBIKindKF8,
		Title: "From HTML",
		KF8: &testgen.MOBIKF8Content{Parts: []testgen.MOBIKF8Part{{
			Head:      `<html><head><title>One</title></head><body aid="0">`,
			Tail:      `</body></html>`,
			Fragments: []string{`<p aid="1" data-AmznPageBreak="always" class="c">One.</p>`},
		}}},
	})
	require.NoError(t, err)
	require.Len(t, book.Documents, 1)
	assert.Equal(t, `<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p class="c">One.</p></body></html>`, string(book.Documents[0].Content))
}
