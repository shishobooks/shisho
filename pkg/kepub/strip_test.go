package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// kepubEntries is a KePub as Kobo's writer leaves it: spans, wrapper divs,
// the style hacks, and kobo.js, with links that target span ids from
// another chapter, the same chapter, and the NCX. ch3 runs a script of its
// own.
func kepubEntries() []Entry {
	head := func(title, extra string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + title + `</title>` + extra + `<style type="text/css" id="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0; }</style><script type="text/javascript" src="../kobo.js"/></head>
<body><div id="book-columns"><div id="book-inner">`
	}
	const foot = `</div></div></body>
</html>`
	return []Entry{
		{Name: "mimetype", Content: []byte("application/epub+zip")},
		{Name: "kobo.js", Content: []byte(koboJS)},
		{Name: "META-INF/container.xml", Content: []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{Name: "OEBPS/content.opf", Content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">urn:uuid:strip-book</dc:identifier>
    <dc:title>Strip Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav scripted"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="plate" href="images/plate.png" media-type="image/png"/>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml" properties="scripted"/>
    <item id="ch2" href="ch2.xhtml" media-type="application/xhtml+xml" properties="scripted"/>
    <item id="ch3" href="ch3.xhtml" media-type="application/xhtml+xml" properties="scripted"/>
    <item id="ch4" href="ch4.xhtml" media-type="application/xhtml+xml" properties="scripted"/>
    <item id="own-kobo" href="extra/kobo.js" media-type="application/javascript"/>
    <item id="kobo-js" href="../kobo.js" media-type="application/javascript"/>
  </manifest>
  <spine toc="ncx"><itemref idref="ch1"/><itemref idref="ch2"/><itemref idref="ch3"/><itemref idref="ch4"/></spine>
  <guide><reference type="text" title="Note" href="ch2.xhtml#kobo.2.1"/></guide>
</package>`)},
		{Name: "OEBPS/extra/kobo.js", Content: []byte("// the book's own file")},
		{Name: "OEBPS/nav.xhtml", Content: []byte(head("Contents", "") + `<nav><ol>
<li><a href="ch1.xhtml"><span class="koboSpan" id="kobo.1.1">One</span></a></li>
<li><a href="ch2.xhtml#kobo.2.1"><span class="koboSpan" id="kobo.2.1">Second note</span></a></li>
</ol></nav>` + foot)},
		{Name: "OEBPS/toc.ncx", Content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><navMap>
<navPoint id="n1" playOrder="1"><navLabel><text>One</text></navLabel><content src="ch1.xhtml"/></navPoint>
<navPoint id="n2" playOrder="2"><navLabel><text>Second note</text></navLabel><content src="ch2.xhtml#kobo.2.1"/></navPoint>
</navMap></ncx>`)},
		{Name: "OEBPS/images/plate.png", Content: []byte("plate bytes")},
		{Name: "OEBPS/ch1.xhtml", Content: []byte(head("One", "") + `
<h1><span class="koboSpan" id="kobo.1.1">Chapter One</span></h1>
<p><span class="koboSpan" id="kobo.2.1">First sentence.</span> <span class="koboSpan" id="kobo.2.2">See <a href="ch2.xhtml#kobo.2.1">the note</a>, or <a href="#kobo.3.1">the plate</a>, or <a href="ch2.xhtml#note">the first note</a>, or <a href='ch2.xhtml#kobo.2.2'>more</a>.</span><a id="mark"/></p>
<p title="a/>b"><span class="koboSpan" id="kobo.4.1">Titled.</span></p>
<p><span class="koboSpan" id="kobo.3.1"><img src="images/plate.png" alt="Plate"/></span></p>
` + foot)},
		{Name: "OEBPS/ch2.xhtml", Content: []byte(head("Two", "") + `
<p id="note"><span class="koboSpan" id="kobo.1.1">A note.</span></p>
<p><span class="koboSpan" id="kobo.2.1">A second note.</span> <span class="koboSpan" id="kobo.2.2">More of it.</span></p>
` + foot)},
		{Name: "OEBPS/ch3.xhtml", Content: []byte(head("Three", `<script type="text/javascript">var own = "<div/>";</script>`) + `
<p><span class="koboSpan" id="kobo.1.1">Three.</span></p>
` + foot)},
		{Name: "OEBPS/ch4.xhtml", Content: []byte(head("Four", "") + `
<form><p><span class="koboSpan" id="kobo.1.1">Four.</span></p></form>
` + foot)},
	}
}

func stripEntries(t *testing.T, entries []Entry) map[string]string {
	t.Helper()
	strip, err := PrepareStrip(context.Background(), entries)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, strip.Write(context.Background(), &out))
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	require.NotEmpty(t, zr.File)
	assert.Equal(t, "mimetype", zr.File[0].Name)
	assert.Equal(t, zip.Store, zr.File[0].Method)
	files := map[string]string{}
	for _, f := range zr.File {
		data, err := readZipFile(f)
		require.NoError(t, err)
		files[f.Name] = string(data)
	}
	return files
}

// bodyText returns the text of a document's body with runs of whitespace
// collapsed, for comparing a document's text across conversions.
func bodyText(t *testing.T, doc string) string {
	t.Helper()
	z := html.NewTokenizer(strings.NewReader(doc))
	var b strings.Builder
	inBody := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		name, _ := z.TagName()
		switch {
		case tt == html.SelfClosingTagToken:
			z.NextIsNotRawText()
		case tt == html.StartTagToken && string(name) == "body":
			inBody = true
		case tt == html.EndTagToken && string(name) == "body":
			inBody = false
		case tt == html.TextToken && inBody:
			b.Write(z.Text())
		}
	}
	require.NotZero(t, b.Len(), "no body text")
	return strings.Join(strings.Fields(b.String()), " ")
}

var (
	hrefPattern = regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	idPattern   = regexp.MustCompile(`\bid="([^"]*)"`)
)

// assertLinksResolve checks that every relative link in files with a
// fragment names an id its target document has.
func assertLinksResolve(t *testing.T, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".ncx") {
			continue
		}
		for _, m := range hrefPattern.FindAllStringSubmatch(content, -1) {
			u, err := url.Parse(m[1])
			require.NoError(t, err)
			if u.Scheme != "" || u.Fragment == "" {
				continue
			}
			target := name
			if u.Path != "" {
				target = path.Join(path.Dir(name), u.Path)
			}
			doc, ok := files[target]
			require.True(t, ok, "%s links to %s, which is missing", name, m[1])
			var ids []string
			for _, id := range idPattern.FindAllStringSubmatch(doc, -1) {
				ids = append(ids, id[1])
			}
			assert.Contains(t, ids, u.Fragment, "%s links to %s", name, m[1])
		}
	}
}

func TestStrip_RemovesKoboMarkup(t *testing.T) {
	t.Parallel()
	files := stripEntries(t, kepubEntries())

	assert.NotContains(t, files, "kobo.js")
	assert.Equal(t, "// the book's own file", files["OEBPS/extra/kobo.js"], "a file of the book's own named kobo.js stays")
	for name, content := range files {
		if !strings.HasSuffix(name, ".xhtml") {
			continue
		}
		assert.NotContains(t, content, "koboSpan", name)
		assert.NotContains(t, content, `id="kobo.`, name)
		assert.NotContains(t, content, "book-columns", name)
		assert.NotContains(t, content, "book-inner", name)
		assert.NotContains(t, content, "kobostylehacks", name)
		assert.NotContains(t, content, "kobo.js", name)
		assert.True(t, strings.HasPrefix(content, `<?xml version="1.0" encoding="UTF-8"?>`), "%s keeps its XML declaration", name)
	}
	assert.Contains(t, files["OEBPS/ch3.xhtml"], `var own = "<div/>";`, "a script of the book's own stays as it was")
	assert.Contains(t, files["OEBPS/ch1.xhtml"], `<p title="a/>b">Titled.</p>`)
	assert.Contains(t, files["OEBPS/ch1.xhtml"], `<a id="mark"/></p>`, "a self-closed anchor wraps nothing")

	opf := files["OEBPS/content.opf"]
	assert.NotContains(t, opf, `id="kobo-js"`)
	assert.Contains(t, opf, `<item id="own-kobo" href="extra/kobo.js"`)
	assert.Regexp(t, `<item id="ch4"[^>]+properties="scripted"`, opf, "a document with a form stays scripted")
	assert.Regexp(t, `<item id="nav" href="nav.xhtml" media-type="application/xhtml\+xml" properties="nav"/>`, opf)
	assert.Regexp(t, `<item id="ch1" href="ch1.xhtml" media-type="application/xhtml\+xml"/>`, opf)
	assert.Regexp(t, `<item id="ch3"[^>]+properties="scripted"`, opf, "a document running its own script stays scripted")
	assert.Equal(t, "plate bytes", files["OEBPS/images/plate.png"])
}

func TestStrip_KeepsTextAndImages(t *testing.T) {
	t.Parallel()
	files := stripEntries(t, kepubEntries())
	for _, e := range kepubEntries() {
		if strings.HasSuffix(e.Name, ".xhtml") {
			assert.Equal(t, bodyText(t, string(e.Content)), bodyText(t, files[e.Name]), e.Name)
		}
	}
	assert.Contains(t, files["OEBPS/ch1.xhtml"], `<img src="images/plate.png" alt="Plate"/>`)
}

// A link to a removed span goes to the nearest element that remains: the
// paragraph holding the span, which gets an id when it has none.
func TestStrip_RetargetsLinksToRemovedSpans(t *testing.T) {
	t.Parallel()
	files := stripEntries(t, kepubEntries())
	assertLinksResolve(t, files)

	ch1, ch2 := files["OEBPS/ch1.xhtml"], files["OEBPS/ch2.xhtml"]
	assert.Contains(t, ch1, `href="ch2.xhtml#note"`, "a link to an id that stays is untouched")
	m := regexp.MustCompile(`<p id="([^"]+)">A second note\.`).FindStringSubmatch(ch2)
	require.NotNil(t, m, "the second note's paragraph gets an id")
	assert.Contains(t, ch1, `href="ch2.xhtml#`+m[1]+`"`)
	assert.Contains(t, files["OEBPS/toc.ncx"], `src="ch2.xhtml#`+m[1]+`"`)
	assert.Contains(t, files["OEBPS/content.opf"], `href="ch2.xhtml#`+m[1]+`"`, "the guide's reference is retargeted")
	assert.Contains(t, ch1, `<a href='ch2.xhtml#`+m[1]+`'>more</a>`, "a single-quoted link is retargeted")
	assert.Contains(t, files["OEBPS/nav.xhtml"], `href="ch2.xhtml#`+m[1]+`"`)
	assert.Regexp(t, `<p id="[^"]+"><img src="images/plate.png"`, ch1, "the plate's paragraph gets an id")
}

// Stripping the KePub converted from an EPUB gives back the EPUB's text and
// links.
func TestStrip_RoundTripFromEPUB(t *testing.T) {
	t.Parallel()
	source := validityEntries("3.0")
	source[4].Content = []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p id="top">Some text. <a href="#top">Back to the top.</a></p><p>Another: <a href="../nav.xhtml">contents</a>.</p></body></html>`)
	var kepubOut bytes.Buffer
	require.NoError(t, NewConverter().WriteEPUB(context.Background(), &kepubOut, source))
	zr, err := zip.NewReader(bytes.NewReader(kepubOut.Bytes()), int64(kepubOut.Len()))
	require.NoError(t, err)
	var kepubEntries []Entry
	for _, f := range zr.File {
		kepubEntries = append(kepubEntries, Entry{Name: f.Name, File: f})
	}

	files := stripEntries(t, kepubEntries)
	assertLinksResolve(t, files)
	assert.NotContains(t, files, "kobo.js")
	for _, e := range source {
		if strings.HasSuffix(e.Name, ".xhtml") {
			assert.Equal(t, bodyText(t, string(e.Content)), bodyText(t, files[e.Name]), e.Name)
			assert.NotContains(t, files[e.Name], "kobo", e.Name)
		}
	}
	ch1 := files["OEBPS/text/ch1.xhtml"]
	assert.Contains(t, ch1, `<a href="#top">`)
	assert.Contains(t, ch1, `<a href="../nav.xhtml">`)
	assert.NotContains(t, files["OEBPS/content.opf"], "scripted")
}

// Converting an EPUB to KePub and stripping the KePub gives back each
// content document byte for byte.
func TestStrip_RoundTripGivesBackEachDocument(t *testing.T) {
	t.Parallel()
	source := validityEntries("3.0")
	for i, e := range source {
		if e.Name == "OEBPS/text/ch1.xhtml" {
			source[i].Content = []byte(fidelityDoc(`<link rel="stylesheet" href="../style.css"/>`, "\n"+fidelityBody+"\n<p>Plate: <img src=\"../images/cover.png\" alt=\"\"/></p>\n"))
		}
	}
	var kepubOut bytes.Buffer
	require.NoError(t, NewConverter().WriteEPUB(context.Background(), &kepubOut, source))
	zr, err := zip.NewReader(bytes.NewReader(kepubOut.Bytes()), int64(kepubOut.Len()))
	require.NoError(t, err)
	var kepubEntries []Entry
	for _, f := range zr.File {
		kepubEntries = append(kepubEntries, Entry{Name: f.Name, File: f})
	}

	files := stripEntries(t, kepubEntries)
	for _, e := range source {
		if strings.HasSuffix(e.Name, ".xhtml") {
			assert.Equal(t, string(e.Content), files[e.Name], e.Name)
		}
	}
}

// A plain EPUB has nothing to strip, so it comes through as it was.
func TestStrip_PlainEPUB(t *testing.T) {
	t.Parallel()
	source := validityEntries("3.0")
	files := stripEntries(t, source)
	for _, e := range source {
		if e.Name != "mimetype" {
			assert.Equal(t, string(e.Content), files[e.Name], e.Name)
		}
	}
}

// Only Kobo's markup changes: everything else in a document is copied as it
// was, including markup an HTML parser would restructure, case-sensitive
// SVG, and a preformatted block's leading newline.
func TestStrip_KeepsOtherMarkupAsItWas(t *testing.T) {
	t.Parallel()
	const body = `<p>a<div>b</div>c</p>
<pre>
line</pre>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><linearGradient id="g"/><path d="M0 0"/></svg>
<p>Tom &amp; Jerry&#160;<br/><a id="anchor"/>after</p>
<![CDATA[ x < y ]]>
<!-- a <span class="koboSpan"> comment -->`
	doc := func(inner string) string {
		return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>T</title></head>
<body>` + inner + `</body>
</html>`
	}
	kobo := doc(`<div id="book-columns"><div id="book-inner"><p><span class="koboSpan" id="kobo.1.1">Hello.</span></p>` + body + `</div></div>`)
	out, _, err := stripDocument([]byte(kobo), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, doc(`<p>Hello.</p>`+body), string(out))
}

func TestPrepareStrip_NoOPF(t *testing.T) {
	t.Parallel()
	_, err := PrepareStrip(context.Background(), []Entry{{Name: "mimetype", Content: []byte("application/epub+zip")}})
	require.Error(t, err)
}

// A koboSpan with attributes of the book's own stays without its Kobo id
// and class, and a link to it goes to the span itself.
func TestStripDocument_KoboSpanWithOtherAttributes(t *testing.T) {
	t.Parallel()
	in := `<html><head></head><body><p><span class="koboSpan extra" id="kobo.1.1" style='color: red'>Hi.</span></p></body></html>`
	out, retarget, err := stripDocument([]byte(in), map[string]bool{"kobo.1.1": true}, nil)
	require.NoError(t, err)
	assert.Equal(t, `<html><head></head><body><p><span class="extra" style='color: red' id="shisho-1-1">Hi.</span></p></body></html>`, string(out))
	assert.Equal(t, map[string]string{"kobo.1.1": "shisho-1-1"}, retarget)
}

func TestStripDocument_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       string
		targets  []string
		rewrite  bool
		want     string
		retarget map[string]string
	}{
		{
			name:     "a link into a kept span goes to an id that remains",
			in:       `<p><span class="koboSpan" id="kobo.1.1" style="c"><span class="koboSpan" id="kobo.1.2">x</span></span></p>`,
			targets:  []string{"kobo.1.2"},
			want:     `<p><span style="c" id="shisho-1-2">x</span></p>`,
			retarget: map[string]string{"kobo.1.2": "shisho-1-2"},
		},
		{
			name:     "an empty id is replaced, not repeated",
			in:       `<p id="" class="a"><span class="koboSpan" id="kobo.1.1">x</span></p>`,
			targets:  []string{"kobo.1.1"},
			want:     `<p class="a" id="shisho-1-1">x</p>`,
			retarget: map[string]string{"kobo.1.1": "shisho-1-1"},
		},
		{
			name: "attribute names and unquoted values are matched as HTML does",
			in:   `<p><SPAN CLASS=koboSpan ID="kobo.1.1">x</SPAN><span class='koboSpan a"b' id='kobo.1.2'>y</span></p>`,
			want: `<p>x<span class='a&#34;b'>y</span></p>`,
		},
		{
			name:     "text inside another attribute's value is left alone",
			in:       `<p><span title=' class="z" id="kobo.9.9"' class="koboSpan" id="kobo.1.1">x</span><a title='see href="#kobo.1.1"' href="#kobo.1.1">link</a></p>`,
			targets:  []string{"kobo.1.1"},
			rewrite:  true,
			want:     `<p><span title=' class="z" id="kobo.9.9"' id="shisho-1-1">x</span><a title='see href="#kobo.1.1"' href="#shisho-1-1">link</a></p>`,
			retarget: map[string]string{"kobo.1.1": "shisho-1-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			targets := map[string]bool{}
			for _, id := range tt.targets {
				targets[id] = true
			}
			var rewrite func(string) string
			if tt.rewrite {
				// The pass that learns the ids runs first, as PrepareStrip does.
				_, learned, err := stripDocument([]byte(tt.in), targets, nil)
				require.NoError(t, err)
				s := &Strip{retarget: map[string]map[string]string{"doc.xhtml": learned}}
				rewrite = s.linkRewriter("doc.xhtml")
			}
			out, retarget, err := stripDocument([]byte(tt.in), targets, rewrite)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
			if tt.retarget == nil {
				tt.retarget = map[string]string{}
			}
			assert.Equal(t, tt.retarget, retarget)
		})
	}
}

// A CDATA section is copied as it was, even when it holds a '>' before
// markup that looks like Kobo's.
func TestStripDocument_CDATA(t *testing.T) {
	t.Parallel()
	in := `<p><span class="koboSpan" id="kobo.1.1">x</span></p><svg><![CDATA[ a > b <span class="koboSpan" id="kobo.2.1">y</span> ]]></svg>`
	out, _, err := stripDocument([]byte(in), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, `<p>x</p><svg><![CDATA[ a > b <span class="koboSpan" id="kobo.2.1">y</span> ]]></svg>`, string(out))
}

// A document without a body start tag strips back to what it was. With no
// head either, the head added for the style and script stays, empty.
func TestStrip_RoundTripWithoutBodyTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "content after the head",
			in:   "<html><head><title>T</title></head>\n<p>Hi. <img src=\"a.png\"/></p>\n</html>",
			want: "<html><head><title>T</title></head>\n<p>Hi. <img src=\"a.png\"/></p>\n</html>",
		},
		{
			name: "no head",
			in:   "<html><p>Hi.</p></html>",
			want: "<html><head></head><p>Hi.</p></html>",
		},
		{
			name: "a fragment",
			in:   "<p>Hi.</p>",
			want: "<head></head><p>Hi.</p>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kobo, err := transformDocument([]byte(tt.in), NewSpanCounter(), "../kobo.js")
			require.NoError(t, err)
			require.Contains(t, string(kobo), `class="koboSpan"`)
			out, _, err := stripDocument(kobo, nil, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}
