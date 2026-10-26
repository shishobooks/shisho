package mobi

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xhtmlText checks that doc is well-formed XHTML and returns the text of
// its body, entities decoded, with a space between blocks and runs of
// whitespace collapsed.
func xhtmlText(t *testing.T, doc []byte) string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var text strings.Builder
	inBody := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "well-formed XML:\n%s", doc)
		switch tok := tok.(type) {
		case xml.StartElement:
			if tok.Name.Local == "html" {
				assert.Equal(t, "http://www.w3.org/1999/xhtml", tok.Name.Space)
			}
			if tok.Name.Local == "body" {
				inBody = true
			}
			if textBreaks[tok.Name.Local] {
				text.WriteByte(' ')
			}
		case xml.EndElement:
			if textBreaks[tok.Name.Local] {
				text.WriteByte(' ')
			}
		case xml.CharData:
			if inBody {
				text.Write(tok)
			}
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

var textBreaks = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "li": true, "td": true, "tr": true,
	"table": true, "blockquote": true, "ul": true, "ol": true, "br": true,
}

func TestReadContent_KF7(t *testing.T) {
	t.Parallel()
	book, err := readBuiltContent(t, testgen.KF7Sample())
	require.NoError(t, err)
	checkKF7Content(t, book)
}

// A KF7 book's text in Windows-1252 reads the same as in UTF-8.
func TestReadContent_KF7CP1252(t *testing.T) {
	t.Parallel()
	opts := testgen.KF7Sample()
	opts.CP1252 = true
	book, err := readBuiltContent(t, opts)
	require.NoError(t, err)
	checkKF7Content(t, book)
}

// checkKF7Content checks what testgen.KF7Sample converts to: a document per
// chapter and one for the printed table of contents, as valid XHTML with
// Mobipocket's markup gone, links and images resolved, and the table of
// contents pointing where the file's does.
func checkKF7Content(t *testing.T, book *Content) {
	t.Helper()
	assert.Equal(t, "Old Book", book.Metadata.Title)
	require.Len(t, book.Documents, 3, "the page breaks split the text, and nothing follows the last")
	one, two, toc := book.Documents[0], book.Documents[1], book.Documents[2]
	for i, doc := range book.Documents {
		assert.Equal(t, partName(i), doc.Name)
		assert.Equal(t, "application/xhtml+xml", doc.MediaType)
		assert.True(t, bytes.HasPrefix(doc.Content, []byte(`<?xml version="1.0" encoding="utf-8"?>`)), doc.Name)
		for _, stray := range []string{"filepos=", "recindex", "mbp:", "<font", "<center", "<guide", "<reference", "align=", "valign=", "height=\"1em\"", "width=\"0pt\"", "</br>", "</img>"} {
			assert.NotContains(t, string(doc.Content), stray, doc.Name)
		}
	}
	assert.Empty(t, book.Styles)

	text := xhtmlText(t, one.Content)
	assert.True(t, strings.HasPrefix(text, "Chapter One Read the next chapter or later on. Café & crème \u2014 “quotes”."), text[:80])
	assert.Equal(t, 120, strings.Count(text, "Ünïcödé filler text keeps the chapter long."))
	assert.True(t, strings.HasSuffix(text, "Later in chapter one."), text[len(text)-40:])
	assert.Equal(t, "Chapter Two Back to the start. Out to the web. On to the end. A paragraph left open A cell One Two Quoted. Centered. Mobipocket marks words too. The end.", xhtmlText(t, two.Content))
	assert.Equal(t, "Table of Contents Chapter One Chapter Two", xhtmlText(t, toc.Content))

	// Presentation moves into styles, and the void and self-closed elements
	// hold nothing.
	assert.Contains(t, string(one.Content), `style="text-indent: 2em"`)
	assert.Contains(t, string(one.Content), `color: #ff0000`)
	assert.Contains(t, string(toc.Content), `text-align: center`)
	assert.Equal(t, 1, strings.Count(string(two.Content), "<br"))
	assert.Contains(t, string(two.Content), `<a id="old"></a>`)
	assert.Regexp(t, `<img src="image0003.png" width="50" height="50" style="vertical-align: baseline" alt=""/>`, string(one.Content))

	link := func(doc Document, label string) string {
		m := regexp.MustCompile(`<a href="([^"]+)">` + regexp.QuoteMeta(label) + `</a>`).FindStringSubmatch(string(doc.Content))
		require.NotNil(t, m, "%s links %q", doc.Name, label)
		return m[1]
	}
	targets := map[string]string{}
	for _, doc := range book.Documents {
		for _, m := range regexp.MustCompile(`\sid="([^"]+)"`).FindAllStringSubmatch(string(doc.Content), -1) {
			assert.Empty(t, targets[m[1]], "id %s is unique", m[1])
			targets[m[1]] = doc.Name
		}
	}
	at := func(href string) (string, int) {
		name, id, ok := strings.Cut(href, "#")
		require.True(t, ok, href)
		require.Equal(t, name, targets[id], "%s names an id in its document", href)
		for _, doc := range book.Documents {
			if doc.Name == name {
				pos := strings.Index(string(doc.Content), `id="`+id+`"`)
				return name, strings.LastIndex(string(doc.Content[:pos]), "<")
			}
		}
		return "", -1
	}
	// textAfter returns the text from the element href names on.
	textAfter := func(href string) string {
		name, pos := at(href)
		for _, doc := range book.Documents {
			if doc.Name == name {
				rest := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(string(doc.Content[pos:]), " ")
				return strings.Join(strings.Fields(rest), " ")
			}
		}
		return ""
	}
	nextChapter := link(one, "the next chapter")
	assert.True(t, strings.HasPrefix(nextChapter, "part0001.xhtml#"), "a link to the page break before a chapter goes to the chapter: %s", nextChapter)
	assert.True(t, strings.HasPrefix(textAfter(nextChapter), "Chapter Two"))
	assert.True(t, strings.HasPrefix(textAfter(link(one, "later on")), "Later in chapter one."))
	assert.True(t, strings.HasPrefix(textAfter(link(two, "the start")), "Chapter One"))
	assert.True(t, strings.HasPrefix(textAfter(link(two, "the end")), "The end."))
	assert.True(t, strings.HasPrefix(textAfter(link(toc, "Chapter Two")), "Chapter Two"))
	assert.Contains(t, string(two.Content), `<a href="https://example.com/">the web</a>`)

	require.NotNil(t, book.Cover)
	assert.Equal(t, "image0001.jpg", book.Cover.Name)
	var names []string
	for _, r := range book.Resources {
		names = append(names, r.Name)
	}
	assert.Equal(t, []string{"image0001.jpg", "image0003.png"}, names)
	plate, err := book.ReadResource(book.Resources[1])
	require.NoError(t, err)
	assert.Equal(t, testgen.KF8SamplePlate, plate)

	require.Len(t, book.TOC, 2)
	assert.Equal(t, "Chapter One", book.TOC[0].Title)
	assert.True(t, strings.HasPrefix(textAfter(book.TOC[0].Href), "Chapter One"), book.TOC[0].Href)
	require.Len(t, book.TOC[0].Children, 1)
	assert.True(t, strings.HasPrefix(textAfter(book.TOC[0].Children[0].Href), "Later in chapter one."))
	assert.Equal(t, "Chapter Two", book.TOC[1].Title)
	assert.True(t, strings.HasPrefix(book.TOC[1].Href, "part0001.xhtml#"), book.TOC[1].Href)
	assert.True(t, strings.HasPrefix(textAfter(book.TOC[1].Href), "Chapter Two"))
}

// A stored chapter's filepos location finds the same place as the table of
// contents.
func TestReadContent_KF7ChapterHrefs(t *testing.T) {
	t.Parallel()
	data := testgen.BuildMOBI(t, testgen.KF7Sample())
	book, err := ReadContent(t.Context(), bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	tocs, err := ReadTOC(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, tocs, 1)
	assert.Equal(t, BookMOBI6, tocs[0].Book)
	href, ok := book.Href(tocs[0].Entries[1].Locator)
	require.True(t, ok, tocs[0].Entries[1].Locator)
	assert.Equal(t, book.TOC[1].Href, href)
	_, ok = book.Href("kindle:pos:fid:0000:off:0000000000")
	assert.False(t, ok)
}

// The cleanup turns Mobipocket markup into valid XHTML.
func TestCleanKF7Markup(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct{ in, want string }{
		"a self-closed anchor holds nothing": {`<p><a name="x"/>Text</p>`, `<p><a id="x"></a>Text</p>`},
		"an end tag on a void element":       {`<p>a<br></br>b</p>`, `<p>a<br/>b</p>`},
		"font becomes a span":                {`<p><font size="+1" color="red" face="Serif">x</font></p>`, `<p><span style="color: red; font-family: Serif; font-size: larger">x</span></p>`},
		"a block in an inline element":       {`<b><p>x</p></b>`, `<div style="font-weight: bold"><p>x</p></div>`},
		"paragraph width and height":         {`<p width="-14pt" height="1em" align="justify">x</p>`, `<p style="text-indent: -14pt; margin-top: 1em; text-align: justify">x</p>`},
		"unknown and Mobipocket elements":    {`<p><mbp:nu>a</mbp:nu><o:p>b</o:p><blink>c</blink></p>`, `<p>abc</p>`},
		"an anchor in a list moves into it":  {`<ul><a id="x"></a><li>a</li></ul>`, `<ul><li><a id="x"></a>a</li></ul>`},
		"ids stay unique":                    {`<p id="a">x</p><p id="a">y</p>`, `<p id="a">x</p><p>y</p>`},
		"a percentage image width":           {`<img recindex="00001" width="100%">`, `<img src="image0001.png" style="width: 100%" alt=""/>`},
		"an image that names nothing":        {`<p>a<img recindex="00009">b</p>`, `<p>ab</p>`},
		"a block in a heading gives way":     {`<h1><center>x</center><font><p>y</p></font></h1>`, `<h1>xy</h1>`},
		"scripts and forms go":               {`<p>a<script>x()</script><input type="text"/>b</p>`, `<p>ab</p>`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			image := func(n uint64) string {
				if n == 1 {
					return "image0001.png"
				}
				return ""
			}
			doc, _, err := kf7Documents([]byte(`<html><body>`+c.in+`</body></html>`), "Title", image)
			require.NoError(t, err)
			require.Len(t, doc, 1)
			body := string(doc[0])
			body = body[strings.Index(body, "<body>")+len("<body>") : strings.LastIndex(body, "</body>")]
			assert.Equal(t, c.want, body)
			xhtmlText(t, doc[0])
		})
	}
}

// A book nested deeper than any real one is declined, so the tree is never
// too deep to walk recursively.
func TestKF7Documents_DeclinesDeepNesting(t *testing.T) {
	t.Parallel()
	text := "<html><body>" + strings.Repeat("<div>", 1000) + "x</body></html>"
	_, _, err := kf7Documents([]byte(text), "Title", nil)
	var unsupported *UnsupportedError
	require.ErrorAs(t, err, &unsupported)

	_, _, err = kf7Documents([]byte("<html><body>"+strings.Repeat("<div>", 100)+"x</body></html>"), "Title", nil)
	require.NoError(t, err)
}

// Each place gets its anchor where it changes no text.
func TestWithFileposAnchors(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		text, at, want string
	}{
		"in text":               {`<p>Chapter</p>`, "pter", `<p>Cha<a id="filepos6"></a>pter</p>`},
		"in a tag":              {`<p class="x">y</p>`, `class`, `<a id="filepos3"></a><p class="x">y</p>`},
		"in a reference":        {`<p>x &amp; y</p>`, "amp", `<p>x <a id="filepos6"></a>&amp; y</p>`},
		"in a link":             {`<p><a href="#x">Chapter</a></p>`, "pter", `<p><a id="filepos19"></a><a href="#x">Chapter</a></p>`},
		"after a self-closed a": {`<p><a name="x"/>Chapter</p>`, "pter", `<p><a name="x"/>Cha<a id="filepos19"></a>pter</p>`},
		"after a closed link":   {`<p><a href="#x">A</a> Chapter</p>`, "pter", `<p><a href="#x">A</a> Cha<a id="filepos25"></a>pter</p>`},
		"in a UTF-8 character":  {`<p>xé</p>`, "\xa9", `<p>x<a id="filepos5"></a>é</p>`},
		"in the head":           {`<html><head><style>p {}</style></head><body><p>x</p>`, "<html", `<html><head><style>p {}</style></head><body><a id="filepos0"></a><p>x</p>`},
		"past the end":          {`<p>x</p>`, "", `<p>x</p><a id="filepos99"></a>`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pos := 99
			if c.at != "" {
				pos = strings.Index(c.text, c.at)
			}
			assert.Equal(t, c.want, string(withFileposAnchors([]byte(c.text), map[int]bool{pos: true}, true)))
		})
	}
	assert.Equal(t, map[int]bool{12: true, 3: true}, fileposPlaces([]byte(`<a filepos=" 12">x</a><a FILEPOS='0003'>y</a>`)))
}

// A page break inside elements that wrap the text still cuts it, each
// wrapper split around it.
func TestKF7Documents_PageBreakInWrappers(t *testing.T) {
	t.Parallel()
	docs, ids, err := kf7Documents([]byte(`<html><body><div id="all" class="book"><font size="3"><p>One</p><mbp:pagebreak/><p id="two">Two</p></font></div></body></html>`), "Title", nil)
	require.NoError(t, err)
	require.Len(t, docs, 2)
	body := func(doc []byte) string {
		s := string(doc)
		return s[strings.Index(s, "<body>")+len("<body>") : strings.LastIndex(s, "</body>")]
	}
	assert.Equal(t, `<div id="all" class="book"><div style="font-size: medium"><p>One</p></div></div>`, body(docs[0]))
	assert.Equal(t, `<div class="book"><div style="font-size: medium"><p id="two">Two</p></div></div>`, body(docs[1]))
	assert.Equal(t, 1, ids["two"])

	// One inside a list or table stays where it is and cuts nothing.
	docs, _, err = kf7Documents([]byte(`<html><body><ul><li>One<mbp:pagebreak/></li><li>Two</li></ul></body></html>`), "Title", nil)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

// A MOBI6 book's text is as long as its header says: a text record past
// that, which old files may hold and which need not decompress, is not
// read.
func TestReadContent_KF7IgnoresTextPastItsLength(t *testing.T) {
	t.Parallel()
	opts := testgen.KF7Sample()
	opts.JunkTextRecord = true
	book, err := readBuiltContent(t, opts)
	require.NoError(t, err)
	checkKF7Content(t, book)
}
