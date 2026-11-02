package testutils

import (
	"archive/zip"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// writeMinimalEPUB writes a small valid EPUB at path so E2E tests can download
// a generated file. It carries only a title, an identifier, and four short
// chapters, so the reader e2e tests can turn pages and reach the end. The
// first is styled with a white body and black text the way Project Gutenberg
// EPUBs are, so the reader e2e test can check that themes override book
// colors. extraParagraphs adds filler text to the first chapter, so a test
// can turn pages within a chapter as well as across chapters.
func writeMinimalEPUB(path, title string, extraParagraphs int) (err error) {
	escaped := html.EscapeString(title)
	filler := strings.Repeat("<p>"+strings.Repeat("Filler text to fill a page. ", 10)+"</p>", extraParagraphs)
	entries := []epubEntry{
		{"META-INF/container.xml", epubContainerXML},
		{"OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>` + escaped + `</dc:title>
    <dc:identifier id="bookid">urn:uuid:e2e-test-book</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter3" href="chapter3.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter4" href="chapter4.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
    <itemref idref="chapter2"/>
    <itemref idref="chapter3"/>
    <itemref idref="chapter4"/>
  </spine>
</package>`},
		{"OEBPS/chapter1.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + escaped + `</title>
<style>body { color: black; background-color: white; } a { color: blue; }</style></head>
<body><p>Test chapter. <a href="#end">A link.</a></p>` + filler + `<p id="end">The end.</p></body>
</html>`},
	}
	for i, name := range []string{"Two", "Three", "Four"} {
		entries = append(entries, epubEntry{
			fmt.Sprintf("OEBPS/chapter%d.xhtml", i+2), `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + escaped + `</title></head>
<body><p>Chapter ` + name + ` text.</p></body>
</html>`,
		})
	}
	return writeEPUBArchive(path, entries)
}

// fixedLayoutPageCount is the number of pages in a fixed-layout EPUB.
const fixedLayoutPageCount = 8

var fixedLayoutColors = []string{"#fde68a", "#bfdbfe", "#bbf7d0", "#fbcfe8"}

// writeFixedLayoutEPUB writes a fixed-layout EPUB, which the reader renders
// through a different path than a reflowable one: each page has a fixed size
// and is scaled to fit. Every page shows its number on its own color, with
// text to select, and the first links to page 6. rtl makes pages run right to
// left, as manga do.
func writeFixedLayoutEPUB(path, title string, rtl bool) error {
	escaped := html.EscapeString(title)
	var manifest, spine, nav strings.Builder
	entries := []epubEntry{{"META-INF/container.xml", epubContainerXML}}
	for i := 1; i <= fixedLayoutPageCount; i++ {
		name := fmt.Sprintf("page%d.xhtml", i)
		fmt.Fprintf(&manifest, `    <item id="page%d" href="%s" media-type="application/xhtml+xml"/>`+"\n", i, name)
		fmt.Fprintf(&spine, `    <itemref idref="page%d"/>`+"\n", i)
		fmt.Fprintf(&nav, `<li><a href="%s">Page %d</a></li>`, name, i)
		link := ""
		if i == 1 {
			link = `<p style="font-size:32px"><a href="page6.xhtml">Jump to page 6</a></p>`
		}
		color := fixedLayoutColors[(i-1)%len(fixedLayoutColors)]
		entries = append(entries, epubEntry{"OEBPS/" + name, fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Page %[1]d</title><meta name="viewport" content="width=600, height=800"/></head>
<body style="margin:0;width:600px;height:800px;overflow:hidden;background:%[2]s;color:#111;font-family:sans-serif;text-align:center">
<h1 style="font-size:160px;margin:120px 0 40px">%[1]d</h1>
<p style="font-size:32px;margin:0 40px">Fixed page %[1]d. This sentence is here to select.</p>
%[3]s
</body>
</html>`, i, color, link)})
	}
	progression := "ltr"
	if rtl {
		progression = "rtl"
	}
	entries = append(entries,
		epubEntry{"OEBPS/nav.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>` + escaped + `</title></head>
<body><nav epub:type="toc"><ol>` + nav.String() + `</ol></nav></body>
</html>`},
		epubEntry{"OEBPS/content.opf", `<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid" prefix="rendition: http://www.idpf.org/vocab/rendition/#">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>` + escaped + `</dc:title>
    <dc:identifier id="bookid">urn:uuid:` + progression + `-fixed-layout-test-book</dc:identifier>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2026-01-01T00:00:00Z</meta>
    <meta property="rendition:layout">pre-paginated</meta>
    <meta property="rendition:spread">auto</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
` + manifest.String() + `  </manifest>
  <spine page-progression-direction="` + progression + `">
` + spine.String() + `  </spine>
</package>`},
	)
	return writeEPUBArchive(path, entries)
}

type epubEntry struct{ name, body string }

const epubContainerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

// writeEPUBArchive zips entries into an EPUB at path.
func writeEPUBArchive(path string, entries []epubEntry) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()

	zw := zip.NewWriter(f)
	// The mimetype entry must come first and be stored uncompressed.
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return errors.WithStack(err)
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		return errors.WithStack(err)
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			return errors.WithStack(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			return errors.WithStack(err)
		}
	}
	return errors.WithStack(zw.Close())
}
