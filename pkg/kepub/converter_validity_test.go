package kepub

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validityEntries is an EPUB with its package in OEBPS/, a nav document for
// EPUB 3, and one chapter in a subdirectory.
func validityEntries(version string) []Entry {
	navItem, navEntry := "", []Entry(nil)
	if version == "3.0" {
		navItem = `<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`
		navEntry = []Entry{{Name: "OEBPS/nav.xhtml", Content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head>
<body><nav epub:type="toc"><ol><li><a href="text/ch1.xhtml">One</a></li></ol></nav></body></html>`)}}
	}
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="` + version + `" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="id">urn:uuid:8b1f3a6e-3a4c-4f1e-9a6b-2c1d0e9f8a7b</dc:identifier>
    <dc:title>Book</dc:title>
    <dc:language>en</dc:language>
    <meta name="cover" content="cover"/>
  </metadata>
  <manifest>
    ` + navItem + `
    <item id="cover" href="images/cover.png" media-type="image/png"/>
    <item id="ch1" href="text/ch1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	entries := []Entry{
		{Name: "mimetype", Content: []byte("application/epub+zip")},
		{Name: "META-INF/container.xml", Content: []byte(`<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)},
		{Name: "OEBPS/content.opf", Content: []byte(opf)},
		{Name: "OEBPS/images/cover.png", Content: []byte("png")},
		{Name: "OEBPS/text/ch1.xhtml", Content: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>One</title></head><body><p>Some text.</p></body></html>`)},
	}
	return append(entries, navEntry...)
}

func convertEntries(t *testing.T, entries []Entry) map[string]string {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, NewConverter().WriteEPUB(context.Background(), &out, entries))
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range zr.File {
		data, err := readZipFile(f)
		require.NoError(t, err)
		files[f.Name] = string(data)
	}
	return files
}

var itemTagRE = regexp.MustCompile(`<item\b[^>]*>`)

// manifestItemTags returns the manifest's item tags keyed by the zip entry
// each names, for a package at OEBPS/content.opf.
func manifestItemTags(opf string) map[string]string {
	tags := map[string]string{}
	for _, tag := range itemTagRE.FindAllString(opf, -1) {
		tags[path.Join("OEBPS", extractAttribute(tag, "href"))] = tag
	}
	return tags
}

// A KePub converted from an EPUB 3 declares the kobo.js its content
// documents load, and marks those documents as scripted, as EPUB 3
// requires.
func TestWriteEPUB_EPUB3DeclaresKoboScript(t *testing.T) {
	t.Parallel()
	files := convertEntries(t, validityEntries("3.0"))
	require.Contains(t, files, "kobo.js")
	items := manifestItemTags(files["OEBPS/content.opf"])

	require.Contains(t, items, "kobo.js", "kobo.js is in the manifest")
	assert.Equal(t, "application/javascript", extractAttribute(items["kobo.js"], "media-type"))
	for _, doc := range []string{"OEBPS/text/ch1.xhtml", "OEBPS/nav.xhtml"} {
		assert.Contains(t, files[doc], "kobo.js", "%s loads kobo.js", doc)
		assert.Contains(t, strings.Fields(extractAttribute(items[doc], "properties")), "scripted", "%s is scripted", doc)
	}
	assert.Contains(t, strings.Fields(extractAttribute(items["OEBPS/nav.xhtml"], "properties")), "nav", "the nav keeps its property")
	assert.Contains(t, strings.Fields(extractAttribute(items["OEBPS/images/cover.png"], "properties")), "cover-image")
}

// A KePub converted from an EPUB 2 declares kobo.js too, and marks nothing
// as scripted: EPUB 2 has no such property. Its cover still gets the
// cover-image property, which Kobo relies on even though EPUB 2 has no
// manifest properties.
func TestWriteEPUB_EPUB2DeclaresKoboScript(t *testing.T) {
	t.Parallel()
	files := convertEntries(t, validityEntries("2.0"))
	opf := files["OEBPS/content.opf"]
	items := manifestItemTags(opf)

	require.Contains(t, items, "kobo.js", "kobo.js is in the manifest")
	assert.NotContains(t, opf, "scripted")
	assert.Contains(t, strings.Fields(extractAttribute(items["OEBPS/images/cover.png"], "properties")), "cover-image")
}

// Converting a KePub again declares kobo.js only once.
func TestWriteEPUB_KoboScriptDeclaredOnce(t *testing.T) {
	t.Parallel()
	files := convertEntries(t, validityEntries("3.0"))
	var again []Entry
	for name, data := range files {
		again = append(again, Entry{Name: name, Content: []byte(data)})
	}
	files = convertEntries(t, again)
	opf := files["OEBPS/content.opf"]
	assert.Equal(t, 1, strings.Count(opf, `kobo.js"`), opf)
	items := manifestItemTags(opf)
	assert.Equal(t, []string{"scripted"}, strings.Fields(extractAttribute(items["OEBPS/text/ch1.xhtml"], "properties")))
}

// A content document whose manifest href is percent-encoded is converted
// and marked scripted like any other.
func TestWriteEPUB_EscapedHref(t *testing.T) {
	t.Parallel()
	entries := validityEntries("3.0")
	for i, e := range entries {
		switch e.Name {
		case "OEBPS/content.opf":
			entries[i].Content = []byte(strings.Replace(string(e.Content), `href="text/ch1.xhtml"`, `href="text/ch%201.xhtml"`, 1))
		case "OEBPS/text/ch1.xhtml":
			entries[i].Name = "OEBPS/text/ch 1.xhtml"
		}
	}
	files := convertEntries(t, entries)
	assert.Contains(t, files["OEBPS/text/ch 1.xhtml"], "koboSpan")
	items := manifestItemTags(files["OEBPS/content.opf"])
	assert.Contains(t, files["OEBPS/content.opf"], `href="text/ch%201.xhtml" media-type="application/xhtml+xml" properties="scripted"`, items)
}

// A single-quoted properties attribute keeps its quotes matched when a
// property is added to it.
func TestWriteEPUB_SingleQuotedProperties(t *testing.T) {
	t.Parallel()
	entries := validityEntries("3.0")
	for i, e := range entries {
		if e.Name == "OEBPS/content.opf" {
			entries[i].Content = []byte(strings.Replace(string(e.Content), `properties="nav"`, `properties='nav'`, 1))
		}
	}
	opf := convertEntries(t, entries)["OEBPS/content.opf"]
	assert.Contains(t, opf, `properties='nav scripted'`)
	require.NoError(t, xml.Unmarshal([]byte(opf), new(struct{})), opf)
}
